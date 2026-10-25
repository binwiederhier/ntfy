package server

import (
	"net/netip"
	"sync"
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

	// visitorExpungeAfter defines how long a visitor is active before it is removed from memory when
	// usage is counted in memory only (no usage file, no database). This number has to be very high
	// to prevent e-mail abuse, but it doesn't really affect the other limits anyway, since they are
	// replenished faster (typically).
	visitorExpungeAfter = oneDay

	// visitorExpungeAfterPersisted is the same when usage is persisted: a visitor's daily counts are
	// in the usage table (or it never had any worth a row) and its buckets refill anyway, so the
	// map only has to hold the active visitors
	visitorExpungeAfterPersisted = 5 * time.Minute

	// visitorThresholdRate is the share of a daily quota at which an IP-keyed visitor's usage gets a
	// row in the usage table (see limit.Rule.Threshold); user-keyed visitors get one from the first
	// unit, as the row is what the account page and billing read
	visitorThresholdRate = 100
)

// visitor represents an API user: its identity (the IP, or the user) and the limits that apply to it.
// The counting itself happens in the limit service, keyed by the visitor key, so two visitor objects
// with the same key share one set of buckets and counts.
type visitor struct {
	config        *Config
	limitsConfig  *vis.Config    // The parts of config that keying and limit resolution need (see visitorConfigFrom)
	limiter       *limit.Service // Decides on and counts usage, keyed by key
	messageCache  *message.Cache
	userManager   *user.Manager      // May be nil
	key           string             // The limiter key: "ip:<prefix>" or "user:<id>", see vis.Key
	ip            netip.Addr         // Visitor IP address
	user          *user.User         // Only set if authenticated user, otherwise nil
	limits        *vis.Limits        // Resolved from the config or the user's tier; replaced by SetUser when the tier changes
	subscriptions *util.FixedLimiter // Gauge of this node's open subscriptions for the visitor; never shared, it protects this node
	firebase      time.Time          // Next allowed Firebase message
	seen          time.Time          // Last seen time of this visitor (needed for removal of stale visitors)
	expungeAfter  time.Duration      // How long after seen the visitor is removed from memory (see visitorExpungeAfterFor)
	mu            sync.RWMutex
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

func newVisitor(conf *Config, limiter *limit.Service, messageCache *message.Cache, userManager *user.Manager, ip netip.Addr, user *user.User) *visitor {
	v := &visitor{
		config:        conf,
		limitsConfig:  visitorConfigFrom(conf),
		limiter:       limiter,
		messageCache:  messageCache,
		userManager:   userManager, // May be nil
		ip:            ip,
		user:          user,
		firebase:      time.Unix(0, 0),
		seen:          time.Now(),
		expungeAfter:  visitorExpungeAfterFor(limiter),
		subscriptions: util.NewFixedLimiter(int64(conf.VisitorSubscriptionLimit)),
	}
	v.resetLimitsNoLock()
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
		"visitor_id":                 v.key,
		"visitor_ip":                 v.ip.String(),
		"visitor_seen":               util.FormatTime(v.seen),
		"visitor_messages":           info.Stats.Messages,
		"visitor_messages_limit":     info.Limits.MessageLimit,
		"visitor_messages_remaining": info.Stats.MessagesRemaining,
		"visitor_requests_remaining": v.limiter.Tokens(v.key, v.requestRule()),
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
	if v.user == nil {
		fields["visitor_auth_attempts_remaining"] = v.limiter.Tokens(v.key, v.authRule())
	}
	if v.topicCreationLimited() {
		fields["visitor_topic_creations_remaining"] = v.limiter.Tokens(v.key, v.topicCreationRule())
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
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.limits.RequestLimitBurst <= 0 {
		return false // A bucket of nothing admits nothing, as before
	}
	return v.limiter.Allow(v.key, v.requestRule(), 1).OK
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
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	return v.limiter.Allow(v.key, v.messageRule(), 1).OK
}

func (v *visitor) EmailAllowed() bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.limits.EmailLimitBurst <= 0 {
		return false // No email bucket means no emails, as before
	}
	return v.limiter.Allow(v.key, v.emailRule(), 1).OK
}

func (v *visitor) CallAllowed() bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	return v.limiter.Allow(v.key, v.callRule(), 1).OK
}

func (v *visitor) SubscriptionAllowed() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.subscriptions.Allow()
}

// TopicCreationAllowed returns true if the visitor is allowed to cause a new topic to be
// inserted into the server's in-memory topic map. Returns true if no limiter is configured.
func (v *visitor) TopicCreationAllowed() bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if !v.topicCreationLimited() {
		return true
	}
	return v.limiter.Allow(v.key, v.topicCreationRule(), 1).OK
}

// AuthAllowed returns true if an auth request can be attempted (a token is available)
func (v *visitor) AuthAllowed() bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.user != nil {
		return true // Users are already logged in, no need to limit requests
	}
	return v.limiter.Tokens(v.key, v.authRule()) >= 1
}

// AuthFailed records an auth failure
func (v *visitor) AuthFailed() {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.user == nil {
		v.limiter.Allow(v.key, v.authRule(), 1)
	}
}

// AccountActionAllowed returns true if a rate-limited account action (signup or password-reset
// request) is currently allowed for this visitor
func (v *visitor) AccountActionAllowed() bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.user != nil {
		return false // Users cannot create accounts when logged in
	}
	return v.limiter.Tokens(v.key, v.accountRule()) >= 1
}

// AccountActionPerformed decreases the account limiter. This is to be called after a rate-limited
// account action (signup or password-reset request).
func (v *visitor) AccountActionPerformed() {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.user == nil {
		v.limiter.Allow(v.key, v.accountRule(), 1)
	}
}

func (v *visitor) BandwidthAllowed(bytes int64) bool {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	if v.limits.AttachmentBandwidthLimit <= 0 {
		return false // A bandwidth limit of zero admits nothing, as before
	}
	return v.limiter.Allow(v.key, v.bandwidthRule(), bytes).OK
}

func (v *visitor) RemoveSubscription() {
	v.mu.RLock()
	defer v.mu.RUnlock()
	v.subscriptions.AllowN(-1)
}

func (v *visitor) Keepalive() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.seen = time.Now()
}

// BandwidthLimiter returns the visitor's bandwidth bucket as a util.Limiter, for the readers and
// writers that meter bytes. A revert on the limiter (a negative AllowN) refunds the bytes.
func (v *visitor) BandwidthLimiter() util.Limiter {
	v.mu.RLock() // limits could be replaced!
	defer v.mu.RUnlock()
	return v.limiter.Limiter(v.key, v.bandwidthRule())
}

func (v *visitor) Stale() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return time.Since(v.seen) > v.expungeAfter
}

// Usage returns the visitor's usage today as this node knows it (see limit.Service.Usage)
func (v *visitor) Usage() limit.Usage {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.limiter.Usage(v.key)
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

// SetUser sets the visitors user to the given value. A changed tier (gained, lost, or a tier
// with other limits) re-resolves the key and the limits; the next call to the limiter uses them,
// with the buckets as they are: nothing is handed back.
func (v *visitor) SetUser(u *user.User) {
	v.mu.Lock()
	defer v.mu.Unlock()
	shouldResetLimits := v.user.TierID() != u.TierID() // TierID works with nil receiver
	tierChanged := shouldResetLimits || (u != nil && u.Tier != nil && *v.user.Tier != *u.Tier)
	v.user = u // u may be nil!
	if tierChanged {
		v.resetLimitsNoLock()
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

// resetLimitsNoLock re-resolves the key and the limits from the current user. The key's daily
// counts are the limiter's (read from the usage table as needed), so nothing is seeded. The
// caller must hold v.mu.
func (v *visitor) resetLimitsNoLock() {
	v.key = vis.Key(v.ip, v.user, v.limitsConfig)
	v.limits = v.limitsNoLock()
	log.Fields(v.contextNoLock()).Debug("Rate limits reset for visitor") // Must be after function, because contextNoLock() describes rate limits
}

func (v *visitor) Limits() *vis.Limits {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.limits
}

func (v *visitor) limitsNoLock() *vis.Limits {
	var tier *user.Tier
	if v.user != nil {
		tier = v.user.Tier // May be nil, which resolves the config-based limits
	}
	return vis.LimitsFor(v.limitsConfig, tier)
}

// The rules the visitor's limits translate to, built per call (they are small values); the
// limiter keeps the state. The caller must hold v.mu.

func (v *visitor) requestRule() limit.Rule {
	return limit.Rule{Kind: limit.KindRequests, Burst: int64(v.limits.RequestLimitBurst), Refill: v.limits.RequestLimitReplenish}
}

func (v *visitor) messageRule() limit.Rule {
	return limit.Rule{Kind: limit.KindMessages, Daily: v.limits.MessageLimit, Threshold: v.thresholdNoLock(v.limits.MessageLimit)}
}

func (v *visitor) emailRule() limit.Rule {
	return limit.Rule{Kind: limit.KindEmails, Burst: int64(v.limits.EmailLimitBurst), Refill: v.limits.EmailLimitReplenish, Daily: v.limits.EmailLimit, Threshold: v.thresholdNoLock(v.limits.EmailLimit)}
}

func (v *visitor) callRule() limit.Rule {
	return limit.Rule{Kind: limit.KindCalls, Daily: v.limits.CallLimit, Threshold: v.thresholdNoLock(v.limits.CallLimit)}
}

// thresholdNoLock returns the daily count from which the visitor's usage is persisted: from the
// first unit for a user with a tier, past a share of the quota for everyone else, so that the
// one-shot visitors of a day never get a row. The caller must hold v.mu.
func (v *visitor) thresholdNoLock(daily int64) int64 {
	if v.user != nil && v.user.Tier != nil {
		return 0
	}
	return daily / visitorThresholdRate
}

// bandwidthRule is a bucket of the daily bandwidth bytes that refills over a day
func (v *visitor) bandwidthRule() limit.Rule {
	rule := limit.Rule{Kind: limit.KindBandwidth, Burst: v.limits.AttachmentBandwidthLimit}
	if rule.Burst > 0 {
		rule.Refill = oneDay / time.Duration(rule.Burst)
	}
	return rule
}

func (v *visitor) topicCreationRule() limit.Rule {
	return limit.Rule{Kind: limit.KindTopics, Burst: int64(v.config.VisitorTopicCreationLimitBurst), Refill: v.config.VisitorTopicCreationLimitReplenish}
}

func (v *visitor) topicCreationLimited() bool {
	return v.config.VisitorTopicCreationLimitBurst > 0 && v.config.VisitorTopicCreationLimitReplenish > 0
}

func (v *visitor) accountRule() limit.Rule {
	return limit.Rule{Kind: limit.KindAccount, Burst: int64(v.config.VisitorAccountCreationLimitBurst), Refill: v.config.VisitorAccountCreationLimitReplenish}
}

func (v *visitor) authRule() limit.Rule {
	return limit.Rule{Kind: limit.KindAuth, Burst: int64(v.config.VisitorAuthFailureLimitBurst), Refill: v.config.VisitorAuthFailureLimitReplenish}
}

// Info returns the visitor's limits and stats for the account page. The daily counts come from
// the usage table if this node has not read them yet, so a node that never saw the visitor
// answers with the same numbers as one that did.
func (v *visitor) Info() (*visitorInfo, error) {
	v.mu.RLock()
	key, limits := v.key, v.limits
	v.mu.RUnlock()
	usage, err := v.limiter.Lookup(key)
	if err != nil {
		return nil, err
	}
	info := infoFromUsage(limits, usage)

	// Attachment stats from database
	var attachmentsBytesUsed int64
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
	return infoFromUsage(v.limits, v.limiter.Usage(v.key))
}

func infoFromUsage(limits *vis.Limits, usage limit.Usage) *visitorInfo {
	messages, emails, calls := usage.Count(limit.KindMessages), usage.Count(limit.KindEmails), usage.Count(limit.KindCalls)
	return &visitorInfo{
		Limits: limits,
		Stats: &visitorStats{
			Messages:          messages,
			MessagesRemaining: zeroIfNegative(limits.MessageLimit - messages),
			Emails:            emails,
			EmailsRemaining:   zeroIfNegative(limits.EmailLimit - emails),
			Calls:             calls,
			CallsRemaining:    zeroIfNegative(limits.CallLimit - calls),
		},
	}
}

func zeroIfNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

// visitorExpungeAfterFor returns how long an idle visitor stays in memory: minutes when its daily
// counts are persisted, a day when they only live in memory
func visitorExpungeAfterFor(limiter *limit.Service) time.Duration {
	if limiter.Persistent() {
		return visitorExpungeAfterPersisted
	}
	return visitorExpungeAfter
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
