package server

import (
	"fmt"
	"math"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
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

	// visitorDefaultCallsLimit is the amount of calls a user without a tier is allowed to make.
	// This number is zero, because phone numbers have to be verified first.
	visitorDefaultCallsLimit = int64(0)
)

// Constants used to convert a tier-user's MessageSizeLimit (see user.Tier) into adequate request limiter
// values (token bucket). This is only used to increase the values in server.yml, never decrease them.
//
// Example: Assuming a user.Tier's MessageSizeLimit is 10,000:
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

// visitor is the per-request view of a visitorBucket: the rate-limit bucket is shared by all requests from
// the same IP prefix (or the same tier'd user), but the user is the one authenticated for THIS request. The
// user must never live on the shared bucket: IP-keyed buckets are shared by untiered accounts and anonymous
// requests alike, so two overlapping requests from one IP would otherwise observe each other's user and be
// authorized as it (see Server.visitor). Each request gets its own visitor, so its user needs no lock.
type visitor struct {
	*visitorBucket
	user *user.User // Authenticated user of this request, or nil
}

// visitorBucket represents an API user, and its associated rate.Limiter used for rate limiting. It is shared
// by all requests with the same visitorID, and is only ever handed out wrapped in a visitor.
type visitorBucket struct {
	config               *Config
	messageCache         *message.Cache
	userManager          *user.Manager      // May be nil
	ip                   netip.Addr         // Visitor IP address
	user                 *user.User         // Tier'd user the bucket is keyed by (see visitorID); nil for IP-keyed buckets
	requestLimiter       *rate.Limiter      // Rate limiter for (almost) all requests (including messages)
	messagesLimiter      *util.FixedLimiter // Rate limiter for messages
	emailsLimiter        *util.RateLimiter  // Rate limiter for emails
	callsLimiter         *util.FixedLimiter // Rate limiter for calls
	subscriptionLimiter  *util.FixedLimiter // Fixed limiter for active subscriptions (ongoing connections)
	topicCreationLimiter *rate.Limiter      // Rate limiter for inserting new topics into the in-memory topic map
	bandwidthLimiter     *util.RateLimiter  // Limiter for attachment downloads and cached-message replay (polls)
	accountLimiter       *rate.Limiter      // Rate limiter for account actions (signup, password-reset requests), may be nil
	authLimiter          *rate.Limiter      // Limiter for incorrect login attempts, may be nil
	firebase             time.Time          // Next allowed Firebase message
	seen                 time.Time          // Last seen time of this visitor (needed for removal of stale visitors)
	mu                   sync.RWMutex
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
	CallLimit                int64
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
	Calls                        int64
	CallsRemaining               int64
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

// newVisitor creates a new bucket for the given IP and user, and returns this request's view of it
func newVisitor(conf *Config, messageCache *message.Cache, userManager *user.Manager, ip netip.Addr, user *user.User) *visitor {
	return newVisitorBucket(conf, messageCache, userManager, ip, user).visitor(user)
}

// newVisitorBucket creates a bucket keyed by the given IP, or by the user if it has a tier (see visitorID)
func newVisitorBucket(conf *Config, messageCache *message.Cache, userManager *user.Manager, ip netip.Addr, user *user.User) *visitorBucket {
	user = visitorBucketUser(user)
	var messages, emails, calls int64
	if user != nil {
		messages = user.Stats.Messages
		emails = user.Stats.Emails
		calls = user.Stats.Calls
	}
	v := &visitorBucket{
		config:               conf,
		messageCache:         messageCache,
		userManager:          userManager, // May be nil
		ip:                   ip,
		user:                 user,
		firebase:             time.Unix(0, 0),
		seen:                 time.Now(),
		subscriptionLimiter:  util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
		requestLimiter:       nil, // Set in resetLimiters
		messagesLimiter:      nil, // Set in resetLimiters, may be nil
		emailsLimiter:        nil, // Set in resetLimiters
		callsLimiter:         nil, // Set in resetLimiters, may be nil
		topicCreationLimiter: nil, // Set in resetLimiters
		bandwidthLimiter:     nil, // Set in resetLimiters
		accountLimiter:       nil, // Set in resetLimiters, may be nil
		authLimiter:          nil, // Set in resetLimiters, may be nil
	}
	v.resetLimitersNoLock(messages, emails, calls, false)
	return v
}

// visitor returns this request's view of the bucket, carrying the request's own user
func (v *visitorBucket) visitor(u *user.User) *visitor {
	return &visitor{visitorBucket: v, user: u}
}

// Context returns the bucket's log context plus the fields of this request's user
func (v *visitor) Context() log.Context {
	fields := v.visitorBucket.Context()
	if v.user != nil {
		fields["user_id"] = v.user.ID
		fields["user_name"] = v.user.Name
		if v.user.Tier != nil {
			for field, value := range v.user.Tier.Context() {
				fields[field] = value
			}
		}
		if v.user.Billing.StripeCustomerID != "" {
			fields["stripe_customer_id"] = v.user.Billing.StripeCustomerID
		}
		if v.user.Billing.StripeSubscriptionID != "" {
			fields["stripe_subscription_id"] = v.user.Billing.StripeSubscriptionID
		}
	}
	return fields
}

func (v *visitorBucket) Context() log.Context {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.contextNoLock()
}

func (v *visitorBucket) contextNoLock() log.Context {
	info := v.infoLightNoLock()
	fields := log.Context{
		"visitor_id":                 visitorID(v.ip, v.user, v.config),
		"visitor_ip":                 v.ip.String(),
		"visitor_seen":               util.FormatTime(v.seen),
		"visitor_messages":           info.Stats.Messages,
		"visitor_messages_limit":     info.Limits.MessageLimit,
		"visitor_messages_remaining": info.Stats.MessagesRemaining,
		"visitor_requests_remaining": int64(math.Floor(v.requestLimiter.Tokens())),
	}
	if v.config.SMTPSenderFrom != "" {
		fields["visitor_emails"] = info.Stats.Emails
		fields["visitor_emails_limit"] = info.Limits.EmailLimit
		fields["visitor_emails_remaining"] = info.Stats.EmailsRemaining
	}
	if v.config.TwilioAccount != "" {
		fields["visitor_calls"] = info.Stats.Calls
		fields["visitor_calls_limit"] = info.Limits.CallLimit
		fields["visitor_calls_remaining"] = info.Stats.CallsRemaining
	}
	if v.authLimiter != nil {
		fields["visitor_auth_attempts_remaining"] = int64(math.Floor(v.authLimiter.Tokens()))
	}
	if v.topicCreationLimiter != nil {
		fields["visitor_topic_creations_remaining"] = int64(math.Floor(v.topicCreationLimiter.Tokens()))
	}
	return fields
}

func visitorExtendedInfoContext(info *visitorInfo) log.Context {
	return log.Context{
		"visitor_reservations":                    info.Stats.Reservations,
		"visitor_reservations_limit":              info.Limits.ReservationsLimit,
		"visitor_reservations_remaining":          info.Stats.ReservationsRemaining,
		"visitor_attachment_total_size":           info.Stats.AttachmentTotalSize,
		"visitor_attachment_total_size_limit":     info.Limits.AttachmentTotalSizeLimit,
		"visitor_attachment_total_size_remaining": info.Stats.AttachmentTotalSizeRemaining,
	}

}
func (v *visitorBucket) RequestAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.requestLimiter.Allow()
}

func (v *visitorBucket) FirebaseAllowed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return !time.Now().Before(v.firebase)
}

func (v *visitorBucket) FirebaseTemporarilyDeny() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.firebase = time.Now().Add(v.config.FirebaseQuotaExceededPenaltyDuration)
}

func (v *visitorBucket) MessageAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.messagesLimiter.Allow()
}

func (v *visitorBucket) EmailAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.emailsLimiter.Allow()
}

func (v *visitorBucket) CallAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.callsLimiter.Allow()
}

func (v *visitorBucket) SubscriptionAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.subscriptionLimiter.Allow()
}

// TopicCreationAllowed returns true if the visitor is allowed to cause a new topic to be
// inserted into the server's in-memory topic map. Returns true if no limiter is configured.
func (v *visitorBucket) TopicCreationAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.topicCreationLimiter == nil {
		return true
	}
	return v.topicCreationLimiter.Allow()
}

// AuthAllowed returns true if an auth request can be attempted (> 1 token available)
func (v *visitorBucket) AuthAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.authLimiter == nil {
		return true
	}
	return v.authLimiter.Tokens() > 1
}

// AuthFailed records an auth failure
func (v *visitorBucket) AuthFailed() {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.authLimiter != nil {
		v.authLimiter.Allow()
	}
}

// AccountActionAllowed returns true if a rate-limited account action (signup or password-reset
// request) is currently allowed for this visitor
func (v *visitorBucket) AccountActionAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.accountLimiter == nil || (v.accountLimiter != nil && v.accountLimiter.Tokens() < 1) {
		return false
	}
	return true
}

// AccountActionPerformed decreases the account limiter. This is to be called after a rate-limited
// account action (signup or password-reset request).
func (v *visitorBucket) AccountActionPerformed() {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.accountLimiter != nil {
		v.accountLimiter.Allow()
	}
}

func (v *visitorBucket) BandwidthAllowed(bytes int64) bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.bandwidthLimiter.AllowN(bytes)
}

func (v *visitorBucket) RemoveSubscription() {
	v.mu.RLock()
	defer v.mu.RUnlock()
	v.subscriptionLimiter.AllowN(-1)
}

func (v *visitorBucket) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitorBucket) BandwidthLimiter() util.Limiter {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.bandwidthLimiter
}

func (v *visitorBucket) Stale() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return time.Since(v.seen) > visitorExpungeAfter
}

func (v *visitorBucket) Stats() *user.Stats {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return &user.Stats{
		Messages: v.messagesLimiter.Value(),
		Emails:   v.emailsLimiter.Value(),
		Calls:    v.callsLimiter.Value(),
	}
}

func (v *visitorBucket) ResetStats() {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	v.emailsLimiter.Reset()
	v.messagesLimiter.Reset()
	v.callsLimiter.Reset()
}

// User returns the user authenticated for this request, or nil if there is none
func (v *visitor) User() *user.User {
	return v.user // May be nil
}

// IP returns the visitor IP address
func (v *visitorBucket) IP() netip.Addr {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.ip
}

// Authenticated returns true if a user successfully authenticated
func (v *visitor) Authenticated() bool {
	return v.user != nil
}

// SetUser sets the user of this request. It does not touch the shared bucket; only Server.visitor may change
// the tier'd user a bucket is keyed by (see visitorBucket.setUser)
func (v *visitor) SetUser(u *user.User) {
	v.user = u // u may be nil!
}

// setUser updates the tier'd user the bucket is keyed by, resetting the limiters if its tier changed. IP-keyed
// buckets carry no user (see visitorBucketUser), so this is a no-op for them.
func (v *visitorBucket) setUser(u *user.User) {
	v.mu.Lock()
	defer v.mu.Unlock()
	u = visitorBucketUser(u)
	shouldResetLimiters := v.user.TierID() != u.TierID() // TierID works with nil receiver
	v.user = u                                           // u may be nil!
	if shouldResetLimiters {
		var messages, emails, calls int64
		if u != nil {
			messages, emails, calls = u.Stats.Messages, u.Stats.Emails, u.Stats.Calls
		}
		v.resetLimitersNoLock(messages, emails, calls, true)
	}
}

// MaybeUserID returns the user ID of the visitor (if any). If this is an anonymous visitor,
// an empty string is returned.
func (v *visitor) MaybeUserID() string {
	if v.user != nil {
		return v.user.ID
	}
	return ""
}

func (v *visitorBucket) resetLimitersNoLock(messages, emails, calls int64, enqueueUpdate bool) {
	limits := v.limitsNoLock()
	v.requestLimiter = rate.NewLimiter(limits.RequestLimitReplenish, limits.RequestLimitBurst)
	v.messagesLimiter = util.NewFixedLimiterWithValue(limits.MessageLimit, messages)
	v.emailsLimiter = util.NewRateLimiterWithValue(limits.EmailLimitReplenish, limits.EmailLimitBurst, emails)
	v.callsLimiter = util.NewFixedLimiterWithValue(limits.CallLimit, calls)
	if v.config.VisitorTopicCreationLimitBurst > 0 && v.config.VisitorTopicCreationLimitReplenish > 0 {
		v.topicCreationLimiter = rate.NewLimiter(rate.Every(v.config.VisitorTopicCreationLimitReplenish), v.config.VisitorTopicCreationLimitBurst)
	} else {
		v.topicCreationLimiter = nil // Disabled
	}
	v.bandwidthLimiter = util.NewBytesLimiter(int(limits.AttachmentBandwidthLimit), oneDay)
	if v.user == nil {
		v.accountLimiter = rate.NewLimiter(rate.Every(v.config.VisitorAccountCreationLimitReplenish), v.config.VisitorAccountCreationLimitBurst)
		v.authLimiter = rate.NewLimiter(rate.Every(v.config.VisitorAuthFailureLimitReplenish), v.config.VisitorAuthFailureLimitBurst)
	} else {
		v.accountLimiter = nil // Users cannot create accounts when logged in
		v.authLimiter = nil    // Users are already logged in, no need to limit requests
	}
	if enqueueUpdate && v.user != nil {
		go v.userManager.EnqueueUserStats(v.user.ID, &user.Stats{
			Messages: messages,
			Emails:   emails,
			Calls:    calls,
		})
	}
	log.Fields(v.contextNoLock()).Debug("Rate limiters reset for visitor") // Must be after function, because contextNoLock() describes rate limiters
}

func (v *visitorBucket) Limits() *visitorLimits {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.limitsNoLock()
}

func (v *visitorBucket) limitsNoLock() *visitorLimits {
	if v.user != nil && v.user.Tier != nil {
		return tierBasedVisitorLimits(v.config, v.user.Tier)
	}
	return configBasedVisitorLimits(v.config)
}

func tierBasedVisitorLimits(conf *Config, tier *user.Tier) *visitorLimits {
	return &visitorLimits{
		Basis:                    visitorLimitBasisTier,
		RequestLimitBurst:        util.MinMax(int(float64(tier.MessageLimit)*visitorMessageToRequestLimitBurstRate), conf.VisitorRequestLimitBurst, visitorMessageToRequestLimitBurstMax),
		RequestLimitReplenish:    util.Max(rate.Every(conf.VisitorRequestLimitReplenish), dailyLimitToRate(tier.MessageLimit*visitorMessageToRequestLimitReplenishFactor)),
		MessageLimit:             tier.MessageLimit,
		MessageExpiryDuration:    tier.MessageExpiryDuration,
		EmailLimit:               tier.EmailLimit,
		EmailLimitBurst:          util.MinMax(int(float64(tier.EmailLimit)*visitorEmailLimitBurstRate), conf.VisitorEmailLimitBurst, visitorEmailLimitBurstMax),
		EmailLimitReplenish:      dailyLimitToRate(tier.EmailLimit),
		CallLimit:                tier.CallLimit,
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
	var emailLimit int64
	if conf.VisitorEmailLimitBurst > 0 {
		emailLimit = replenishDurationToDailyLimit(conf.VisitorEmailLimitReplenish) // Approximation!
	}
	return &visitorLimits{
		Basis:                    visitorLimitBasisIP,
		RequestLimitBurst:        conf.VisitorRequestLimitBurst,
		RequestLimitReplenish:    rate.Every(conf.VisitorRequestLimitReplenish),
		MessageLimit:             messagesLimit,
		MessageExpiryDuration:    conf.CacheDuration,
		EmailLimit:               emailLimit,
		EmailLimitBurst:          conf.VisitorEmailLimitBurst,
		EmailLimitReplenish:      rate.Every(conf.VisitorEmailLimitReplenish),
		CallLimit:                visitorDefaultCallsLimit,
		ReservationsLimit:        visitorDefaultReservationsLimit,
		AttachmentTotalSizeLimit: conf.VisitorAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:  conf.AttachmentFileSizeLimit,
		AttachmentExpiryDuration: conf.AttachmentExpiryDuration,
		AttachmentBandwidthLimit: conf.VisitorAttachmentDailyBandwidthLimit,
	}
}

func (v *visitor) Info() (*visitorInfo, error) {
	v.visitorBucket.mu.RLock()
	info := v.infoLightNoLock()
	v.visitorBucket.mu.RUnlock()

	// Attachment stats from database
	var attachmentsBytesUsed int64
	var err error
	u := v.User()
	if u != nil {
		attachmentsBytesUsed, err = v.messageCache.AttachmentBytesUsedByUser(u.ID)
	} else {
		attachmentsBytesUsed, err = v.messageCache.AttachmentBytesUsedBySender(v.IP().String())
	}
	if err != nil {
		return nil, err
	}
	info.Stats.AttachmentTotalSize = attachmentsBytesUsed
	info.Stats.AttachmentTotalSizeRemaining = zeroIfNegative(info.Limits.AttachmentTotalSizeLimit - attachmentsBytesUsed)

	// Reservation stats from database
	var reservations int64
	if v.userManager != nil && u != nil {
		reservations, err = v.userManager.ReservationsCount(u.Name)
		if err != nil {
			return nil, err
		}
	}
	info.Stats.Reservations = reservations
	info.Stats.ReservationsRemaining = zeroIfNegative(info.Limits.ReservationsLimit - reservations)

	return info, nil
}

func (v *visitorBucket) infoLightNoLock() *visitorInfo {
	messages := v.messagesLimiter.Value()
	emails := v.emailsLimiter.Value()
	calls := v.callsLimiter.Value()
	limits := v.limitsNoLock()
	stats := &visitorStats{
		Messages:          messages,
		MessagesRemaining: zeroIfNegative(limits.MessageLimit - messages),
		Emails:            emails,
		EmailsRemaining:   zeroIfNegative(limits.EmailLimit - emails),
		Calls:             calls,
		CallsRemaining:    zeroIfNegative(limits.CallLimit - calls),
	}
	return &visitorInfo{
		Limits: limits,
		Stats:  stats,
	}
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

// visitorID returns a unique identifier for a visitor based on user or IP, using configurable prefix bits for IPv4/IPv6
func visitorID(ip netip.Addr, u *user.User, conf *Config) string {
	if visitorBucketUser(u) != nil {
		return fmt.Sprintf("user:%s", u.ID)
	}
	if ip.Is4() {
		ip = netip.PrefixFrom(ip, conf.VisitorPrefixBitsIPv4).Masked().Addr()
	} else if ip.Is6() {
		ip = netip.PrefixFrom(ip, conf.VisitorPrefixBitsIPv6).Masked().Addr()
	}
	return fmt.Sprintf("ip:%s", ip.String())
}

// visitorBucketUser returns the user a bucket may be keyed by, which is only ever a tier'd user. Untiered
// users share the IP-keyed bucket with anonymous requests, so the bucket must not carry them.
func visitorBucketUser(u *user.User) *user.User {
	if u != nil && u.Tier != nil {
		return u
	}
	return nil
}
