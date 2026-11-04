package server

import (
	"time"

	"heckel.io/ntfy/v2/cluster"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/metrics"
	"heckel.io/ntfy/v2/model"
)

const (
	// requestGossipInterval is how often a node sends its peers the requests it admitted per hot
	// visitor key. A client spread over the nodes gets at most about one burst per node more than
	// its limit before the peers know, once; after that, the cluster admits its configured rate.
	requestGossipInterval = time.Second
)

// deliverFromBus delivers a message received from a peer node (via the cluster) to this
// node's local subscribers. It is the receive-side counterpart to Cluster.ForwardMessage: local
// delivery and all global side effects (Firebase, email, web push, upstream) already ran on the
// origin node, so this only publishes to the local topic, and never re-relays.
//
// A message whose topic has no subscriber here is counted as wasted and not published: publishing
// would keep the idle topic alive (Keepalive), so a node would never expunge a topic the other
// nodes keep publishing to. A subscriber that comes along later gets it from its since= read of
// the cache, or from the tail (which sends what never went out here).
func (s *Server) deliverFromBus(m *model.Message) {
	s.mu.RLock()
	t, ok := s.topics[m.Topic]
	s.mu.RUnlock()
	if !ok || t.SubscribersCount() == 0 {
		metrics.ClusterMessagesWasted.Inc() // Relayed here needlessly: no subscriber here wanted the message
		return
	}
	v := s.visitor(m.Sender, nil)
	if err := t.Publish(v, m); err != nil {
		logvm(v, m).Err(err).Warn("Cluster: unable to deliver fan-out message to local subscribers")
	}
}

// maybeCloseLocalSubscribers closes every subscriber connection on this node. The cluster calls it
// while this node is isolated (lost its registration, peers healthy): peers no longer forward
// to it, so its subscribers would silently receive nothing; they reconnect to a healthy node.
//
// Not while the tail still reads the database: then the subscribers get every message anyway,
// a little late. Heartbeats time out when the database is slow, not only when it is gone, and
// closing tens of thousands of connections then sends them to nodes that are just as slow, as
// a reconnect storm that keeps the database slow.
func (s *Server) maybeCloseLocalSubscribers() {
	if s.tail != nil && time.Since(s.tail.LastRead()) < tailStaleAfter {
		log.Tag(tagTail).Info("Keeping local subscribers although the cluster registration is stale: the tail still reads the database")
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.topics {
		t.CancelAllSubscribers()
	}
}

// runRequestGossip sends the peers the requests admitted here every requestGossipInterval, until
// the server stops
func (s *Server) runRequestGossip() {
	for {
		select {
		case <-time.After(requestGossipInterval):
			s.gossipRequests()
		case <-s.closeChan:
			return
		}
	}
}

// gossipRequests sends the peers the requests this node admitted per hot visitor key (see
// limit.Service.HotRequests), which they charge into their own request buckets, so a client spread
// over the nodes gets its request limit once, not once per node
func (s *Server) gossipRequests() {
	requests := s.limiter.HotRequests()
	if len(requests) == 0 {
		return
	}
	s.cluster.BroadcastState(&cluster.State{
		Requests: requests,
	})
}
