package server

import (
	"errors"
	"heckel.io/ntfy/auth"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/util"
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
	config         *Config
	messageCache   *messageCache
	ip             netip.Addr
	user           *auth.User
	requests       *util.AtomicCounter[int64]
	requestLimiter *rate.Limiter
	emails         *rate.Limiter
	subscriptions  util.Limiter
	bandwidth      util.Limiter
	firebase       time.Time // Next allowed Firebase message
	seen           time.Time
	mu             sync.Mutex
}

type visitorStats struct {
	AttachmentFileSizeLimit         int64 `json:"attachmentFileSizeLimit"`
	VisitorAttachmentBytesTotal     int64 `json:"visitorAttachmentBytesTotal"`
	VisitorAttachmentBytesUsed      int64 `json:"visitorAttachmentBytesUsed"`
	VisitorAttachmentBytesRemaining int64 `json:"visitorAttachmentBytesRemaining"`
}

func newVisitor(conf *Config, messageCache *messageCache, ip netip.Addr, user *auth.User) *visitor {
	var requestLimiter *rate.Limiter
	if user != nil && user.Plan != nil {
		requestLimiter = rate.NewLimiter(rate.Limit(user.Plan.RequestLimit)*rate.Every(24*time.Hour), conf.VisitorRequestLimitBurst)
	} else {
		requestLimiter = rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst)
	}
	return &visitor{
		config:         conf,
		messageCache:   messageCache,
		ip:             ip,
		user:           user,
		requests:       util.NewAtomicCounter[int64](0),
		requestLimiter: requestLimiter,
		emails:         rate.NewLimiter(rate.Every(conf.VisitorEmailLimitReplenish), conf.VisitorEmailLimitBurst),
		subscriptions:  util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		bandwidth:      util.NewBytesLimiter(conf.VisitorAttachmentDailyBandwidthLimit, 24*time.Hour),
		firebase:       time.Unix(0, 0),
		seen:           time.Now(),
	}
}

func (v *visitor) RequestAllowed() error {
	if !v.requestLimiter.Allow() {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) FirebaseAllowed() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if time.Now().Before(v.firebase) {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) FirebaseTemporarilyDeny() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.firebase = time.Now().Add(v.config.FirebaseQuotaExceededPenaltyDuration)
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
	attachmentsBytesUsed, err := v.messageCache.AttachmentBytesUsed(v.ip.String())
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
