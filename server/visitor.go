package server

import (
	"golang.org/x/time/rate"
	"heckel.io/ntfy/util"
	"sync"
	"time"
)

const (
	visitorExpungeAfter = 30 * time.Minute
)

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	config        *Config
	limiter       *rate.Limiter
	subscriptions *util.Limiter
	seen          time.Time
	mu            sync.Mutex
}

func newVisitor(conf *Config) *visitor {
	return &visitor{
		config:        conf,
		limiter:       rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst),
		subscriptions: util.NewLimiter(int64(conf.VisitorSubscriptionLimit)),
		seen:          time.Now(),
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
	if err := v.subscriptions.Add(1); err != nil {
		return errHTTPTooManyRequests
	}
	return nil
}

func (v *visitor) RemoveSubscription() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptions.Sub(1)
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitor) Stale() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return time.Since(v.seen) > visitorExpungeAfter
}
