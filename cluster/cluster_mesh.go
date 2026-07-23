package cluster

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
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
	meshHTTPTimeout   = 5 * time.Second
	peerQueueSize     = 1024              // Bounded per-peer fan-out queue (drop on overflow)
	batchMaxMessages  = 100               // Flush a batch early when it reaches this many messages
	batchMaxBytes     = 256 * 1024        // Flush a batch early when it reaches this size
	stateMaxBytes     = 4 * 1024 * 1024   // Upper bound for inbound state bodies (filter over ~1M topics)
	stateFilterFPRate = 0.01              // Bloom false-positive rate; a false positive is one wasted send
	leaderLockKey     = int64(0x6e746679) // Advisory-lock key ("ntfy") for the singleton-job leader
	tag               = "cluster"
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
	topics     TopicSource
	registry   *registry
	leader     *pg.Leader
	httpClient *http.Client
	mux        *http.ServeMux // The internal peer API; Cluster is an http.Handler
	mu         sync.Mutex
	queues     map[NodeID]*peerQueue // per-peer send queues; reconciled against the registry
	closed     bool                  // Guards against Broadcast spawning new workers after Close
	statesMu   sync.Mutex
	states     map[NodeID]*peerState // what each peer last told us (subscription knowledge)
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// newMeshCluster creates the mesh cluster: it ensures the registry table exists, registers this
// node, and starts the heartbeat/leader-election loop. Peer delivery workers are started lazily
// as peers appear in the registry.
func newMeshCluster(conf *Config, pool *db.DB, deliver DeliverFunc, topics TopicSource) (*meshCluster, error) {
	if topics == nil {
		topics = func() []string { return nil } // No known topics; peers will broadcast to us
	}
	registry, err := newRegistry(pool, conf.NodeID, conf.AdvertiseURL, conf.NodeTTL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &meshCluster{
		conf:       conf,
		deliver:    deliver,
		topics:     topics,
		registry:   registry,
		leader:     pg.NewLeader(pool.Primary(), leaderLockKey),
		httpClient: &http.Client{Timeout: meshHTTPTimeout},
		queues:     make(map[NodeID]*peerQueue),
		states:     make(map[NodeID]*peerState),
		ctx:        ctx,
		cancel:     cancel,
	}
	c.mux = http.NewServeMux()
	c.mux.HandleFunc("POST "+MessagePath, c.authenticated(c.handleMessage))
	c.mux.HandleFunc("POST "+StatePath, c.authenticated(c.handleState))
	c.wg.Add(1)
	go c.heartbeatLoop()
	return c, nil
}

// ServeHTTP serves the internal peer API. Auth lives in the authenticated middleware, so every
// endpoint gets the same shared-secret and origin handling.
func (c *meshCluster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}

// authenticated wraps a peer API handler with the checks every endpoint needs: the shared
// secret (constant-time compare, rejected before any body is read), a present origin, and the
// origin self-skip (a request carrying this node's own traffic is acknowledged but ignored).
func (c *meshCluster) authenticated(h func(origin NodeID, w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		origin := NodeID(r.Header.Get(originHeader))
		if origin == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if origin == c.conf.NodeID {
			w.WriteHeader(http.StatusOK) // Our own traffic; nothing to do
			return
		}
		h(origin, w, r)
	}
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
	var lastStatePush time.Time // Zero, so the first tick pushes immediately
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
				if time.Since(lastStatePush) >= c.conf.StateInterval {
					c.pushState(peers)
					lastStatePush = time.Now()
				}
			}
		}
	}
}

// reconcileQueues aligns the per-peer queues with the given live peer set: it updates the URLs of
// known peers and stops the queues (and workers) of peers that have left the registry. New queues
// are created lazily by Broadcast, not here, so a freshly joined peer is reachable immediately.
func (c *meshCluster) reconcileQueues(peers []peer) {
	metrics.ClusterPeers.Set(float64(len(peers)))
	alive := make(map[NodeID]string, len(peers)) // node ID -> advertise URL
	for _, p := range peers {
		alive[p.nodeID] = p.advertiseURL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for nodeID, q := range c.queues {
		if url, ok := alive[nodeID]; ok {
			q.mu.Lock()
			q.url = messageURL(url)
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
		url: messageURL(p.advertiseURL),
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
	peers, err := c.registry.LivePeers()
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		return nil // Cluster of one; skip the marshal
	}
	frag, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	metrics.ClusterMessagesBroadcast.Inc()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil // Shutting down; the message is dropped like any other in-flight fan-out
	}
	for _, p := range peers {
		// Route around peers whose fresh state provably excludes this topic; anything less
		// certain (no state, stale state) falls back to broadcasting
		if !c.mayNeed(p.nodeID, msg.Topic) {
			metrics.ClusterRouteSkipped.Inc()
			continue
		}
		if !c.queueFor(p).queue.TryEnqueue(frag) {
			metrics.ClusterQueueDropped.Inc()
			log.Tag(tag).Warn("Fan-out queue for peer %s full, dropping message %s", p.nodeID, msg.ID)
		}
	}
	return nil
}

// mayNeed reports whether the peer may have a subscriber for the topic. Conservative by
// construction: it returns false only when a fresh state snapshot provably excludes the topic.
// A false positive costs one wasted send; a false negative would lose a message and cannot
// happen for topics a peer has reported (Bloom filters have no false negatives).
func (c *meshCluster) mayNeed(peer NodeID, topic string) bool {
	c.statesMu.Lock()
	defer c.statesMu.Unlock()
	state, ok := c.states[peer]
	if !ok || time.Since(state.updatedAt) > 3*c.conf.StateInterval {
		return true // No knowledge, or too old to trust for skipping
	}
	return state.topics.Contains(topic)
}

// peerWorker delivers batches of queued fan-out messages to a single peer. Batches form in the
// peer's LingerQueue (up to BatchLinger delay, flushed early on size/count caps); the worker
// exits when the queue is closed (peer left the registry, or mesh shutdown) and drained.
func (c *meshCluster) peerWorker(nodeID NodeID, q *peerQueue) {
	defer c.wg.Done()
	for frags := range q.queue.Dequeue() {
		c.postToPeer(nodeID, q.MessageURL(), contentTypeNDJSON, assembleMessageBody(frags))
		metrics.ClusterBatchesSent.Inc()
	}
}

// postToPeer POSTs a peer API payload, authenticated with the shared cluster secret. Failures
// are logged and counted, never retried: peer traffic is best-effort by design (messages are
// recovered via since= replay, state via the next periodic push).
func (c *meshCluster) postToPeer(nodeID NodeID, url, contentType string, payload []byte) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Err(err).Warn("Failed to build request for peer %s", nodeID)
		return
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(secretHeader, c.conf.Secret)
	req.Header.Set(originHeader, string(c.conf.NodeID))
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if c.ctx.Err() == nil {
			metrics.ClusterSendErrors.Inc()
			log.Tag(tag).Err(err).Warn("Failed to send to peer %s (%s)", nodeID, url)
		}
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Warn("Peer %s (%s) rejected request with HTTP %d", nodeID, url, resp.StatusCode)
	}
}

// handleMessage receives a batch of peer messages (NDJSON) and streams them to local
// subscribers line by line, delivering each message as it is decoded.
func (c *meshCluster) handleMessage(_ NodeID, w http.ResponseWriter, r *http.Request) {
	// A batch can exceed its byte cap by one message, plus framing overhead
	maxBodyBytes := int64(batchMaxBytes) + c.conf.MaxMessageBytes + 1024
	if err := decodeMessageBody(io.LimitReader(r.Body, maxBodyBytes), int(c.conf.MaxMessageBytes), c.deliver); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleState receives a peer's state envelope and applies each section it carries.
func (c *meshCluster) handleState(origin NodeID, w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, stateMaxBytes))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var state apiState
	if err := json.Unmarshal(body, &state); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if state.Topics != nil {
		if err := c.applyTopicState(origin, state.Topics); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// applyTopicState updates what we know about a peer's subscriptions: a full snapshot replaces
// all prior knowledge, an incremental add merges into it. Increments without a baseline are
// ignored on purpose -- without a snapshot the peer is broadcast to anyway.
func (c *meshCluster) applyTopicState(origin NodeID, topics *apiStateTopics) error {
	c.statesMu.Lock()
	defer c.statesMu.Unlock()
	if len(topics.Filter) > 0 {
		filter, err := util.UnmarshalBloomFilter(topics.Filter)
		if err != nil {
			return err
		}
		c.states[origin] = &peerState{topics: filter, updatedAt: time.Now()}
		return nil
	}
	if state, ok := c.states[origin]; ok {
		for _, topic := range topics.Added {
			state.topics.Add(topic)
		}
		state.updatedAt = time.Now()
	}
	return nil
}

// pushState sends a full state snapshot to every live peer: a Bloom filter over the topics that
// currently have local subscribers. Sent directly (not via the linger queues -- state must not
// wait behind message batches); a lost push self-heals at the next interval. Topics without
// subscribers disappear simply by not being in the next snapshot.
func (c *meshCluster) pushState(peers []peer) {
	if len(peers) == 0 {
		return
	}
	topics := c.topics()
	filter := util.NewBloomFilter(len(topics), stateFilterFPRate)
	for _, topic := range topics {
		filter.Add(topic)
	}
	data, err := filter.MarshalBinary()
	if err != nil {
		return
	}
	body, err := json.Marshal(&apiState{Topics: &apiStateTopics{Filter: data}})
	if err != nil {
		return
	}
	for _, p := range peers {
		go c.postToPeer(p.nodeID, stateURL(p.advertiseURL), contentTypeJSON, body)
	}
	metrics.ClusterStatePushes.Inc()
}

// AnnounceTopics immediately tells all live peers that these topics gained their first local
// subscriber, shrinking the window in which a publisher could wrongly skip this node from a
// full state interval down to about one round trip.
func (c *meshCluster) AnnounceTopics(topics []string) {
	if len(topics) == 0 {
		return
	}
	peers, err := c.registry.LivePeers()
	if err != nil || len(peers) == 0 {
		return
	}
	body, err := json.Marshal(&apiState{Topics: &apiStateTopics{Added: topics}})
	if err != nil {
		return
	}
	for _, p := range peers {
		go c.postToPeer(p.nodeID, stateURL(p.advertiseURL), contentTypeJSON, body)
	}
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
	c.closed = true
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
