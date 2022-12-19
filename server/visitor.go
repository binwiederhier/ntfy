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
	config              *Config
	messageCache        *messageCache
	ip                  netip.Addr
	user                *auth.User
	messages            int64
	emails              int64
	requestLimiter      *rate.Limiter
	emailsLimiter       *rate.Limiter
	subscriptionLimiter util.Limiter
	bandwidthLimiter    util.Limiter
	firebase            time.Time // Next allowed Firebase message
	seen                time.Time
	mu                  sync.Mutex
}

type visitorStats struct {
	Messages        int64
	Emails          int64
	AttachmentBytes int64
}

func newVisitor(conf *Config, messageCache *messageCache, ip netip.Addr, user *auth.User) *visitor {
	var requestLimiter *rate.Limiter
	if user != nil && user.Plan != nil {
		requestLimiter = rate.NewLimiter(rate.Limit(user.Plan.MessageLimit)*rate.Every(24*time.Hour), conf.VisitorRequestLimitBurst)
	} else {
		requestLimiter = rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst)
	}
	return &visitor{
		config:              conf,
		messageCache:        messageCache,
		ip:                  ip,
		user:                user,
		messages:            0, // TODO
		emails:              0, // TODO
		requestLimiter:      requestLimiter,
		emailsLimiter:       rate.NewLimiter(rate.Every(conf.VisitorEmailLimitReplenish), conf.VisitorEmailLimitBurst),
		subscriptionLimiter: util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		bandwidthLimiter:    util.NewBytesLimiter(conf.VisitorAttachmentDailyBandwidthLimit, 24*time.Hour),
		firebase:            time.Unix(0, 0),
		seen:                time.Now(),
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
	if !v.emailsLimiter.Allow() {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) SubscriptionAllowed() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.subscriptionLimiter.Allow(1); err != nil {
		return errVisitorLimitReached
	}
	return nil
}

func (v *visitor) RemoveSubscription() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.subscriptionLimiter.Allow(-1)
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitor) BandwidthLimiter() util.Limiter {
	return v.bandwidthLimiter
}

func (v *visitor) Stale() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return time.Since(v.seen) > visitorExpungeAfter
}

func (v *visitor) IncrMessages() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.messages++
}

func (v *visitor) IncrEmails() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emails++
}

func (v *visitor) Stats() (*visitorStats, error) {
	attachmentsBytesUsed, err := v.messageCache.AttachmentBytesUsed(v.ip.String())
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return &visitorStats{
		Messages:        v.messages,
		Emails:          v.emails,
		AttachmentBytes: attachmentsBytesUsed,
	}, nil
}
