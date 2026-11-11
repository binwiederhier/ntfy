// Package visitor turns a request's identity (its IP, or its user) into the visitor key the rate
// limiters are keyed by, and resolves the limits that apply to it from the server defaults or
// the user's tier. It knows nothing about HTTP or about counting; see package limit for that.
package visitor

import (
	"time"
)

const (
	// DefaultReservationsLimit is the amount of topic names a user without a tier is allowed to reserve.
	// This number is zero, and changing it may have unintended consequences in the web app, or otherwise
	DefaultReservationsLimit = int64(0)

	// DefaultCallsLimit is the amount of calls a user without a tier is allowed to make.
	// This number is zero, because phone numbers have to be verified first.
	DefaultCallsLimit = int64(0)

	// oneDay is an approximation of a day as a time.Duration
	oneDay = 24 * time.Hour
)

// Constants used to convert a tier-user's MessageSizeLimit (see user.Tier) into adequate request limiter
// values (token bucket). This is only used to increase the values in server.yml, never decrease them.
//
// Example: Assuming a user.Tier's MessageSizeLimit is 10,000:
// - the allowed burst is 500 (= 10,000 * 5%), which is < 1000 (the max)
// - the replenish rate is 2 * 10,000 / 24 hours
const (
	messageToRequestLimitBurstRate       = 0.05
	messageToRequestLimitBurstMax        = 1000
	messageToRequestLimitReplenishFactor = 2
)

// Constants used to convert a tier-user's EmailLimit (see user.Tier) into adequate email limiter
// values (token bucket). Example: Assuming a user.Tier's EmailLimit is 200, the allowed burst is
// 40 (= 200 * 20%), which is <150 (the max).
const (
	emailLimitBurstRate = 0.2
	emailLimitBurstMax  = 150
)

// Basis describes how the visitor limits were derived, either from a user's
// IP address (default config), or from its tier
type Basis string

// The two bases of a visitor's limits
const (
	BasisIP   = Basis("ip")
	BasisTier = Basis("tier")
)

// Config carries the parts of the server config that keying and limit resolution need. The server
// assembles it from its own config, which keeps this package free of server types.
type Config struct {
	PrefixBitsIPv4                int           // Number of bits for IPv4 rate limiting (default: 32)
	PrefixBitsIPv6                int           // Number of bits for IPv6 rate limiting (default: 64)
	RequestLimitBurst             int           // Request bucket size for IP-keyed visitors; also the floor for tiers
	RequestLimitReplenish         time.Duration // Time to refill one request token for IP-keyed visitors; also the floor for tiers
	MessageDailyLimit             int           // Daily messages for IP-keyed visitors; 0 derives it from RequestLimitReplenish
	EmailLimitBurst               int           // Email bucket size for IP-keyed visitors; also the floor for tiers
	EmailLimitReplenish           time.Duration // Time to refill one email token for IP-keyed visitors
	AttachmentTotalSizeLimit      int64         // Total attachment bytes an IP-keyed visitor may store
	AttachmentFileSizeLimit       int64         // Largest single attachment for IP-keyed visitors
	AttachmentExpiryDuration      time.Duration // How long IP-keyed visitors' attachments are kept
	AttachmentDailyBandwidthLimit int64         // Daily attachment bandwidth bytes for IP-keyed visitors
	MessageExpiryDuration         time.Duration // How long IP-keyed visitors' messages are cached
}

// Limits are the limits that apply to one visitor, resolved from the config (Basis "ip") or from
// the user's tier (Basis "tier")
type Limits struct {
	Basis                    Basis
	RequestLimitBurst        int
	RequestLimitReplenish    time.Duration // Time to refill one request token
	MessageLimit             int64
	MessageExpiryDuration    time.Duration
	EmailLimit               int64
	EmailLimitBurst          int
	EmailLimitReplenish      time.Duration // Time to refill one email token; 0 never refills
	CallLimit                int64
	ReservationsLimit        int64
	AttachmentTotalSizeLimit int64
	AttachmentFileSizeLimit  int64
	AttachmentExpiryDuration time.Duration
	AttachmentBandwidthLimit int64
}
