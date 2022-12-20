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
	Basis                        string // "ip", "role" or "plan"
	Messages                     int64
	MessagesLimit                int64
	MessagesRemaining            int64
	Emails                       int64
	EmailsLimit                  int64
	EmailsRemaining              int64
	AttachmentTotalSize          int64
	AttachmentTotalSizeLimit     int64
	AttachmentTotalSizeRemaining int64
	AttachmentFileSizeLimit      int64
}

func newVisitor(conf *Config, messageCache *messageCache, ip netip.Addr, user *auth.User) *visitor {
	var requestLimiter, emailsLimiter *rate.Limiter
	if user != nil && user.Plan != nil {
		requestLimiter = rate.NewLimiter(dailyLimitToRate(user.Plan.MessagesLimit), conf.VisitorRequestLimitBurst)
		emailsLimiter = rate.NewLimiter(dailyLimitToRate(user.Plan.EmailsLimit), conf.VisitorEmailLimitBurst)
	} else {
		requestLimiter = rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst)
		emailsLimiter = rate.NewLimiter(rate.Every(conf.VisitorEmailLimitReplenish), conf.VisitorEmailLimitBurst)
	}
	return &visitor{
		config:              conf,
		messageCache:        messageCache,
		ip:                  ip,
		user:                user,
		messages:            0, // TODO
		emails:              0, // TODO
		requestLimiter:      requestLimiter,
		emailsLimiter:       emailsLimiter,
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
	v.mu.Lock()
	messages := v.messages
	emails := v.emails
	v.mu.Unlock()
	stats := &visitorStats{}
	if v.user != nil && v.user.Role == auth.RoleAdmin {
		stats.Basis = "role"
		stats.MessagesLimit = 0
		stats.EmailsLimit = 0
		stats.AttachmentTotalSizeLimit = 0
		stats.AttachmentFileSizeLimit = 0
	} else if v.user != nil && v.user.Plan != nil {
		stats.Basis = "plan"
		stats.MessagesLimit = v.user.Plan.MessagesLimit
		stats.EmailsLimit = v.user.Plan.EmailsLimit
		stats.AttachmentTotalSizeLimit = v.user.Plan.AttachmentTotalSizeLimit
		stats.AttachmentFileSizeLimit = v.user.Plan.AttachmentFileSizeLimit
	} else {
		stats.Basis = "ip"
		stats.MessagesLimit = replenishDurationToDailyLimit(v.config.VisitorRequestLimitReplenish)
		stats.EmailsLimit = replenishDurationToDailyLimit(v.config.VisitorEmailLimitReplenish)
		stats.AttachmentTotalSizeLimit = v.config.VisitorAttachmentTotalSizeLimit
		stats.AttachmentFileSizeLimit = v.config.AttachmentFileSizeLimit
	}
	var attachmentsBytesUsed int64
	var err error
	if v.user != nil {
		attachmentsBytesUsed, err = v.messageCache.AttachmentBytesUsedByUser(v.user.Name)
	} else {
		attachmentsBytesUsed, err = v.messageCache.AttachmentBytesUsedBySender(v.ip.String())
	}
	if err != nil {
		return nil, err
	}
	stats.Messages = messages
	stats.MessagesRemaining = zeroIfNegative(stats.MessagesLimit - stats.MessagesLimit)
	stats.Emails = emails
	stats.EmailsRemaining = zeroIfNegative(stats.EmailsLimit - stats.EmailsRemaining)
	stats.AttachmentTotalSize = attachmentsBytesUsed
	stats.AttachmentTotalSizeRemaining = zeroIfNegative(stats.AttachmentTotalSizeLimit - stats.AttachmentTotalSize)
	return stats, nil
}

func zeroIfNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func replenishDurationToDailyLimit(duration time.Duration) int64 {
	return int64(24 * time.Hour / duration)
}

func dailyLimitToRate(limit int64) rate.Limit {
	return rate.Limit(limit) * rate.Every(24*time.Hour)
}
