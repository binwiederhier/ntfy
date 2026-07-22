package cluster

import (
	"bytes"
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"sync"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	meshHTTPTimeout  = 5 * time.Second
	peerQueueSize    = 1024              // Bounded per-peer fan-out queue (drop on overflow)
	batchMaxMessages = 100               // Flush a batch early when it reaches this many messages
	batchMaxBytes    = 256 * 1024        // Flush a batch early when it reaches this size
	leaderLockKey    = int64(0x6e746679) // Advisory-lock key ("ntfy") for the singleton-job leader
	tag              = "cluster"
)

// meshCluster fans messages out directly to peer nodes over HTTP (the data plane), using
// PostgreSQL only as a control plane: the node_registry table for membership/discovery, and a
// Postgres advisory lock for singleton-job leader election. Fan-out never touches the database on
// the message path (only the cached peer list does). See plans/260715-scale-out-mesh.md.
//
// Each peer has its own bounded send queue and delivery worker, so a slow or wedged peer only
// backs up (and eventually drops) its own queue and never delays delivery to healthy peers.
type meshCluster struct {
	conf       *Config
	deliver    DeliverFunc
	registry   *registry
	leader     *pg.Leader
	httpClient *http.Client
	mu         sync.Mutex
	queues     map[string]*peerQueue // node_id -> queue; reconciled against the registry
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// newMeshCluster creates the mesh cluster: it ensures the registry table exists, registers this
// node, and starts the heartbeat/leader-election loop. Peer delivery workers are started lazily
// as peers appear in the registry.
func newMeshCluster(conf *Config, pool *db.DB, deliver DeliverFunc) (*meshCluster, error) {
	registry, err := newRegistry(pool, conf.NodeID, conf.AdvertiseURL, conf.NodeTTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &meshCluster{
		conf:       conf,
		deliver:    deliver,
		registry:   registry,
		leader:     pg.NewLeader(pool.Primary(), leaderLockKey),
		httpClient: &http.Client{Timeout: meshHTTPTimeout},
		queues:     make(map[string]*peerQueue),
		ctx:        ctx,
		cancel:     cancel,
	}
	c.wg.Add(1)
	go c.heartbeatLoop()
	return c, nil
}

// heartbeatLoop periodically refreshes this node's registry heartbeat, retries/confirms the
// leader lock, reconciles the per-peer queues against the registry, and (as leader) prunes
// long-dead registry rows.
func (c *meshCluster) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.conf.HeartbeatInterval)
	defer ticker.Stop()
	// Attempt leadership immediately: the ticker first fires a full interval after startup, and
	// a fresh single-node cluster should not be leaderless (skipping leader-gated jobs) until then
	if c.leader.TryAcquire(c.ctx) {
		metrics.ClusterLeader.Set(1)
	}
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.registry.Register(); err != nil {
				log.Tag(tag).Err(err).Warn("Failed to refresh node registry heartbeat")
			}
			if c.leader.TryAcquire(c.ctx) {
				metrics.ClusterLeader.Set(1)
				if err := c.registry.Prune(); err != nil {
					log.Tag(tag).Err(err).Warn("Failed to prune stale nodes")
				}
			} else {
				metrics.ClusterLeader.Set(0)
			}
			if peers, err := c.registry.LivePeers(); err == nil {
				c.reconcileQueues(peers)
			}
		}
	}
}

// reconcileQueues aligns the per-peer queues with the given live peer set: it updates the URLs of
// known peers and stops the queues (and workers) of peers that have left the registry. New queues
// are created lazily by Broadcast, not here, so a freshly joined peer is reachable immediately.
func (c *meshCluster) reconcileQueues(peers []peer) {
	metrics.ClusterPeers.Set(float64(len(peers)))
	alive := make(map[string]string, len(peers)) // node_id -> advertise URL
	for _, p := range peers {
		alive[p.nodeID] = p.advertiseURL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for nodeID, q := range c.queues {
		if url, ok := alive[nodeID]; ok {
			q.mu.Lock()
			q.url = deliverURL(url)
			q.mu.Unlock()
		} else {
			q.queue.Close() // Flushes the remainder; the worker exits when the queue is drained
			delete(c.queues, nodeID)
		}
	}
}

// queueFor returns the send queue for the given peer, creating it (and its delivery worker) if it
// does not exist yet. The caller must hold c.mu.
func (c *meshCluster) queueFor(p peer) *peerQueue {
	q, ok := c.queues[p.nodeID]
	if ok {
		return q
	}
	q = &peerQueue{
		url: deliverURL(p.advertiseURL),
		queue: util.NewLingerQueue(peerQueueSize, batchMaxMessages, batchMaxBytes,
			func(frag []byte) int { return len(frag) }, c.conf.BatchLinger),
	}
	c.queues[p.nodeID] = q
	c.wg.Add(1)
	go c.peerWorker(p.nodeID, q)
	return q
}

// Broadcast enqueues the message for delivery to every live peer node. Delivery is fire-and-forget
// via each peer's bounded batching queue; if a peer's queue is full the message is dropped for
// that peer (subscribers reconnect and re-poll history from the database).
func (c *meshCluster) Broadcast(msg *model.Message) error {
	frag, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	peers, err := c.registry.LivePeers()
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		return nil
	}
	metrics.ClusterMessagesBroadcast.Inc()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range peers {
		if !c.queueFor(p).queue.TryEnqueue(frag) {
			metrics.ClusterQueueDropped.Inc()
			log.Tag(tag).Warn("Fan-out queue for peer %s full, dropping message %s", p.nodeID, msg.ID)
		}
	}
	return nil
}

// peerWorker delivers batches of queued fan-out messages to a single peer. Batches form in the
// peer's LingerQueue (up to BatchLinger delay, flushed early on size/count caps); the worker
// exits when the queue is closed (peer left the registry, or mesh shutdown) and drained.
func (c *meshCluster) peerWorker(nodeID string, q *peerQueue) {
	defer c.wg.Done()
	for frags := range q.queue.Dequeue() {
		c.deliverToPeer(nodeID, q.DeliverURL(), assembleDeliverBody(frags))
		metrics.ClusterBatchesSent.Inc()
	}
}

// deliverToPeer POSTs a fan-out payload to a peer, authenticated with the shared cluster secret.
// Failures are logged and counted, never retried: fan-out is best-effort by design.
func (c *meshCluster) deliverToPeer(nodeID, url string, payload []byte) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Err(err).Warn("Failed to build fan-out request for peer %s", nodeID)
		return
	}
	req.Header.Set("Content-Type", contentTypeNDJSON)
	req.Header.Set(secretHeader, c.conf.Secret)
	req.Header.Set(originHeader, c.conf.NodeID)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.ctx.Err() == nil {
			metrics.ClusterSendErrors.Inc()
			log.Tag(tag).Err(err).Warn("Failed to deliver fan-out message to peer %s (%s)", nodeID, url)
		}
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Warn("Peer %s (%s) rejected fan-out with HTTP %d", nodeID, url, resp.StatusCode)
	}
}

// ServeDeliver handles an inbound fan-out request from a peer node. It authenticates via the
// shared cluster secret (constant-time compare, rejected before any parsing), skips requests
// carrying this node's own broadcasts (origin header; loop prevention), and streams the NDJSON
// body to local subscribers line by line, delivering each message as it is decoded.
func (c *meshCluster) ServeDeliver(w http.ResponseWriter, r *http.Request) {
	if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	origin := r.Header.Get(originHeader)
	if origin == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if origin == c.conf.NodeID {
		w.WriteHeader(http.StatusOK) // Our own broadcast; nothing to do
		return
	}
	// A batch can exceed its byte cap by one message, plus framing overhead
	maxBodyBytes := int64(batchMaxBytes) + c.conf.MaxMessageBytes + 1024
	if err := decodeDeliverBody(io.LimitReader(r.Body, maxBodyBytes), int(c.conf.MaxMessageBytes), c.deliver); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// IsLeader reports whether this node currently holds singleton-job leadership.
func (c *meshCluster) IsLeader() bool {
	return c.leader.IsLeader()
}

// Close stops the mesh: it deregisters this node, releases leadership, stops all peer workers,
// and waits for them to exit.
func (c *meshCluster) Close() error {
	c.cancel() // Stops the heartbeat loop and aborts in-flight peer deliveries
	// Close the peer queues so their workers flush and exit; final sends are best-effort since
	// the context is already canceled (parity with fire-and-forget delivery)
	c.mu.Lock()
	for nodeID, q := range c.queues {
		q.queue.Close()
		delete(c.queues, nodeID)
	}
	c.mu.Unlock()
	if err := c.registry.Deregister(); err != nil {
		log.Tag(tag).Err(err).Warn("Failed to deregister node")
	}
	c.leader.Release()
	metrics.ClusterLeader.Set(0)
	c.wg.Wait()
	return nil
}
