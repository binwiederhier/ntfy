package server

import (
	"golang.org/x/time/rate"
	"heckel.io/ntfy/config"
	"sync"
	"time"
)

const (
	visitorExpungeAfter = 30 * time.Minute
)

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	config        *config.Config
	limiter       *rate.Limiter
	subscriptions int
	seen          time.Time
	mu            sync.Mutex
}

func newVisitor(conf *config.Config) *visitor {
	return &visitor{
		config:  conf,
		limiter: rate.NewLimiter(conf.RequestLimit, conf.RequestLimitBurst),
		seen:    time.Now(),
	}
}

func (v *visitor) RequestAllowed() error {
	if !v.limiter.Allow() {
		return errHTTPTooManyRequests
	}
	return nil
}

func (v *visitor) AddSubscription() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.subscriptions >= v.config.SubscriptionLimit {
		return errHTTPTooManyRequests
	}
	v.subscriptions++
	return nil
}

func (v *visitor) RemoveSubscription() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptions--
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitor) Stale() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
	return time.Since(v.seen) > visitorExpungeAfter
}
