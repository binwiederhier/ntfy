package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"heckel.io/ntfy/v2/cluster/registry"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

const (
	peerStatsRetention = 24 * time.Hour  // How long a peer that left stays in Status
	peerHealthTimeout  = 2 * time.Second // Per-peer health probe while this node is unhealthy
	peerQueueSize      = 1024            // Bounded per-peer fan-out queue (drop on overflow)
	batchMaxMessages   = 100             // Flush a batch early when it reaches this many messages
	batchMaxBytes      = 256 * 1024      // Flush a batch early when it reaches this size
	stateMaxBytes      = 1024 * 1024     // Upper bound for inbound state bodies (announcements, cancels)
)

// meshCluster fans messages out directly to peer nodes over HTTP (the data plane), using
// PostgreSQL only as a control plane: the node_registry table is both the membership mechanism
// and, read as "the live active node with the lowest id", the leader election (see
// registry.IsLeader).
// The message path never touches the database, not even for membership: see registry.Peers.
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
	client         *Client
	mux            *http.ServeMux            // The internal peer API; Cluster is an http.Handler
	queues         map[NodeID]*peerQueue     // per-peer send queues; reconciled against the registry
	closed         bool                      // Guards against ForwardMessage spawning new workers after Close
	knownPeers     map[NodeID]*registry.Peer // Peers seen in the last reconcile, for join/leave logging and fencing
	stats          map[NodeID]*peerStats     // Per-peer batch counters for Status; kept after a peer leaves
	lastRegistered time.Time                 // Last successful registry heartbeat, for Healthy
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	maintenance    atomic.Bool // Maintenance mode: this node acts as passive (see SetMaintenance)
	mu             sync.Mutex  // Protects queues, closed, knownPeers, stats and lastRegistered
}

// newMeshCluster creates the mesh cluster: it sets up the registry schema, registers this node
// (synchronously, so it is discoverable before New returns), and starts the heartbeat loop.
// Peer delivery workers are started lazily as peers appear in the registry.
func newMeshCluster(conf *Config, pool *db.DB, deliver DeliverFunc) (*meshCluster, error) {
	reg, err := registry.New(pool, string(conf.NodeID), conf.AdvertiseURL, conf.Role, conf.NodeTTL)
	if err != nil {
		return nil, err
	}
	// Register synchronously so the node is discoverable before the constructor returns; the
	// heartbeat loop refreshes the registration from here on. The first peer snapshot is taken
	// here too, so fan-out works from the first publish rather than the first heartbeat.
	//
	// This does announce the node before anything is listening for peers: the cluster listener
	// binds later, in the server's Run. The window is bounded by process startup (74ms measured
	// on a three-node harness during a rolling restart, one refused batch per peer), it is a
	// fraction of the restart gap during which nothing is listening at all, and a refused batch
	// is recovered by the delivery-gap report that follows it. Binding earlier would shrink the
	// window, not close it.
	if err := reg.Register(); err != nil {
		return nil, err
	}
	peers, err := reg.Refresh()
	if err != nil {
		return nil, err
	}
	if conf.Role == RolePassive && !hasActivePeer(peers) {
		log.Tag(tag).Warn("This node is passive and no active node is live: load balancers checking /v1/ready send it no clients, and the cluster has no leader until an active node joins")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &meshCluster{
		conf:           conf,
		deliver:        deliver,
		registry:       reg,
		client:         NewClient(conf.Secret, conf.NodeID),
		queues:         make(map[NodeID]*peerQueue),
		lastRegistered: time.Now(), // The synchronous Register above just succeeded
		knownPeers:     make(map[NodeID]*registry.Peer),
		stats:          make(map[NodeID]*peerStats),
		ctx:            ctx,
		cancel:         cancel,
	}
	c.mux = http.NewServeMux()
	c.mux.HandleFunc("POST "+MessagePath, c.authenticated(c.handleMessage))
	c.mux.HandleFunc("POST "+StatePath, c.authenticated(c.handleState))
	c.mux.HandleFunc("GET "+MembersPath, c.secretAuthenticated(c.handleMembers))
	c.mux.HandleFunc("GET "+HealthPath, c.secretAuthenticated(c.handleHealth))
	c.mux.HandleFunc("GET "+StatusPath, c.secretAuthenticated(c.handleStatus))
	c.wg.Add(2)
	go c.heartbeatLoop()
	go c.isolationLoop()
	return c, nil
}

// hasActivePeer reports whether any of the peers is active
func hasActivePeer(peers []*registry.Peer) bool {
	for _, p := range peers {
		if p.Role == RoleActive {
			return true
		}
	}
	return false
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

	metrics.ClusterMessagesForwarded.Inc()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil // Shutting down; the message is dropped like any other in-flight fan-out
	}
	for _, p := range peers {
		if !c.queueForNoLock(p).queue.TryEnqueue(data) {
			metrics.ClusterQueueDropped.Inc()
			c.statsForNoLock(NodeID(p.NodeID)).out.Dropped++
			log.Tag(tag).Warn("Fan-out queue for peer %s full, dropping message %s", p.NodeID, msg.ID)
		} else if ev := log.Tag(tag); ev.IsTrace() {
			ev.Trace("Enqueued message %s (topic %s) for peer %s", msg.ID, msg.Topic, p.NodeID)
		}
	}
	return nil
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
		go func(p *registry.Peer) {
			_ = c.sendToPeer(NodeID(p.NodeID), stateURL(p.AdvertiseURL), func() error {
				return c.client.SendState(c.ctx, p.AdvertiseURL, body)
			})
		}(p)
	}
}

// Members returns this node plus the peers the registry currently considers live, for the LB
// agents. A peer's health here is this node's knowledge of it, not a probe: it is healthy if the
// registry view is still fresh, and unknown (reported unhealthy) once this node can no longer
// read the registry. An isolated node therefore reports nobody as healthy, including itself,
// which is what an agent needs to disregard its answer and keep the upstreams it has.
func (c *meshCluster) Members() []Member {
	fresh := c.registry.Fresh()
	members := []Member{{
		NodeID:       c.conf.NodeID,
		AdvertiseURL: c.conf.AdvertiseURL,
		Role:         c.effectiveRole(),
		Healthy:      c.Healthy(),
	}}
	for _, p := range c.registry.Peers() {
		members = append(members, Member{NodeID: NodeID(p.NodeID), AdvertiseURL: p.AdvertiseURL, Role: p.Role, Healthy: fresh})
	}
	return members
}

// Status reports this node's view of the cluster (see Status): every live peer from the registry
// snapshot, plus every peer this node exchanged batches with, each with both directions' counters.
// A link that only works one way shows as failed batches on one side and nothing received on the other.
func (c *meshCluster) Status() *Status {
	now := time.Now()
	status := &Status{
		NodeID:  c.conf.NodeID,
		Role:    c.effectiveRole(),
		Leader:  c.IsLeader(),
		Healthy: c.Healthy(),
		Time:    now.Unix(),
	}
	peers := make(map[NodeID]*PeerStatus)
	for _, p := range c.registry.Peers() {
		peers[NodeID(p.NodeID)] = &PeerStatus{
			NodeID:       NodeID(p.NodeID),
			AdvertiseURL: p.AdvertiseURL,
			Role:         p.Role,
			Live:         true,
			HeartbeatAge: int64(now.Sub(p.HeartbeatAt).Seconds()),
		}
	}
	c.mu.Lock()
	for nodeID, s := range c.stats {
		ps, ok := peers[nodeID]
		if !ok {
			ps = &PeerStatus{NodeID: nodeID} // Left the registry; its counters still tell what happened
			peers[nodeID] = ps
		}
		ps.Out, ps.In, ps.LastSeen = s.out, s.in, s.lastSeen
	}
	for nodeID, q := range c.queues {
		if ps, ok := peers[nodeID]; ok {
			ps.Out.Queued = q.queue.Len()
		}
	}
	c.mu.Unlock()
	status.Peers = make([]*PeerStatus, 0, len(peers))
	for _, ps := range peers {
		status.Peers = append(status.Peers, ps)
	}
	sort.Slice(status.Peers, func(i, j int) bool {
		return status.Peers[i].NodeID < status.Peers[j].NodeID
	})
	return status
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

// SetMaintenance turns maintenance mode on or off: while on, this node acts as passive. It is not
// ready, gives up leadership at once, never fences, and registers as passive at its next
// heartbeat, so its peers stop counting it as a place for clients and the next leader takes over.
// Its configured role (Role) does not change.
func (c *meshCluster) SetMaintenance(on bool) {
	c.maintenance.Store(on)
	c.registry.SetRole(c.effectiveRole())
}

// effectiveRole is the role this node acts in: passive in maintenance mode, else its configured role
func (c *meshCluster) effectiveRole() Role {
	if c.maintenance.Load() {
		return RolePassive
	}
	return c.conf.Role
}

// Role reports this node's configured role
func (c *meshCluster) Role() Role {
	return c.conf.Role
}

// Ready reports whether a load balancer should send clients here (see the Cluster interface). It
// reads the last peer snapshot rather than probing peers, since load balancers call it often.
func (c *meshCluster) Ready() bool {
	if c.effectiveRole() != RoleActive {
		return false
	} else if c.Healthy() {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.knownPeers {
		if p.Role == RoleActive {
			return false // Another active node can take this node's clients
		}
	}
	return true
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

// heartbeat is one control-plane tick: refresh this node's registry row, read the live set back
// (which is also what updates this node's leadership standing), prune long-dead rows if this node
// leads, and reconcile the per-peer queues.
//
// A node that cannot even register itself aborts the tick: the remaining database work would
// fail against the same database, and everything downstream degrades safely without it:
// ForwardMessage keeps serving the last peer snapshot.
func (c *meshCluster) heartbeat() error {
	if err := c.registry.Register(); err != nil {
		return err
	}
	c.mu.Lock()
	c.lastRegistered = time.Now()
	c.mu.Unlock()
	// Effective leadership: the lowest live active node id, with a hold-off that guarantees a
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
	return nil
}

// reconcilePeers aligns this node's per-peer attachments with the live peer set: it retires the
// queues (and workers) of peers that have left the registry or re-registered under a new
// advertise URL (the retired queue's remainder was headed for a dead address anyway). New and
// replacement queues are created lazily by ForwardMessage, not here, so a freshly joined peer is
// reachable immediately.
func (c *meshCluster) reconcilePeers(peers []*registry.Peer) {
	metrics.ClusterPeers.Set(float64(len(peers)))
	alive := make(map[NodeID]*registry.Peer, len(peers))
	for _, p := range peers {
		alive[NodeID(p.NodeID)] = p
	}
	c.mu.Lock()
	// Log joins and leaves (as seen through the up-to-NodeTTL-stale registry view)
	for nodeID, p := range alive {
		if _, ok := c.knownPeers[nodeID]; !ok {
			log.Tag(tag).Info("Peer %s (%s, %s) joined the cluster", nodeID, p.AdvertiseURL, p.Role)
		}
	}
	for nodeID := range c.knownPeers {
		if _, ok := alive[nodeID]; !ok {
			log.Tag(tag).Info("Peer %s left the cluster", nodeID)
		}
	}
	c.knownPeers = alive
	c.updatePeerStatsNoLock(alive)
	for nodeID, q := range c.queues {
		if p, ok := alive[nodeID]; !ok || q.advertiseURL != p.AdvertiseURL {
			q.queue.Close() // Flushes the remainder; the worker exits when the queue is drained
			delete(c.queues, nodeID)
		}
	}
	c.mu.Unlock()
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
// forward to it) but at least one known active peer is healthy.
//
// The peer probe is what makes this safe, and it answers one question: is there somewhere better
// for these clients to go? Closing them only helps if their reconnect lands on a node that still
// receives fan-out. Without the probe, a shared-database outage would make every node decide it
// is unhealthy and disconnect everyone, in a loop, every tick, while the mesh is in fact still
// delivering, because Peers serves the last snapshot and never queries. So with no healthy peer
// this does nothing: here is as good as anywhere, and the alternative is a reconnect storm.
//
// Roles narrow this further. A passive peer is not behind the load balancer, so it is nowhere
// better for clients that reconnect through it. A passive node never fences at all: its
// clients connected to it directly, and nothing would route them elsewhere. Nor does a node in
// maintenance mode: it is draining its clients already, gradually rather than all at once.
func (c *meshCluster) maybeIsolated() {
	if c.conf.IsolatedFunc == nil || c.effectiveRole() == RolePassive || c.Healthy() {
		return
	}
	c.mu.Lock()
	urls := make([]string, 0, len(c.knownPeers))
	for _, p := range c.knownPeers {
		if p.Role == RoleActive {
			urls = append(urls, p.AdvertiseURL)
		}
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
	healthy, err := c.client.Health(ctx, url)
	return err == nil && healthy
}

// queueForNoLock returns the send queue for the given peer, creating it (and its delivery
// worker) if it does not exist yet. The caller must hold c.mu.
func (c *meshCluster) queueForNoLock(p *registry.Peer) *peerQueue {
	nodeID := NodeID(p.NodeID)
	q, ok := c.queues[nodeID]
	if ok {
		return q
	}
	q = &peerQueue{
		advertiseURL: p.AdvertiseURL,
		queue: util.NewLingerQueue(peerQueueSize, batchMaxMessages, batchMaxBytes,
			func(line []byte) int { return len(line) }, c.conf.BatchLinger),
	}
	c.queues[nodeID] = q
	c.wg.Add(1)
	go c.peerWorker(nodeID, q)
	return q
}

// peerWorker delivers batches of queued fan-out messages to a single peer. Batches form in the
// peer's LingerQueue (up to BatchLinger delay, flushed early on size/count caps); the worker
// exits when the queue is closed (peer left the registry, or mesh shutdown) and drained.
//
// A batch that cannot be delivered is dropped: recovering it is the delivery-gap mechanism, which
// is not in this piece yet, so a failed batch is counted and logged and nothing else.
func (c *meshCluster) peerWorker(nodeID NodeID, q *peerQueue) {
	defer c.wg.Done()
	for lines := range q.queue.Dequeue() {
		body := assembleMessageBody(lines)
		log.Tag(tag).Debug("Sending batch of %d message(s) (%d bytes) to peer %s", len(lines), len(body), nodeID)
		err := c.sendToPeer(nodeID, messageURL(q.advertiseURL), func() error {
			return c.client.SendMessages(c.ctx, q.advertiseURL, body)
		})
		if err != nil {
			log.Tag(tag).Err(err).Warn("Batch to peer %s was lost", nodeID)
		}
		if c.ctx.Err() == nil { // A send aborted by shutdown says nothing about the link
			c.recordSent(nodeID, len(lines), err)
		}
		metrics.ClusterBatchesSent.Inc()
	}
}

// recordSent counts a message batch sent to a peer, per peer, for Status and the metrics
func (c *meshCluster) recordSent(nodeID NodeID, messages int, err error) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.statsForNoLock(nodeID)
	if err != nil {
		s.out.BatchesFailed++
		s.out.LastError = err.Error()
		s.out.LastErrorAt = now.Unix()
		s.out.LastFailed = true
		metrics.ClusterPeerBatchesSent.WithLabelValues(string(nodeID), "error").Inc()
		return
	}
	s.out.BatchesOK++
	s.out.Messages += int64(messages)
	s.out.LastOK = now.Unix()
	s.out.LastFailed = false
	metrics.ClusterPeerBatchesSent.WithLabelValues(string(nodeID), "ok").Inc()
	metrics.ClusterPeerLastSendSuccess.WithLabelValues(string(nodeID)).Set(float64(now.Unix()))
}

// recordReceived counts a message batch received from a peer, per peer, for Status and the metrics
func (c *meshCluster) recordReceived(nodeID NodeID, messages int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.statsForNoLock(nodeID)
	s.in.Batches++
	s.in.Messages += int64(messages)
	s.in.LastAt = time.Now().Unix()
	metrics.ClusterPeerBatchesReceived.WithLabelValues(string(nodeID)).Inc()
}

// statsForNoLock returns the counters for the given peer, creating them on first use. The caller
// must hold c.mu.
func (c *meshCluster) statsForNoLock(nodeID NodeID) *peerStats {
	s, ok := c.stats[nodeID]
	if !ok {
		s = &peerStats{lastSeen: time.Now().Unix()} // First contact counts as seen, so it can expire
		c.stats[nodeID] = s
		seedPeerMetrics(nodeID)
	}
	return s
}

// updatePeerStatsNoLock marks the live peers as seen now, and forgets peers gone for longer than
// peerStatsRetention, so a decommissioned node does not stay in Status. The caller must hold c.mu.
func (c *meshCluster) updatePeerStatsNoLock(alive map[NodeID]*registry.Peer) {
	now := time.Now().Unix()
	for nodeID := range alive {
		c.statsForNoLock(nodeID).lastSeen = now
	}
	for nodeID, s := range c.stats {
		if _, ok := alive[nodeID]; !ok && now-s.lastSeen > int64(peerStatsRetention.Seconds()) {
			delete(c.stats, nodeID)
			deletePeerMetrics(nodeID)
		}
	}
}

// seedPeerMetrics creates a new peer's labeled series at zero, so a peer that never succeeds still
// has a last-success sample (0, never) for an alert on its age, and the counters have a baseline
func seedPeerMetrics(nodeID NodeID) {
	metrics.ClusterPeerBatchesSent.WithLabelValues(string(nodeID), "ok")
	metrics.ClusterPeerBatchesSent.WithLabelValues(string(nodeID), "error")
	metrics.ClusterPeerBatchesReceived.WithLabelValues(string(nodeID))
	metrics.ClusterPeerLastSendSuccess.WithLabelValues(string(nodeID))
}

// deletePeerMetrics retires a forgotten peer's labeled series, so departed IDs are not exported forever
func deletePeerMetrics(nodeID NodeID) {
	metrics.ClusterPeerBatchesSent.DeleteLabelValues(string(nodeID), "ok")
	metrics.ClusterPeerBatchesSent.DeleteLabelValues(string(nodeID), "error")
	metrics.ClusterPeerBatchesReceived.DeleteLabelValues(string(nodeID))
	metrics.ClusterPeerLastSendSuccess.DeleteLabelValues(string(nodeID))
}

// sendToPeer runs one peer API POST (via the client, authenticated with the shared cluster
// secret). Failures are logged, counted and returned; the caller decides what a failure means (a
// message batch is dropped, state is retried on the next heartbeat).
func (c *meshCluster) sendToPeer(nodeID NodeID, url string, send func() error) error {
	err := send()
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		metrics.ClusterSendErrors.Inc()
		log.Tag(tag).Warn("Peer %s (%s) rejected request with HTTP %d", nodeID, url, httpErr.StatusCode)
		return fmt.Errorf("peer %s rejected request with HTTP %d", nodeID, httpErr.StatusCode)
	} else if err != nil {
		if c.ctx.Err() == nil {
			metrics.ClusterSendErrors.Inc()
			log.Tag(tag).Err(err).Warn("Failed to send to peer %s (%s)", nodeID, url)
		}
		return err
	}
	return nil
}
