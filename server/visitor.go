package server

import (
	"errors"
	"heckel.io/ntfy/user"
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
	userManager         *user.Manager // May be nil!
	ip                  netip.Addr
	user                *user.User
	messages            int64         // Number of messages sent, reset every day
	emails              int64         // Number of emails sent, reset every day
	requestLimiter      *rate.Limiter // Rate limiter for (almost) all requests (including messages)
	messagesLimiter     util.Limiter  // Rate limiter for messages, may be nil
	emailsLimiter       *rate.Limiter // Rate limiter for emails
	subscriptionLimiter util.Limiter  // Fixed limiter for active subscriptions (ongoing connections)
	bandwidthLimiter    util.Limiter  // Limiter for attachment bandwidth downloads
	accountLimiter      *rate.Limiter // Rate limiter for account creation
	firebase            time.Time     // Next allowed Firebase message
	seen                time.Time     // Last seen time of this visitor (needed for removal of stale visitors)
	mu                  sync.Mutex
}

type visitorInfo struct {
	Limits *visitorLimits
	Stats  *visitorStats
}

type visitorLimits struct {
	Basis                    visitorLimitBasis
	MessagesLimit            int64
	MessagesExpiryDuration   time.Duration
	EmailsLimit              int64
	ReservationsLimit        int64
	AttachmentTotalSizeLimit int64
	AttachmentFileSizeLimit  int64
	AttachmentExpiryDuration time.Duration
}

type visitorStats struct {
	Messages                     int64
	MessagesRemaining            int64
	Emails                       int64
	EmailsRemaining              int64
	Reservations                 int64
	ReservationsRemaining        int64
	AttachmentTotalSize          int64
	AttachmentTotalSizeRemaining int64
}

// visitorLimitBasis describes how the visitor limits were derived, either from a user's
// IP address (default config), or from its tier
type visitorLimitBasis string

const (
	visitorLimitBasisIP   = visitorLimitBasis("ip")
	visitorLimitBasisTier = visitorLimitBasis("tier")
)

func newVisitor(conf *Config, messageCache *messageCache, userManager *user.Manager, ip netip.Addr, user *user.User) *visitor {
	var messagesLimiter util.Limiter
	var requestLimiter, emailsLimiter, accountLimiter *rate.Limiter
	var messages, emails int64
	if user != nil {
		messages = user.Stats.Messages
		emails = user.Stats.Emails
	} else {
		accountLimiter = rate.NewLimiter(rate.Every(conf.VisitorAccountCreateLimitReplenish), conf.VisitorAccountCreateLimitBurst)
	}
	if user != nil && user.Tier != nil {
		requestLimiter = rate.NewLimiter(dailyLimitToRate(user.Tier.MessagesLimit), conf.VisitorRequestLimitBurst)
		messagesLimiter = util.NewFixedLimiter(user.Tier.MessagesLimit)
		emailsLimiter = rate.NewLimiter(dailyLimitToRate(user.Tier.EmailsLimit), conf.VisitorEmailLimitBurst)
	} else {
		requestLimiter = rate.NewLimiter(rate.Every(conf.VisitorRequestLimitReplenish), conf.VisitorRequestLimitBurst)
		emailsLimiter = rate.NewLimiter(rate.Every(conf.VisitorEmailLimitReplenish), conf.VisitorEmailLimitBurst)
	}
	return &visitor{
		config:              conf,
		messageCache:        messageCache,
		userManager:         userManager, // May be nil
		ip:                  ip,
		user:                user,
		messages:            messages,
		emails:              emails,
		requestLimiter:      requestLimiter,
		messagesLimiter:     messagesLimiter, // May be nil
		emailsLimiter:       emailsLimiter,
		subscriptionLimiter: util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		bandwidthLimiter:    util.NewBytesLimiter(conf.VisitorAttachmentDailyBandwidthLimit, 24*time.Hour),
		accountLimiter:      accountLimiter, // May be nil
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

func (v *visitor) MessageAllowed() error {
	if v.messagesLimiter != nil && v.messagesLimiter.Allow(1) != nil {
		return errVisitorLimitReached
	}
	return nil
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

func (v *visitor) IncrementMessages() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.messages++
	if v.user != nil {
		v.user.Stats.Messages = v.messages
	}
}

func (v *visitor) IncrementEmails() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emails++
	if v.user != nil {
		v.user.Stats.Emails = v.emails
	}
}

func (v *visitor) ResetStats() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.messages = 0
	v.emails = 0
	if v.user != nil {
		v.user.Stats.Messages = 0
		v.user.Stats.Emails = 0
		// v.messagesLimiter = ... // FIXME
	}
}

func (v *visitor) Limits() *visitorLimits {
	limits := defaultVisitorLimits(v.config)
	if v.user != nil && v.user.Tier != nil {
		limits.Basis = visitorLimitBasisTier
		limits.MessagesLimit = v.user.Tier.MessagesLimit
		limits.MessagesExpiryDuration = v.user.Tier.MessagesExpiryDuration
		limits.EmailsLimit = v.user.Tier.EmailsLimit
		limits.ReservationsLimit = v.user.Tier.ReservationsLimit
		limits.AttachmentTotalSizeLimit = v.user.Tier.AttachmentTotalSizeLimit
		limits.AttachmentFileSizeLimit = v.user.Tier.AttachmentFileSizeLimit
		limits.AttachmentExpiryDuration = v.user.Tier.AttachmentExpiryDuration
	}
	return limits
}

func (v *visitor) Info() (*visitorInfo, error) {
	v.mu.Lock()
	messages := v.messages
	emails := v.emails
	v.mu.Unlock()
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
	var reservations int64
	if v.user != nil && v.userManager != nil {
		reservations, err = v.userManager.ReservationsCount(v.user.Name)
		if err != nil {
			return nil, err
		}
	}
	limits := v.Limits()
	stats := &visitorStats{
		Messages:                     messages,
		MessagesRemaining:            zeroIfNegative(limits.MessagesLimit - messages),
		Emails:                       emails,
		EmailsRemaining:              zeroIfNegative(limits.EmailsLimit - emails),
		Reservations:                 reservations,
		ReservationsRemaining:        zeroIfNegative(limits.ReservationsLimit - reservations),
		AttachmentTotalSize:          attachmentsBytesUsed,
		AttachmentTotalSizeRemaining: zeroIfNegative(limits.AttachmentTotalSizeLimit - attachmentsBytesUsed),
	}
	return &visitorInfo{
		Limits: limits,
		Stats:  stats,
	}, nil
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

func defaultVisitorLimits(conf *Config) *visitorLimits {
	return &visitorLimits{
		Basis:                    visitorLimitBasisIP,
		MessagesLimit:            replenishDurationToDailyLimit(conf.VisitorRequestLimitReplenish),
		MessagesExpiryDuration:   conf.CacheDuration,
		EmailsLimit:              replenishDurationToDailyLimit(conf.VisitorEmailLimitReplenish),
		ReservationsLimit:        0, // No reservations for anonymous users, or users without a tier
		AttachmentTotalSizeLimit: conf.VisitorAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  conf.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: conf.AttachmentExpiryDuration,
	}
}
