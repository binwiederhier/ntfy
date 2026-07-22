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
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	meshHTTPTimeout = 5 * time.Second
	peerQueueSize   = 1024 // Bounded per-peer fan-out queue (drop on overflow)
	tag             = "cluster"
)

// meshCluster fans messages out directly to peer nodes over HTTP (the data plane), using
// PostgreSQL only as a control plane: the node_registry table for membership/discovery, and a
// Postgres advisory lock for singleton-job leader election. Fan-out never touches the database on
// the message path (only the cached peer list does). See plans/260715-scale-out-mesh.md.
//
// Each peer has its own bounded send queue and delivery worker, so a slow or wedged peer only
// backs up (and eventually drops) its own queue and never delays delivery to healthy peers.
type meshCluster struct {
	conf       Config
	deliver    DeliverFunc
	registry   *registry
	leader     *leader
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
func newMeshCluster(conf Config, pool *db.DB, deliver DeliverFunc) (*meshCluster, error) {
	registry, err := newRegistry(pool, conf.NodeID, conf.AdvertiseURL, conf.NodeTTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &meshCluster{
		conf:       conf,
		deliver:    deliver,
		registry:   registry,
		leader:     newLeader(pool),
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
	c.leader.tryAcquire(c.ctx)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.registry.register(); err != nil {
				log.Tag(tag).Err(err).Warn("Failed to refresh node registry heartbeat")
			}
			c.leader.tryAcquire(c.ctx)
			if c.leader.isLeader() {
				if err := c.registry.prune(); err != nil {
					log.Tag(tag).Err(err).Warn("Failed to prune stale nodes")
				}
			}
			if peers, err := c.registry.livePeers(); err == nil {
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
			q.url = fanoutURL(url)
			q.mu.Unlock()
		} else {
			close(q.ch) // Worker drains the remainder and exits
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
		url: fanoutURL(p.advertiseURL),
		ch:  make(chan []byte, peerQueueSize),
	}
	c.queues[p.nodeID] = q
	c.wg.Add(1)
	go c.peerWorker(p.nodeID, q)
	return q
}

// Broadcast enqueues the message for delivery to every live peer node. Delivery is fire-and-forget
// via each peer's bounded queue; if a peer's queue is full the message is dropped for that peer
// (subscribers reconnect and re-poll history from the database).
func (c *meshCluster) Broadcast(msg *model.Message) error {
	payload, err := marshalEnvelope(c.conf.NodeID, msg)
	if err != nil {
		return err
	}
	peers, err := c.registry.livePeers()
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
		q := c.queueFor(p)
		select {
		case q.ch <- payload:
		default:
			metrics.ClusterQueueDropped.Inc()
			log.Tag(tag).Warn("Fan-out queue for peer %s full, dropping message %s", p.nodeID, msg.ID)
		}
	}
	return nil
}

// peerWorker delivers queued fan-out payloads to a single peer until the queue is closed (peer
// left the registry) or the mesh shuts down.
func (c *meshCluster) peerWorker(nodeID string, q *peerQueue) {
	defer c.wg.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case payload, ok := <-q.ch:
			if !ok {
				return
			}
			c.deliverToPeer(nodeID, q.fanoutURL(), payload)
		}
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
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(secretHeader, c.conf.Secret)
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

// ServeFanout handles an inbound fan-out request from a peer node. It authenticates via the
// shared cluster secret (constant-time compare, rejected before any parsing), and delivers
// message envelopes that originated on other nodes to local subscribers. Envelopes of unknown
// kinds are ignored with a 200 response, so newer nodes can introduce new envelope kinds without
// breaking mixed-version clusters during rolling deploys.
func (c *meshCluster) ServeFanout(w http.ResponseWriter, r *http.Request) {
	if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, c.conf.MaxMessageBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	env, err := unmarshalEnvelope(body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if env.Kind == envelopeKindMessage && env.Origin != c.conf.NodeID {
		c.deliver(env.Message)
	}
	w.WriteHeader(http.StatusOK)
}

// IsLeader reports whether this node currently holds singleton-job leadership.
func (c *meshCluster) IsLeader() bool {
	return c.leader.isLeader()
}

// Close stops the mesh: it deregisters this node, releases leadership, stops all peer workers,
// and waits for them to exit.
func (c *meshCluster) Close() error {
	c.cancel()
	if err := c.registry.deregister(); err != nil {
		log.Tag(tag).Err(err).Warn("Failed to deregister node")
	}
	c.leader.release()
	c.wg.Wait()
	return nil
}
