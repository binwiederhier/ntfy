package server

import (
	"errors"
	"golang.org/x/time/rate"
	"heckel.io/ntfy/util"
	"sync"
	"time"
)

const (
	// visitorExpungeAfter defines how long a visitor is active before it is removed from memory. This number
	// has to be very high to prevent e-mail abuse, but it doesn't really affect the other limits anyway, since
	// they are replenished faster (typically).
	visitorExpungeAfter = 24 * time.Hour
)

var (
	errVisitorLimitReached = errors.New("limit reached")
)

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	config        *Config
	messageCache  *messageCache
	ip            string
	requests      *rate.Limiter
	emails        *rate.Limiter
	subscriptions util.Limiter
	bandwidth     util.Limiter
	seen          time.Time
	mu            sync.Mutex
}

type visitorStats struct {
	AttachmentFileSizeLimit         int64 `json:"attachmentFileSizeLimit"`
	VisitorAttachmentBytesTotal     int64 `json:"visitorAttachmentBytesTotal"`
	VisitorAttachmentBytesUsed      int64 `json:"visitorAttachmentBytesUsed"`
	VisitorAttachmentBytesRemaining int64 `json:"visitorAttachmentBytesRemaining"`
}

func newVisitor(conf *Config, messageCache *messageCache, ip string) *visitor {
	return &visitor{
		config:        conf,
		messageCache:  messageCache,
		ip:            ip,
		requests:      rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst),
		emails:        rate.NewLimiter(rate.Every(conf.VisitorEmailLimitReplenish), conf.VisitorEmailLimitBurst),
		subscriptions: util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		bandwidth:     util.NewBytesLimiter(conf.VisitorAttachmentDailyBandwidthLimit, 24*time.Hour),
		seen:          time.Now(),
	}
}

func (v *visitor) IP() string {
	return v.ip
}

func (v *visitor) RequestAllowed() error {
	if !v.requests.Allow() {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) EmailAllowed() error {
	if !v.emails.Allow() {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) SubscriptionAllowed() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.subscriptions.Allow(1); err != nil {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) RemoveSubscription() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptions.Allow(-1)
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitor) BandwidthLimiter() util.Limiter {
	return v.bandwidth
}

func (v *visitor) Stale() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return time.Since(v.seen) > visitorExpungeAfter
}

func (v *visitor) Stats() (*visitorStats, error) {
	attachmentsBytesUsed, err := v.messageCache.AttachmentBytesUsed(v.ip)
	if err != nil {
		return nil, err
	}
	attachmentsBytesRemaining := v.config.VisitorAttachmentTotalSizeLimit - attachmentsBytesUsed
	if attachmentsBytesRemaining < 0 {
		attachmentsBytesRemaining = 0
	}
	return &visitorStats{
		AttachmentFileSizeLimit:         v.config.AttachmentFileSizeLimit,
		VisitorAttachmentBytesTotal:     v.config.VisitorAttachmentTotalSizeLimit,
		VisitorAttachmentBytesUsed:      attachmentsBytesUsed,
		VisitorAttachmentBytesRemaining: attachmentsBytesRemaining,
	}, nil
}
