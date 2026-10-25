package server

import (
	"math"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	vis "heckel.io/ntfy/v2/visitor" // Aliased: the server has its own visitor type
)

const (
	// oneDay is an approximation of a day as a time.Duration
	oneDay = 24 * time.Hour

	// visitorExpungeAfter defines how long a visitor is active before it is removed from memory. This number
	// has to be very high to prevent e-mail abuse, but it doesn't really affect the other limits anyway, since
	// they are replenished faster (typically).
	visitorExpungeAfter = oneDay
)

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	config               *Config
	limitsConfig         *vis.Config // The parts of config that keying and limit resolution need (see visitorConfigFrom)
	messageCache         *message.Cache
	userManager          *user.Manager      // May be nil
	ip                   netip.Addr         // Visitor IP address
	user                 *user.User         // Only set if authenticated user, otherwise nil
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
	statsPersisted       user.Stats         // Counter values already handed to the user manager's write queue; deltas against it are enqueued (see enqueueUserStatsDeltaNoLock)
	mu                   sync.RWMutex
}

type visitorInfo struct {
	Limits *vis.Limits
	Stats  *visitorStats
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

func newVisitor(conf *Config, messageCache *message.Cache, userManager *user.Manager, ip netip.Addr, user *user.User) *visitor {
	var messages, emails, calls int64
	if user != nil {
		messages = user.Stats.Messages
		emails = user.Stats.Emails
		calls = user.Stats.Calls
	}
	v := &visitor{
		config:               conf,
		limitsConfig:         visitorConfigFrom(conf),
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
	v.resetLimitersNoLock(messages, emails, calls)
	return v
}

func (v *visitor) Context() log.Context {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.contextNoLock()
}

func (v *visitor) contextNoLock() log.Context {
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
func (v *visitor) RequestAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.requestLimiter.Allow()
}

func (v *visitor) FirebaseAllowed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return !time.Now().Before(v.firebase)
}

func (v *visitor) FirebaseTemporarilyDeny() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.firebase = time.Now().Add(v.config.FirebaseQuotaExceededPenaltyDuration)
}

func (v *visitor) MessageAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.messagesLimiter.Allow()
}

func (v *visitor) EmailAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.emailsLimiter.Allow()
}

func (v *visitor) CallAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.callsLimiter.Allow()
}

func (v *visitor) SubscriptionAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.subscriptionLimiter.Allow()
}

// TopicCreationAllowed returns true if the visitor is allowed to cause a new topic to be
// inserted into the server's in-memory topic map. Returns true if no limiter is configured.
func (v *visitor) TopicCreationAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.topicCreationLimiter == nil {
		return true
	}
	return v.topicCreationLimiter.Allow()
}

// AuthAllowed returns true if an auth request can be attempted (> 1 token available)
func (v *visitor) AuthAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.authLimiter == nil {
		return true
	}
	return v.authLimiter.Tokens() > 1
}

// AuthFailed records an auth failure
func (v *visitor) AuthFailed() {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.authLimiter != nil {
		v.authLimiter.Allow()
	}
}

// AccountActionAllowed returns true if a rate-limited account action (signup or password-reset
// request) is currently allowed for this visitor
func (v *visitor) AccountActionAllowed() bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.accountLimiter == nil || (v.accountLimiter != nil && v.accountLimiter.Tokens() < 1) {
		return false
	}
	return true
}

// AccountActionPerformed decreases the account limiter. This is to be called after a rate-limited
// account action (signup or password-reset request).
func (v *visitor) AccountActionPerformed() {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	if v.accountLimiter != nil {
		v.accountLimiter.Allow()
	}
}

func (v *visitor) BandwidthAllowed(bytes int64) bool {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.bandwidthLimiter.AllowN(bytes)
}

func (v *visitor) RemoveSubscription() {
	v.mu.RLock()
	defer v.mu.RUnlock()
	v.subscriptionLimiter.AllowN(-1)
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

func (v *visitor) BandwidthLimiter() util.Limiter {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return v.bandwidthLimiter
}

func (v *visitor) Stale() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return time.Since(v.seen) > visitorExpungeAfter
}

func (v *visitor) Stats() *user.Stats {
	v.mu.RLock() // limiters could be replaced!
	defer v.mu.RUnlock()
	return &user.Stats{
		Messages: v.messagesLimiter.Value(),
		Emails:   v.emailsLimiter.Value(),
		Calls:    v.callsLimiter.Value(),
	}
}

// ResetStats zeroes the visitor's in-memory stats at the daily rollover, and with them the
// persisted seeds its deltas are measured against, because the caller zeroes the user's
// database row in the same pass (in a cluster that row is reset once, by the leader). Only the
// stats resetter calls this.
func (v *visitor) ResetStats() {
	v.mu.Lock() // Also protects statsPersisted
	defer v.mu.Unlock()
	v.emailsLimiter.Reset()
	v.messagesLimiter.Reset()
	v.callsLimiter.Reset()
	v.statsPersisted = user.Stats{} // The database row is zeroed by the (leader's) daily reset
}

// EnqueueUserStatsDelta enqueues the visitor's not-yet-persisted stats increments to the user
// manager's write queue. Deltas add up across cluster nodes (see Manager.EnqueueUserStats).
func (v *visitor) EnqueueUserStatsDelta() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.enqueueUserStatsDeltaNoLock()
}

func (v *visitor) enqueueUserStatsDeltaNoLock() {
	u := v.user // Pinned for the whole function: SetUser(nil) must not turn this into a nil deref
	if v.userManager == nil || u == nil {
		return
	}
	current := user.Stats{
		Messages: v.messagesLimiter.Value(),
		Emails:   v.emailsLimiter.Value(),
		Calls:    v.callsLimiter.Value(),
	}
	delta := user.Stats{
		Messages: current.Messages - v.statsPersisted.Messages,
		Emails:   current.Emails - v.statsPersisted.Emails,
		Calls:    current.Calls - v.statsPersisted.Calls,
	}
	if delta == (user.Stats{}) {
		return
	}
	v.statsPersisted = current
	v.userManager.EnqueueUserStats(u.ID, &delta)
}

// User returns the visitor user, or nil if there is none
func (v *visitor) User() *user.User {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.user // May be nil
}

// IP returns the visitor IP address
func (v *visitor) IP() netip.Addr {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.ip
}

// Authenticated returns true if a user successfully authenticated
func (v *visitor) Authenticated() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.user != nil
}

// SetUser sets the visitors user to the given value
func (v *visitor) SetUser(u *user.User) {
	v.mu.Lock()
	defer v.mu.Unlock()
	shouldResetLimiters := v.user.TierID() != u.TierID() // TierID works with nil receiver
	if shouldResetLimiters && v.user != nil && v.user.Tier != nil {
		// Flush increments counted against the old limiters, under the old account, before they
		// are replaced below; the rebuilt limiters are re-seeded from the new user's persisted
		// stats. Usage of an IP-keyed visitor (no user, or no tier) is never persisted, so it must
		// not land on the new account.
		v.enqueueUserStatsDeltaNoLock()
	}
	v.user = u // u may be nil!
	if shouldResetLimiters {
		var messages, emails, calls int64
		if u != nil {
			messages, emails, calls = u.Stats.Messages, u.Stats.Emails, u.Stats.Calls
		}
		v.resetLimitersNoLock(messages, emails, calls)
	}
}

// MaybeUserID returns the user ID of the visitor (if any). If this is an anonymous visitor,
// an empty string is returned.
func (v *visitor) MaybeUserID() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.user != nil {
		return v.user.ID
	}
	return ""
}

func (v *visitor) resetLimitersNoLock(messages, emails, calls int64) {
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
	v.statsPersisted = user.Stats{Messages: messages, Emails: emails, Calls: calls} // Seeds come from the persisted user stats, so nothing is owed to the queue
	log.Fields(v.contextNoLock()).Debug("Rate limiters reset for visitor")          // Must be after function, because contextNoLock() describes rate limiters
}

func (v *visitor) Limits() *vis.Limits {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.limitsNoLock()
}

func (v *visitor) limitsNoLock() *vis.Limits {
	var tier *user.Tier
	if v.user != nil {
		tier = v.user.Tier // May be nil, which resolves the config-based limits
	}
	return vis.LimitsFor(v.limitsConfig, tier)
}

func (v *visitor) Info() (*visitorInfo, error) {
	v.mu.RLock()
	info := v.infoLightNoLock()
	v.mu.RUnlock()

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

func (v *visitor) infoLightNoLock() *visitorInfo {
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

// visitorID returns a unique identifier for a visitor based on user or IP, using configurable prefix bits for IPv4/IPv6
func visitorID(ip netip.Addr, u *user.User, conf *Config) string {
	return vis.Key(ip, u, visitorConfigFrom(conf))
}

// visitorConfigFrom assembles the keying and limit-resolution config from the server config
func visitorConfigFrom(conf *Config) *vis.Config {
	return &vis.Config{
		PrefixBitsIPv4:                conf.VisitorPrefixBitsIPv4,
		PrefixBitsIPv6:                conf.VisitorPrefixBitsIPv6,
		RequestLimitBurst:             conf.VisitorRequestLimitBurst,
		RequestLimitReplenish:         conf.VisitorRequestLimitReplenish,
		MessageDailyLimit:             conf.VisitorMessageDailyLimit,
		EmailLimitBurst:               conf.VisitorEmailLimitBurst,
		EmailLimitReplenish:           conf.VisitorEmailLimitReplenish,
		AttachmentTotalSizeLimit:      conf.VisitorAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:       conf.AttachmentFileSizeLimit,
		AttachmentExpiryDuration:      conf.AttachmentExpiryDuration,
		AttachmentDailyBandwidthLimit: conf.VisitorAttachmentDailyBandwidthLimit,
		MessageExpiryDuration:         conf.CacheDuration,
	}
}
