package server

import (
	"sync"
)

// subscriptionGauge counts the open subscriptions per visitor key on this node. It is never shared
// with other nodes: it protects this node's connections.
type subscriptionGauge struct {
	counts map[string]int64
	mu     sync.Mutex // Protects counts
}

func newSubscriptionGauge() *subscriptionGauge {
	return &subscriptionGauge{
		counts: make(map[string]int64),
	}
}

// Acquire counts one more subscription for the key, unless the key already has limit of them
func (g *subscriptionGauge) Acquire(key string, limit int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.counts[key] >= limit {
		return false
	}
	g.counts[key]++
	return true
}

// Release counts one subscription less for the key, and forgets the key when it has none
func (g *subscriptionGauge) Release(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.counts[key]--
	if g.counts[key] <= 0 {
		delete(g.counts, key)
	}
}

// SubscriptionAllowed takes one of the visitor key's subscription slots on this node, if one is free.
// Every successful call must be paired with RemoveSubscription.
func (v *visitor) SubscriptionAllowed() bool {
	return v.subscriptions.Acquire(v.key, int64(v.config.VisitorSubscriptionLimit))
}

// RemoveSubscription gives back a slot taken by SubscriptionAllowed
func (v *visitor) RemoveSubscription() {
	v.subscriptions.Release(v.key)
}
