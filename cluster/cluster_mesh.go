package cluster

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	meshHTTPTimeout   = 5 * time.Second
	peerHealthTimeout = 2 * time.Second // Per-peer health probe while this node is unhealthy
	peerQueueSize     = 1024            // Bounded per-peer fan-out queue (drop on overflow)
	gapMaxTopics      = 256             // Beyond this many gapped topics for one peer, report GapAllTopics instead
	batchMaxMessages  = 100             // Flush a batch early when it reaches this many messages
	batchMaxBytes     = 256 * 1024      // Flush a batch early when it reaches this size
	stateMaxBytes     = 1024 * 1024     // Upper bound for inbound state bodies (announcements, cancels)
	tag               = "cluster"
)

// meshCluster fans messages out directly to peer nodes over HTTP (the data plane), using
// PostgreSQL only as a control plane: the node_registry table for membership/discovery, and a
// Postgres advisory lock for singleton-job leader election. The message path never touches the
// database (not even for membership: see registry.Peers).
//
// A published message goes to EVERY live peer, which then drops it unless it has a subscriber
// for the topic. Routing by subscription knowledge was tried and removed: it made delivery
// correctness depend on the ordering of state messages, and at the cluster sizes this is built
// for the saving is a fraction of a few hundred KB/s (see docs/10-cluster.md).
//
// Each peer has its own bounded send queue and delivery worker, so a slow or wedged peer only
// backs up (and eventually drops) its own queue and never delays delivery to healthy peers.
type meshCluster struct {
	conf           *Config
	deliver        DeliverFunc
	registry       *registry.Registry
	httpClient     *http.Client
	mux            *http.ServeMux        // The internal peer API; Cluster is an http.Handler
	queues         map[NodeID]*peerQueue // per-peer send queues; reconciled against the registry
	closed         bool                  // Guards against ForwardMessage spawning new workers after Close
	knownPeers     map[NodeID]string     // Peers seen in the last reconcile, for join/leave logging
	gaps           map[NodeID][]string   // Topics whose messages we could not deliver, per peer; reported and cleared by the heartbeat
	lastRegistered time.Time             // Last successful registry heartbeat, for Healthy
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.Mutex // Protects queues, closed, knownPeers and lastRegistered
}

// newMeshCluster creates the mesh cluster: it sets up the registry schema, registers this node
// (synchronously, so it is discoverable before New returns), and starts the heartbeat loop.
// Peer delivery workers are started lazily as peers appear in the registry.
func newMeshCluster(conf *Config, pool *db.DB, deliver DeliverFunc) (*meshCluster, error) {
	reg, err := registry.New(pool, string(conf.NodeID), conf.AdvertiseURL, conf.NodeTTL)
	if err != nil {
		return nil, err
	}
	// Register synchronously so the node is discoverable before the constructor returns; the
	// heartbeat loop refreshes the registration from here on. The first peer snapshot is taken
	// here too, so fan-out works from the first publish rather than the first heartbeat.
	if err := reg.Register(); err != nil {
		return nil, err
	}
	if _, err := reg.Refresh(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &meshCluster{
		conf:           conf,
		deliver:        deliver,
		registry:       reg,
		httpClient:     &http.Client{Timeout: meshHTTPTimeout},
		queues:         make(map[NodeID]*peerQueue),
		lastRegistered: time.Now(), // The synchronous Register above just succeeded
		knownPeers:     make(map[NodeID]string),
		gaps:           make(map[NodeID][]string),
		ctx:            ctx,
		cancel:         cancel,
	}
	c.mux = http.NewServeMux()
	c.mux.HandleFunc("POST "+MessagePath, c.authenticated(c.handleMessage))
	c.mux.HandleFunc("POST "+StatePath, c.authenticated(c.handleState))
	c.mux.HandleFunc("GET "+MembersPath, c.secretAuthenticated(c.handleMembers))
	c.wg.Add(2)
	go c.heartbeatLoop()
	go c.isolationLoop()
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

// secretAuthenticated wraps a handler with the shared-secret check only. Unlike authenticated
// it expects no origin node, because the callers are the load balancers' agents, not peers.
func (c *meshCluster) secretAuthenticated(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c.conf.Secret == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(secretHeader)), []byte(c.conf.Secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// handleMembers lists the live cluster members for the load balancers' agents
func (c *meshCluster) handleMembers(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", contentTypeJSON)
	if err := json.NewEncoder(w).Encode(c.Members()); err != nil {
		log.Tag(tag).Err(err).Warn("Cannot write member list")
	}
}

// Members returns this node plus the peers the registry currently considers live. The peer
// list is the cached registry view, so it is at most one heartbeat stale.
func (c *meshCluster) Members() []Member {
	members := []Member{{NodeID: c.conf.NodeID, AdvertiseURL: c.conf.AdvertiseURL, Healthy: c.Healthy()}}
	for _, p := range c.registry.Peers() {
		members = append(members, Member{NodeID: NodeID(p.NodeID), AdvertiseURL: p.AdvertiseURL, Healthy: true})
	}
	return members
}

// heartbeatLoop runs one heartbeat immediately (the ticker first fires a full interval after
// startup, and a fresh node should be leader-capable and state-visible right away), then one per
// interval until shutdown.
func (c *meshCluster) heartbeatLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.conf.HeartbeatInterval)
	defer ticker.Stop()
	if err := c.heartbeat(); err != nil {
		log.Tag(tag).Err(err).Warn("Cluster heartbeat failed")
	}
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.heartbeat(); err != nil {
				log.Tag(tag).Err(err).Warn("Cluster heartbeat failed")
			}
		}
	}
}

// isolationLoop runs the isolation check on its own ticker: a partitioned database can make
// heartbeat calls hang for a while, and Healthy is time-based, so this still notices
func (c *meshCluster) isolationLoop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.conf.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.maybeIsolated()
		}
	}
}

// maybeIsolated calls IsolatedFunc while this node's registration is stale (peers no longer
// forward to it) but at least one known peer is healthy. With no healthy peer (e.g. a full
// database outage) nothing happens: the mesh keeps delivering on its cached peer view.
func (c *meshCluster) maybeIsolated() {
	if c.conf.IsolatedFunc == nil || c.Healthy() {
		return
	}
	c.mu.Lock()
	urls := make([]string, 0, len(c.knownPeers))
	for _, url := range c.knownPeers {
		urls = append(urls, url)
	}
	c.mu.Unlock()
	for _, url := range urls {
		if c.peerHealthy(url) {
			log.Tag(tag).Warn("This node lost its cluster registration while peer %s is healthy; closing local subscribers so they reconnect elsewhere", url)
			c.conf.IsolatedFunc()
			return
		}
	}
}

func (c *meshCluster) peerHealthy(url string) bool {
	ctx, cancel := context.WithTimeout(c.ctx, peerHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+HealthPath, nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// heartbeat is one control-plane tick: refresh this node's registry row, retry/confirm the
// leader lock, prune long-dead registry rows (as leader), and reconcile the per-peer queues.
//
// A node that cannot even register itself aborts the tick: the remaining database work would
// fail against the same database, and everything downstream degrades safely without it --
// ForwardMessage keeps serving the last peer snapshot.
func (c *meshCluster) heartbeat() error {
	if err := c.registry.Register(); err != nil {
		return err
	}
	c.mu.Lock()
	c.lastRegistered = time.Now()
	c.mu.Unlock()
	// Effective leadership: the lowest live node id, with a hold-off that guarantees a
	// no-leader gap on failover rather than two leaders (see registry.IsLeader)
	if c.registry.IsLeader() {
		metrics.ClusterLeader.Set(1)
		if err := c.registry.Prune(); err != nil {
			log.Tag(tag).Err(err).Warn("Failed to prune stale nodes") // Housekeeping only; not fatal for the tick
		}
	} else {
		metrics.ClusterLeader.Set(0)
	}
	// A fresh read (not the cached view): heartbeat cadence bounds how long a newly joined
	// peer can go unseen; ForwardMessage keeps using the cache on the hot path
	peers, err := c.registry.Refresh()
	if err != nil {
		return err
	}
	c.reconcilePeers(peers)
	c.reportGaps(peers)
	return nil
}

// reconcilePeers aligns this node's per-peer attachments with the live peer set: it retires the
// queues (and workers) of peers that have left the registry or re-registered under a new
// advertise URL (the retired queue's remainder was headed for a dead address anyway). New and
// replacement queues are created lazily by ForwardMessage, not here, so a freshly joined peer is
// reachable immediately.
func (c *meshCluster) reconcilePeers(peers []*registry.Peer) {
	metrics.ClusterPeers.Set(float64(len(peers)))
	alive := make(map[NodeID]string, len(peers)) // node ID -> advertise URL
	for _, p := range peers {
		alive[NodeID(p.NodeID)] = p.AdvertiseURL
	}
	c.mu.Lock()
	// Log joins and leaves (as seen through the up-to-NodeTTL-stale registry view)
	for nodeID, url := range alive {
		if _, ok := c.knownPeers[nodeID]; !ok {
			log.Tag(tag).Info("Peer %s (%s) joined the cluster", nodeID, url)
		}
	}
	for nodeID := range c.knownPeers {
		if _, ok := alive[nodeID]; !ok {
			log.Tag(tag).Info("Peer %s left the cluster", nodeID)
		}
	}
	c.knownPeers = alive
	for nodeID, q := range c.queues {
		if url, ok := alive[nodeID]; !ok || q.advertiseURL != url {
			q.queue.Close() // Flushes the remainder; the worker exits when the queue is drained
			delete(c.queues, nodeID)
		}
	}
	c.mu.Unlock()
}

// queueFor returns the send queue for the given peer, creating it (and its delivery worker) if it
// does not exist yet. The caller must hold c.mu.
func (c *meshCluster) queueFor(p *registry.Peer) *peerQueue {
	nodeID := NodeID(p.NodeID)
	q, ok := c.queues[nodeID]
	if ok {
		return q
	}
	q = &peerQueue{
		advertiseURL: p.AdvertiseURL,
		queue: util.NewLingerQueue(peerQueueSize, batchMaxMessages, batchMaxBytes,
			func(f *fragment) int { return len(f.data) }, c.conf.BatchLinger),
	}
	c.queues[nodeID] = q
	c.wg.Add(1)
	go c.peerWorker(nodeID, q)
	return q
}

// ForwardMessage enqueues the message for delivery to every live peer node; a peer without
// subscribers for the topic drops it on arrival. Delivery is fire-and-forget via each peer's
// bounded batching queue; if a peer's queue is full the message is dropped for that peer
// (subscribers reconnect and re-poll history from the database).
func (c *meshCluster) ForwardMessage(msg *model.Message) error {
	peers := c.registry.Peers() // The heartbeat's snapshot; never a database call on the publish path
	if len(peers) == 0 {
		return nil // Cluster of one; skip the marshal
	}
	data, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	frag := &fragment{topic: msg.Topic, data: data}
	metrics.ClusterMessagesForwarded.Inc()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil // Shutting down; the message is dropped like any other in-flight fan-out
	}
	for _, p := range peers {
		if !c.queueFor(p).queue.TryEnqueue(frag) {
			metrics.ClusterQueueDropped.Inc()
			log.Tag(tag).Warn("Fan-out queue for peer %s full, dropping message %s", p.NodeID, msg.ID)
			c.recordGapLocked(NodeID(p.NodeID), []string{msg.Topic})
		} else if ev := log.Tag(tag); ev.IsTrace() {
			ev.Trace("Enqueued message %s (topic %s) for peer %s", msg.ID, msg.Topic, p.NodeID)
		}
	}
	return nil
}

// peerWorker delivers batches of queued fan-out messages to a single peer. Batches form in the
// peer's LingerQueue (up to BatchLinger delay, flushed early on size/count caps); the worker
// exits when the queue is closed (peer left the registry, or mesh shutdown) and drained.
//
// A batch that cannot be delivered becomes a delivery gap for its topics: the peer is told, and
// closes those topics' subscribers so their clients replay the gap via since= (see GapFunc).
func (c *meshCluster) peerWorker(nodeID NodeID, q *peerQueue) {
	defer c.wg.Done()
	for frags := range q.queue.Dequeue() {
		body := assembleMessageBody(frags)
		log.Tag(tag).Debug("Sending batch of %d message(s) (%d bytes) to peer %s", len(frags), len(body), nodeID)
		if err := c.postToPeer(nodeID, messageURL(q.advertiseURL), contentTypeNDJSON, body); err != nil {
			c.recordGap(nodeID, fragmentTopics(frags))
		}
		metrics.ClusterBatchesSent.Inc()
	}
}

// postToPeer POSTs a peer API payload, authenticated with the shared cluster secret. Failures
// are logged, counted and returned; the caller decides what a failure means (a message batch
// becomes a delivery gap, state is retried on the next heartbeat).
func (c *meshCluster) postToPeer(nodeID NodeID, url, contentType string, payload []byte) error {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Err(err).Warn("Failed to build request for peer %s", nodeID)
		return err
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
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Warn("Peer %s (%s) rejected request with HTTP %d", nodeID, url, resp.StatusCode)
		return fmt.Errorf("peer %s rejected request with HTTP %d", nodeID, resp.StatusCode)
	}
	return nil
}

// recordGap remembers that messages for these topics did not reach the peer. The heartbeat
// reports them; until then they accumulate, and a peer with more than gapMaxTopics gapped
// topics is reported as GapAllTopics rather than a list.
func (c *meshCluster) recordGap(nodeID NodeID, topics []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recordGapLocked(nodeID, topics)
}

// recordGapLocked is recordGap for callers already holding c.mu
func (c *meshCluster) recordGapLocked(nodeID NodeID, topics []string) {
	gapped := c.gaps[nodeID]
	if slices.Contains(gapped, GapAllTopics) {
		return
	}
	for _, topic := range topics {
		if !slices.Contains(gapped, topic) {
			gapped = append(gapped, topic)
		}
	}
	if len(gapped) > gapMaxTopics {
		gapped = []string{GapAllTopics}
	}
	c.gaps[nodeID] = gapped
}

// reportGaps tells each live peer which topics we failed to deliver to it, so it can make the
// affected clients replay. A failed report is kept for the next heartbeat: a gap must not be
// forgotten because the peer was briefly unreachable (which is usually why there is a gap).
func (c *meshCluster) reportGaps(peers []*registry.Peer) {
	c.mu.Lock()
	pending := make(map[NodeID][]string, len(c.gaps))
	for nodeID, topics := range c.gaps {
		pending[nodeID] = topics
	}
	c.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	for _, p := range peers {
		nodeID := NodeID(p.NodeID)
		topics, ok := pending[nodeID]
		if !ok || len(topics) == 0 {
			continue
		}
		body, err := json.Marshal(&apiState{Gaps: topics})
		if err != nil {
			continue
		}
		log.Tag(tag).Info("Reporting %d undelivered topic(s) to peer %s", len(topics), nodeID)
		if err := c.postToPeer(nodeID, stateURL(p.AdvertiseURL), contentTypeJSON, body); err != nil {
			continue // Keep them; the next heartbeat tries again
		}
		metrics.ClusterGapsReported.Inc()
		c.mu.Lock()
		if slices.Equal(c.gaps[nodeID], topics) { // Nothing new while we were sending
			delete(c.gaps, nodeID)
		} else {
			c.gaps[nodeID] = subtractTopics(c.gaps[nodeID], topics)
		}
		c.mu.Unlock()
	}
}

// handleMessage receives a batch of peer messages (NDJSON) and streams them to local
// subscribers line by line, delivering each message as it is decoded.
func (c *meshCluster) handleMessage(origin NodeID, w http.ResponseWriter, r *http.Request) {
	// A batch can exceed its byte cap by one message, plus framing overhead
	maxBodyBytes := int64(batchMaxBytes) + c.conf.MaxMessageBytes + 1024
	received := 0
	deliver := func(m *model.Message) {
		received++
		if ev := log.Tag(tag); ev.IsTrace() {
			ev.Trace("Delivering message %s (topic %s) from peer %s", m.ID, m.Topic, origin)
		}
		c.deliver(m)
	}
	if err := decodeMessageBody(io.LimitReader(r.Body, maxBodyBytes), int(c.conf.MaxMessageBytes), deliver); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	log.Tag(tag).Debug("Received batch of %d message(s) from peer %s", received, origin)
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
	if state.Topics != nil && len(state.Topics.Added) > 0 && c.conf.TopicsAddedFunc != nil {
		log.Tag(tag).Debug("Received %d announced topic(s) from peer %s", len(state.Topics.Added), origin)
		c.conf.TopicsAddedFunc(state.Topics.Added)
	}
	if len(state.Gaps) > 0 && c.conf.GapFunc != nil {
		log.Tag(tag).Warn("Peer %s could not deliver messages for %d topic(s); closing their subscribers so clients replay", origin, len(state.Gaps))
		metrics.ClusterGapsReceived.Inc()
		c.conf.GapFunc(state.Gaps)
	}
	if len(state.Cancels) > 0 && c.conf.CancelFunc != nil {
		log.Tag(tag).Debug("Received %d subscriber cancel(s) from peer %s", len(state.Cancels), origin)
		for _, cancel := range state.Cancels {
			c.conf.CancelFunc(cancel)
		}
	}
	w.WriteHeader(http.StatusOK)
}

// BroadcastState tells all live peers about state changes that must not wait: topics that
// gained their first local subscriber (a hint for peers' cached rate-visitor lookups, never
// used for routing) and subscriber cancels.
func (c *meshCluster) BroadcastState(state *State) {
	if len(state.AddedTopics) == 0 && len(state.SubscriberCancels) == 0 {
		return
	}
	peers := c.registry.Peers()
	if len(peers) == 0 {
		return
	}
	envelope := &apiState{Cancels: state.SubscriberCancels}
	if len(state.AddedTopics) > 0 {
		envelope.Topics = &apiStateTopics{Added: state.AddedTopics}
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	log.Tag(tag).Debug("Broadcasting state (%d new topics, %d cancels) to %d peer(s)", len(state.AddedTopics), len(state.SubscriberCancels), len(peers))
	for _, p := range peers {
		go c.postToPeer(NodeID(p.NodeID), stateURL(p.AdvertiseURL), contentTypeJSON, body)
	}
}

// IsLeader reports whether this node currently holds singleton-job leadership.
func (c *meshCluster) IsLeader() bool {
	return c.registry.IsLeader()
}

// Healthy reports whether this node's registry heartbeat is fresh enough that peers still
// forward messages to it (see the Cluster interface for the checker's fail-open duty).
func (c *meshCluster) Healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Since(c.lastRegistered) < c.conf.NodeTTL
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
	// Wait for the loops BEFORE deregistering: an in-flight heartbeat's Register would otherwise
	// re-insert our row right after Deregister deleted it
	c.wg.Wait()
	if err := c.registry.Deregister(); err != nil {
		log.Tag(tag).Err(err).Warn("Failed to deregister node") // Its row goes stale on its own
	}
	metrics.ClusterLeader.Set(0)
	return nil
}
