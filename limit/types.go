package limit

import (
	"time"
)

// Kind is what a key is being limited on: a request, a message, an email, ... Each kind has its
// own token bucket and daily count per key.
type Kind uint8

// The kinds of usage a key is limited on
const (
	KindRequests  Kind = iota // Requests of (almost) any type, including publishes
	KindMessages              // Published messages
	KindEmails                // Emails sent on behalf of the key
	KindCalls                 // Phone calls made on behalf of the key
	KindBandwidth             // Attachment and poll bytes, up and down
	KindTopics                // Topics created in the server's in-memory topic map
	KindAccount               // Account actions: signups and password reset requests
	KindAuth                  // Failed authentication attempts
	KindStrikes               // Weighted strikes of the abuse ban feed, keyed by prefix (see package ban)
	numKinds                  // Sentinel, keep last
)

// kindNames are the stable names of the kinds, used in the usage table and on the wire; they
// must never change once released
var kindNames = [numKinds]string{"requests", "messages", "emails", "calls", "bandwidth", "topics", "account", "auth", "strikes"}

// Rule says how much of a kind a key may use: a token bucket of Burst tokens refilling one token
// per Refill, a Daily cap, or both. A zero Rule allows everything.
type Rule struct {
	Kind      Kind
	Burst     int64         // Token bucket size; 0 for no bucket
	Refill    time.Duration // Time to refill one token; 0 for a bucket that never refills
	Daily     int64         // Daily cap; 0 for none
	Threshold int64         // Daily count from which the key's usage of the kind is persisted; 0 persists from the first unit
}

// Decision is the answer to Allow: whether the usage was admitted, and if not, how long until
// it could be (0 if unknown, e.g. when n exceeds the burst)
type Decision struct {
	OK         bool
	RetryAfter time.Duration
}

// Delta is what a node tells its peers: the key used n units of the kind since the last round
type Delta struct {
	Key  string
	Kind Kind
	N    int64
}

// Usage is a key's usage of every kind today
type Usage struct {
	counts [numKinds]int64
}

// Config configures the service. ResetTime is the wall-clock time of day (UTC) at which the
// usage day starts over. FlushInterval is how often usage is written to and read from the store
// (DefaultFlushInterval when 0); tests set it.
type Config struct {
	ResetTime     time.Time
	FlushInterval time.Duration
}

// limiter is the util.Limiter view of one key's bucket for one rule (see Service.Limiter)
type limiter struct {
	s    *Service
	key  string
	rule Rule
}

// String returns the kind's stable name
func (k Kind) String() string {
	if k < numKinds {
		return kindNames[k]
	}
	return "unknown"
}

// ParseKind returns the kind with the given name
func ParseKind(name string) (Kind, bool) {
	for k, n := range kindNames {
		if n == name {
			return Kind(k), true
		}
	}
	return 0, false
}

// Count returns the usage of the given kind
func (u Usage) Count(kind Kind) int64 {
	return u.counts[kind]
}

// Allow uses one unit, or returns false if the limit has been reached
func (l *limiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN uses n units, or returns false if the limit has been reached. A negative n refunds, which
// is how util.LimitReader and util.LimitWriter revert a limiter when a later one rejects.
func (l *limiter) AllowN(n int64) bool {
	if n < 0 {
		l.s.Refund(l.key, l.rule.Kind, -n)
		return true
	} else if n == 0 {
		return true
	}
	return l.s.Allow(l.key, l.rule, n).OK
}

// Value returns the key's usage of the kind today
func (l *limiter) Value() int64 {
	return l.s.Usage(l.key).Count(l.rule.Kind)
}

// Reset fills the bucket and zeroes the key's usage of the kind
func (l *limiter) Reset() {
	l.s.reset(l.key, l.rule.Kind)
}
