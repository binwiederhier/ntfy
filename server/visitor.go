package server

import (
	"net/netip"
	"time"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/message"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	vis "heckel.io/ntfy/v2/visitor" // Aliased: the server has its own visitor type
)

const (
	// oneDay is an approximation of a day as a time.Duration
	oneDay = 24 * time.Hour

	// visitorExpungeAfter is how long a limiter key may go unused before its buckets and counts are
	// forgotten (see Server.pruneVisitors), and how long a topic keeps a rate visitor whose subscriber
	// is gone. This number has to be very high to prevent e-mail abuse, but it doesn't really affect
	// the other limits anyway, since they are replenished faster (typically).
	visitorExpungeAfter = oneDay
)

// visitor represents one request's API user: its identity (the IP, or the user) and the limits that
// apply to it. It is built per request and never changed afterwards, so concurrent requests from the
// same IP cannot see each other's user (#1051). The counting happens in the limit service, keyed by
// key, so two visitors with the same key share one set of buckets and counts.
type visitor struct {
	env    *visitorEnv
	key    string      // The limiter key: "ip:<prefix>" or "user:<id>", see vis.Key
	ip     netip.Addr  // Visitor IP address
	user   *user.User  // Only set if authenticated user, otherwise nil
	limits *vis.Limits // Resolved from the config or the user's tier
}

// visitorEnv is what every visitor of a server shares; it is built once, so building a visitor per
// request costs little
type visitorEnv struct {
	config        *Config
	limitsConfig  *vis.Config    // The parts of config that keying and limit resolution need (see visitorConfigFrom)
	ipLimits      *vis.Limits    // The config-based limits of visitors without a tier; never modified
	limiter       *limit.Service // Decides on and counts usage, keyed by key
	subscriptions *subscriptionGauge
	messageCache  *message.Cache
	userManager   *user.Manager // May be nil
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

func newVisitorEnv(conf *Config, limiter *limit.Service, messageCache *message.Cache, userManager *user.Manager) *visitorEnv {
	limitsConfig := visitorConfigFrom(conf)
	return &visitorEnv{
		config:        conf,
		limitsConfig:  limitsConfig,
		ipLimits:      vis.LimitsFor(limitsConfig, nil),
		limiter:       limiter,
		subscriptions: newSubscriptionGauge(),
		messageCache:  messageCache,
		userManager:   userManager, // May be nil
	}
}

func newVisitor(env *visitorEnv, ip netip.Addr, u *user.User) *visitor {
	limits := env.ipLimits
	var tier *user.Tier
	if u != nil && u.Tier != nil {
		tier = u.Tier
		limits = vis.LimitsFor(env.limitsConfig, tier) // Resolved per request: the tier's fields may change at any time
	}
	v := &visitor{
		env:    env,
		key:    vis.Key(ip, u, env.limitsConfig),
		ip:     ip,
		user:   u,
		limits: limits,
	}
	env.limiter.Touch(v.key) // The key is seen now, as the visitor's request is
	return v
}

func (v *visitor) Context() log.Context {
	info := v.infoLight()
	fields := log.Context{
		"visitor_id":                 v.key,
		"visitor_ip":                 v.ip.String(),
		"visitor_messages":           info.Stats.Messages,
		"visitor_messages_limit":     info.Limits.MessageLimit,
		"visitor_messages_remaining": info.Stats.MessagesRemaining,
		"visitor_requests_remaining": v.env.limiter.Tokens(v.key, v.requestRule()),
	}
	if v.env.config.SMTPSenderFrom != "" {
		fields["visitor_emails"] = info.Stats.Emails
		fields["visitor_emails_limit"] = info.Limits.EmailLimit
		fields["visitor_emails_remaining"] = info.Stats.EmailsRemaining
	}
	if v.env.config.TwilioAccount != "" {
		fields["visitor_calls"] = info.Stats.Calls
		fields["visitor_calls_limit"] = info.Limits.CallLimit
		fields["visitor_calls_remaining"] = info.Stats.CallsRemaining
	}
	if v.user == nil {
		fields["visitor_auth_attempts_remaining"] = v.env.limiter.Tokens(v.key, v.authRule())
	}
	if v.topicCreationLimited() {
		fields["visitor_topic_creations_remaining"] = v.env.limiter.Tokens(v.key, v.topicCreationRule())
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
	if v.limits.RequestLimitBurst <= 0 {
		return false // A bucket of nothing admits nothing, as before
	}
	return v.env.limiter.Allow(v.key, v.requestRule(), 1).OK
}

// FirebaseAllowed returns false while Firebase sends are paused for the key (see FirebaseTemporarilyDeny)
func (v *visitor) FirebaseAllowed() bool {
	rule := v.firebaseRule()
	if rule.Refill <= 0 {
		return true
	}
	return v.env.limiter.Tokens(v.key, rule) >= 1
}

// FirebaseTemporarilyDeny pauses Firebase sends for the key for the configured penalty duration
func (v *visitor) FirebaseTemporarilyDeny() {
	rule := v.firebaseRule()
	if rule.Refill <= 0 {
		return
	}
	v.env.limiter.Allow(v.key, rule, 1)
}

func (v *visitor) MessageAllowed() bool {
	if v.limits.MessageLimit <= 0 {
		return false // A daily limit of zero admits nothing; the limit service reads it as "no cap"
	}
	allowed := v.env.limiter.Allow(v.key, v.messageRule(), 1).OK
	if allowed {
		v.enqueueUserStats(&user.Stats{
			Messages: 1,
		})
	}
	return allowed
}

func (v *visitor) EmailAllowed() bool {
	if v.limits.EmailLimitBurst <= 0 {
		return false // No email bucket
	} else if v.limits.Basis == vis.BasisTier && v.limits.EmailLimit <= 0 {
		return false // No emails on the tier: the limit service reads a daily limit of zero as "no cap"
	}
	allowed := v.env.limiter.Allow(v.key, v.emailRule(), 1).OK
	if allowed {
		v.enqueueUserStats(&user.Stats{
			Emails: 1,
		})
	}
	return allowed
}

func (v *visitor) CallAllowed() bool {
	if v.limits.CallLimit <= 0 {
		return false // A daily limit of zero admits nothing (e.g. a tier without calls, verified number or not)
	}
	allowed := v.env.limiter.Allow(v.key, v.callRule(), 1).OK
	if allowed {
		v.enqueueUserStats(&user.Stats{
			Calls: 1,
		})
	}
	return allowed
}

// enqueueUserStats hands what a tier user's request admitted to the user manager's write queue, which
// adds it to the persisted stats (deltas add up across requests and cluster nodes)
func (v *visitor) enqueueUserStats(delta *user.Stats) {
	if v.env.userManager == nil || v.user == nil || v.user.Tier == nil {
		return
	}
	v.env.userManager.EnqueueUserStats(v.user.ID, delta)
}

// SubscriptionAllowed takes one of the visitor key's subscription slots on this node, if one is free.
// Every successful call must be paired with RemoveSubscription.
func (v *visitor) SubscriptionAllowed() bool {
	return v.env.subscriptions.Acquire(v.key, int64(v.env.config.VisitorSubscriptionLimit))
}

// TopicCreationAllowed returns true if the visitor is allowed to cause a new topic to be
// inserted into the server's in-memory topic map. Returns true if no limiter is configured.
func (v *visitor) TopicCreationAllowed() bool {
	if !v.topicCreationLimited() {
		return true
	}
	return v.env.limiter.Allow(v.key, v.topicCreationRule(), 1).OK
}

// AuthAllowed returns true if an auth request can be attempted (a token is available)
func (v *visitor) AuthAllowed() bool {
	return v.env.limiter.Tokens(v.key, v.authRule()) >= 1
}

// AuthFailed records an auth failure
func (v *visitor) AuthFailed() {
	v.env.limiter.Allow(v.key, v.authRule(), 1)
}

// AccountActionAllowed returns true if a rate-limited account action (signup or password-reset
// request) is currently allowed for this visitor
func (v *visitor) AccountActionAllowed() bool {
	return v.env.limiter.Tokens(v.key, v.accountRule()) >= 1
}

// AccountActionPerformed decreases the account limiter. This is to be called after a rate-limited
// account action (signup or password-reset request).
func (v *visitor) AccountActionPerformed() {
	v.env.limiter.Allow(v.key, v.accountRule(), 1)
}

func (v *visitor) BandwidthAllowed(bytes int64) bool {
	if v.limits.AttachmentBandwidthLimit <= 0 {
		return false // A bandwidth limit of zero admits nothing, as before
	}
	return v.env.limiter.Allow(v.key, v.bandwidthRule(), bytes).OK
}

// RemoveSubscription gives back a slot taken by SubscriptionAllowed
func (v *visitor) RemoveSubscription() {
	v.env.subscriptions.Release(v.key)
}

// Keepalive marks the visitor's key as seen, so the limiter keeps the key of a long-lived subscription
func (v *visitor) Keepalive() {
	v.env.limiter.Touch(v.key)
}

// BandwidthLimiter returns the visitor's bandwidth bucket as a util.Limiter, for the readers and
// writers that meter bytes. A revert on the limiter (a negative AllowN) refunds the bytes.
func (v *visitor) BandwidthLimiter() util.Limiter {
	if v.limits.AttachmentBandwidthLimit <= 0 {
		return util.NewFixedLimiter(0) // A bandwidth limit of zero admits nothing, as BandwidthAllowed
	}
	return v.env.limiter.Limiter(v.key, v.bandwidthRule())
}

func (v *visitor) Stats() *user.Stats {
	usage := v.env.limiter.Usage(v.key)
	return &user.Stats{
		Messages: usage.Count(limit.KindMessages),
		Emails:   usage.Count(limit.KindEmails),
		Calls:    usage.Count(limit.KindCalls),
	}
}

// User returns the visitor user, or nil if there is none
func (v *visitor) User() *user.User {
	return v.user // May be nil
}

// IP returns the visitor IP address
func (v *visitor) IP() netip.Addr {
	return v.ip
}

// Authenticated returns true if a user successfully authenticated
func (v *visitor) Authenticated() bool {
	return v.user != nil
}

// MaybeUserID returns the user ID of the visitor (if any). If this is an anonymous visitor,
// an empty string is returned.
func (v *visitor) MaybeUserID() string {
	if v.user != nil {
		return v.user.ID
	}
	return ""
}

func (v *visitor) Limits() *vis.Limits {
	return v.limits
}

// The rules the visitor's limits translate to, built per call (they are small values); the
// limiter keeps the state.

func (v *visitor) requestRule() limit.Rule {
	return limit.Rule{
		Kind:   limit.KindRequests,
		Burst:  int64(v.limits.RequestLimitBurst),
		Refill: v.limits.RequestLimitReplenish,
	}
}

func (v *visitor) messageRule() limit.Rule {
	return limit.Rule{
		Kind:  limit.KindMessages,
		Daily: v.limits.MessageLimit,
	}
}

// emailRule is the email bucket, and for a tier also its daily email limit. Without a tier, the
// daily limit is only derived from the refill, for display, and is not a cap.
func (v *visitor) emailRule() limit.Rule {
	rule := limit.Rule{
		Kind:   limit.KindEmails,
		Burst:  int64(v.limits.EmailLimitBurst),
		Refill: v.limits.EmailLimitReplenish,
	}
	if v.limits.Basis == vis.BasisTier {
		rule.Daily = v.limits.EmailLimit
	}
	return rule
}

func (v *visitor) callRule() limit.Rule {
	return limit.Rule{
		Kind:  limit.KindCalls,
		Daily: v.limits.CallLimit,
	}
}

// bandwidthRule is a bucket of the daily bandwidth bytes that refills over a day
func (v *visitor) bandwidthRule() limit.Rule {
	rule := limit.Rule{
		Kind:  limit.KindBandwidth,
		Burst: v.limits.AttachmentBandwidthLimit,
	}
	if rule.Burst > 0 {
		rule.Refill = oneDay / time.Duration(rule.Burst)
	}
	return rule
}

func (v *visitor) topicCreationRule() limit.Rule {
	return limit.Rule{
		Kind:   limit.KindTopics,
		Burst:  int64(v.env.config.VisitorTopicCreationLimitBurst),
		Refill: v.env.config.VisitorTopicCreationLimitReplenish,
	}
}

func (v *visitor) topicCreationLimited() bool {
	return v.env.config.VisitorTopicCreationLimitBurst > 0 && v.env.config.VisitorTopicCreationLimitReplenish > 0
}

func (v *visitor) accountRule() limit.Rule {
	return limit.Rule{
		Kind:   limit.KindAccount,
		Burst:  int64(v.env.config.VisitorAccountCreationLimitBurst),
		Refill: v.env.config.VisitorAccountCreationLimitReplenish,
	}
}

func (v *visitor) authRule() limit.Rule {
	return limit.Rule{
		Kind:   limit.KindAuth,
		Burst:  int64(v.env.config.VisitorAuthFailureLimitBurst),
		Refill: v.env.config.VisitorAuthFailureLimitReplenish,
	}
}

// firebaseRule is a single token that takes the penalty duration to come back: taking it pauses sends
func (v *visitor) firebaseRule() limit.Rule {
	return limit.Rule{
		Kind:   limit.KindFirebase,
		Burst:  1,
		Refill: v.env.config.FirebaseQuotaExceededPenaltyDuration,
	}
}

func (v *visitor) Info() (*visitorInfo, error) {
	info := v.infoLight()

	// Attachment stats from database
	var attachmentsBytesUsed int64
	var err error
	u := v.User()
	if u != nil {
		attachmentsBytesUsed, err = v.env.messageCache.AttachmentBytesUsedByUser(u.ID)
	} else {
		attachmentsBytesUsed, err = v.env.messageCache.AttachmentBytesUsedBySender(v.IP().String())
	}
	if err != nil {
		return nil, err
	}
	info.Stats.AttachmentTotalSize = attachmentsBytesUsed
	info.Stats.AttachmentTotalSizeRemaining = zeroIfNegative(info.Limits.AttachmentTotalSizeLimit - attachmentsBytesUsed)

	// Reservation stats from database
	var reservations int64
	if v.env.userManager != nil && u != nil {
		reservations, err = v.env.userManager.ReservationsCount(u.Name)
		if err != nil {
			return nil, err
		}
	}
	info.Stats.Reservations = reservations
	info.Stats.ReservationsRemaining = zeroIfNegative(info.Limits.ReservationsLimit - reservations)

	return info, nil
}

func (v *visitor) infoLight() *visitorInfo {
	stats := v.Stats()
	limits := v.limits
	return &visitorInfo{
		Limits: limits,
		Stats: &visitorStats{
			Messages:          stats.Messages,
			MessagesRemaining: zeroIfNegative(limits.MessageLimit - stats.Messages),
			Emails:            stats.Emails,
			EmailsRemaining:   zeroIfNegative(limits.EmailLimit - stats.Emails),
			Calls:             stats.Calls,
			CallsRemaining:    zeroIfNegative(limits.CallLimit - stats.Calls),
		},
	}
}

func zeroIfNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
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
