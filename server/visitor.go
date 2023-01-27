package server

import (
	"errors"
	"fmt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/util"
)

const (
	// oneDay is an approximation of a day as a time.Duration
	oneDay = 24 * time.Hour

	// visitorExpungeAfter defines how long a visitor is active before it is removed from memory. This number
	// has to be very high to prevent e-mail abuse, but it doesn't really affect the other limits anyway, since
	// they are replenished faster (typically).
	visitorExpungeAfter = oneDay

	// visitorDefaultReservationsLimit is the amount of topic names a user without a tier is allowed to reserve.
	// This number is zero, and changing it may have unintended consequences in the web app, or otherwise
	visitorDefaultReservationsLimit = int64(0)
)

// Constants used to convert a tier-user's MessageLimit (see user.Tier) into adequate request limiter
// values (token bucket).
//
// Example: Assuming a user.Tier's MessageLimit is 10,000:
// - the allowed burst is 500 (= 10,000 * 5%), which is < 1000 (the max)
// - the replenish rate is 2 * 10,000 / 24 hours
const (
	visitorMessageToRequestLimitBurstRate       = 0.05
	visitorMessageToRequestLimitBurstMax        = 1000
	visitorMessageToRequestLimitReplenishFactor = 2
)

// Constants used to convert a tier-user's EmailLimit (see user.Tier) into adequate email limiter
// values (token bucket). Example: Assuming a user.Tier's EmailLimit is 200, the allowed burst is
// 40 (= 200 * 20%), which is <150 (the max).
const (
	visitorEmailLimitBurstRate = 0.2
	visitorEmailLimitBurstMax  = 150
)

var (
	errVisitorLimitReached = errors.New("limit reached")
)

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	config              *Config
	messageCache        *messageCache
	userManager         *user.Manager      // May be nil
	ip                  netip.Addr         // Visitor IP address
	user                *user.User         // Only set if authenticated user, otherwise nil
	emails              int64              // Number of emails sent, reset every day
	requestLimiter      *rate.Limiter      // Rate limiter for (almost) all requests (including messages)
	messagesLimiter     *util.FixedLimiter // Rate limiter for messages
	emailsLimiter       *rate.Limiter      // Rate limiter for emails
	subscriptionLimiter util.Limiter       // Fixed limiter for active subscriptions (ongoing connections)
	bandwidthLimiter    util.Limiter       // Limiter for attachment bandwidth downloads
	accountLimiter      *rate.Limiter      // Rate limiter for account creation, may be nil
	firebase            time.Time          // Next allowed Firebase message
	seen                time.Time          // Last seen time of this visitor (needed for removal of stale visitors)
	mu                  sync.Mutex
}

type visitorInfo struct {
	Limits *visitorLimits
	Stats  *visitorStats
}

type visitorLimits struct {
	Basis                    visitorLimitBasis
	RequestLimitBurst        int
	RequestLimitReplenish    rate.Limit
	MessageLimit             int64
	MessageExpiryDuration    time.Duration
	EmailLimit               int64
	EmailLimitBurst          int
	EmailLimitReplenish      rate.Limit
	ReservationsLimit        int64
	AttachmentTotalSizeLimit int64
	AttachmentFileSizeLimit  int64
	AttachmentExpiryDuration time.Duration
	AttachmentBandwidthLimit int64
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
	var messages, emails int64
	if user != nil {
		messages = user.Stats.Messages
		emails = user.Stats.Emails
	}
	v := &visitor{
		config:              conf,
		messageCache:        messageCache,
		userManager:         userManager, // May be nil
		ip:                  ip,
		user:                user,
		emails:              emails,
		firebase:            time.Unix(0, 0),
		seen:                time.Now(),
		subscriptionLimiter: util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		requestLimiter:      nil, // Set in resetLimiters
		messagesLimiter:     nil, // Set in resetLimiters, may be nil
		emailsLimiter:       nil, // Set in resetLimiters
		bandwidthLimiter:    nil, // Set in resetLimiters
		accountLimiter:      nil, // Set in resetLimiters, may be nil
	}
	v.resetLimiters(messages)
	return v
}

func (v *visitor) String() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stringNoLock()
}

func (v *visitor) stringNoLock() string {
	if v.user != nil && v.user.Billing.StripeCustomerID != "" {
		return fmt.Sprintf("%s/%s/%s", v.ip.String(), v.user.ID, v.user.Billing.StripeCustomerID)
	} else if v.user != nil {
		return fmt.Sprintf("%s/%s", v.ip.String(), v.user.ID)
	}
	return v.ip.String()
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
	if v.messagesLimiter.Allow(1) != nil {
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

func (v *visitor) AccountCreationAllowed() error {
	if v.accountLimiter == nil || (v.accountLimiter != nil && !v.accountLimiter.Allow()) {
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

func (v *visitor) IncrementEmails() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emails++
}

func (v *visitor) Stats() *user.Stats {
	v.mu.Lock()
	defer v.mu.Unlock()
	return &user.Stats{
		Messages: v.messagesLimiter.Value(),
		Emails:   v.emails,
	}
}

func (v *visitor) ResetStats() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.emails = 0
	v.messagesLimiter.Reset()
}

// SetUser sets the visitors user to the given value
func (v *visitor) SetUser(u *user.User) {
	v.mu.Lock()
	defer v.mu.Unlock()
	shouldResetLimiters := v.user.TierID() != u.TierID() // TierID works with nil receiver
	v.user = u
	if shouldResetLimiters {
		v.resetLimiters(0)
	}
}

// MaybeUserID returns the user ID of the visitor (if any). If this is an anonymous visitor,
// an empty string is returned.
func (v *visitor) MaybeUserID() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.user != nil {
		return v.user.ID
	}
	return ""
}

func (v *visitor) resetLimiters(messages int64) {
	log.Debug("%s Resetting limiters for visitor", v.stringNoLock())
	limits := v.limitsNoLock()
	v.requestLimiter = rate.NewLimiter(limits.RequestLimitReplenish, limits.RequestLimitBurst)
	v.messagesLimiter = util.NewFixedLimiterWithValue(limits.MessageLimit, messages)
	v.emailsLimiter = rate.NewLimiter(limits.EmailLimitReplenish, limits.EmailLimitBurst)
	v.bandwidthLimiter = util.NewBytesLimiter(int(limits.AttachmentBandwidthLimit), oneDay)
	if v.user == nil {
		v.accountLimiter = rate.NewLimiter(rate.Every(v.config.VisitorAccountCreationLimitReplenish), v.config.VisitorAccountCreationLimitBurst)
	} else {
		v.accountLimiter = nil // Users cannot create accounts when logged in
	}
}

func (v *visitor) Limits() *visitorLimits {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.limitsNoLock()
}

func (v *visitor) limitsNoLock() *visitorLimits {
	if v.user != nil && v.user.Tier != nil {
		return tierBasedVisitorLimits(v.config, v.user.Tier)
	}
	return configBasedVisitorLimits(v.config)
}

func tierBasedVisitorLimits(conf *Config, tier *user.Tier) *visitorLimits {
	return &visitorLimits{
		Basis:                    visitorLimitBasisTier,
		RequestLimitBurst:        util.MinMax(int(float64(tier.MessageLimit)*visitorMessageToRequestLimitBurstRate), conf.VisitorRequestLimitBurst, visitorMessageToRequestLimitBurstMax),
		RequestLimitReplenish:    dailyLimitToRate(tier.MessageLimit * visitorMessageToRequestLimitReplenishFactor),
		MessageLimit:             tier.MessageLimit,
		MessageExpiryDuration:    tier.MessageExpiryDuration,
		EmailLimit:               tier.EmailLimit,
		EmailLimitBurst:          util.MinMax(int(float64(tier.EmailLimit)*visitorEmailLimitBurstRate), conf.VisitorEmailLimitBurst, visitorEmailLimitBurstMax),
		EmailLimitReplenish:      dailyLimitToRate(tier.EmailLimit),
		ReservationsLimit:        tier.ReservationLimit,
		AttachmentTotalSizeLimit: tier.AttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  tier.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: tier.AttachmentExpiryDuration,
		AttachmentBandwidthLimit: tier.AttachmentBandwidthLimit,
	}
}

func configBasedVisitorLimits(conf *Config) *visitorLimits {
	messagesLimit := replenishDurationToDailyLimit(conf.VisitorRequestLimitReplenish) // Approximation!
	if conf.VisitorMessageDailyLimit > 0 {
		messagesLimit = int64(conf.VisitorMessageDailyLimit)
	}
	return &visitorLimits{
		Basis:                    visitorLimitBasisIP,
		RequestLimitBurst:        conf.VisitorRequestLimitBurst,
		RequestLimitReplenish:    rate.Every(conf.VisitorRequestLimitReplenish),
		MessageLimit:             messagesLimit,
		MessageExpiryDuration:    conf.CacheDuration,
		EmailLimit:               replenishDurationToDailyLimit(conf.VisitorEmailLimitReplenish), // Approximation!
		EmailLimitBurst:          conf.VisitorEmailLimitBurst,
		EmailLimitReplenish:      rate.Every(conf.VisitorEmailLimitReplenish),
		ReservationsLimit:        visitorDefaultReservationsLimit,
		AttachmentTotalSizeLimit: conf.VisitorAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  conf.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: conf.AttachmentExpiryDuration,
		AttachmentBandwidthLimit: conf.VisitorAttachmentDailyBandwidthLimit,
	}
}

func (v *visitor) Info() (*visitorInfo, error) {
	v.mu.Lock()
	messages := v.messagesLimiter.Value()
	emails := v.emails
	v.mu.Unlock()
	var attachmentsBytesUsed int64
	var err error
	if v.user != nil {
		attachmentsBytesUsed, err = v.messageCache.AttachmentBytesUsedByUser(v.user.ID)
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
		MessagesRemaining:            zeroIfNegative(limits.MessageLimit - messages),
		Emails:                       emails,
		EmailsRemaining:              zeroIfNegative(limits.EmailLimit - emails),
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
	return int64(oneDay / duration)
}

func dailyLimitToRate(limit int64) rate.Limit {
	return rate.Limit(limit) * rate.Every(oneDay)
}

func visitorID(ip netip.Addr, u *user.User) string {
	if u != nil && u.Tier != nil {
		return fmt.Sprintf("user:%s", u.ID)
	}
	return fmt.Sprintf("ip:%s", ip.String())
}
