package limit

import (
	"time"
)

// Kind is what a key is being limited on: a request, a message, an email, ... Each kind has its
// own token bucket per key; messages, emails and calls also have a daily count.
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
	KindFirebase              // Firebase sends, paused for a while after Firebase reports the quota exceeded
	numKinds                  // Sentinel, keep last
)

const (
	numDaily = 3 // Kinds with a daily count: messages, emails, calls (see dailyIndex)
)

// kindNames are the stable names of the kinds, used in the usage table and on the wire; they
// must never change once released
var kindNames = [numKinds]string{"requests", "messages", "emails", "calls", "bandwidth", "topics", "account", "auth", "firebase"}

// Rule says how much of a kind a key may use: a token bucket of Burst tokens refilling one token
// per Refill, a Daily cap, or both. A zero Rule allows everything.
type Rule struct {
	Kind      Kind
	Burst     int64         // Token bucket size; 0 for no bucket
	Refill    time.Duration // Time to refill one token; 0 for a bucket that never refills
	Daily     int64         // Cap per 24h period of the key (see periodStart), for kinds with a daily count; 0 for none
	Threshold int64         // Daily count from which the key's counts are written to the store; 0 from the first unit
}

// Decision is the answer to Allow: whether the usage was admitted, and if not, how long until
// it could be (0 if unknown, e.g. when n exceeds the burst)
type Decision struct {
	OK         bool
	RetryAfter time.Duration
}

// Usage is a key's daily counts in its current period
type Usage struct {
	counts [numKinds]int64
}

// Config configures the service
type Config struct {
	Store *Store // Where the daily counts are shared and kept across restarts; nil for memory only
}

// counter is one key's daily usage in its current period
type counter struct {
	periodStart int64           // Database time the period started (unix seconds, see periodStart)
	base        [numDaily]int64 // The key's row as last read: all nodes' usage
	unflushed   [numDaily]int64 // Admitted here since the last write
	promoted    bool            // Written to the store: reached the threshold here, or has a row
}

// bucketEntry is one key's token buckets
type bucketEntry struct {
	buckets     [numKinds]bucket
	counts      [numKinds]int64 // Units used in the current period of the kinds without a counter (bandwidth, ...), memory only
	periodStart int64           // The period of counts (see periodStart)
	seen        int64           // Last use, unix nanoseconds
	full        int64           // When every bucket is full again, unix nanoseconds
}

// bucket is a token bucket. It is created full: last is zero until the first use, and tokens
// then start at the rule's burst. Tokens may go negative (debt) when a burst shrinks.
type bucket struct {
	tokens float64
	last   int64 // Unix nanoseconds of the last refill, 0 if never used
	burst  int64 // Burst of the rule the bucket was last used with; caps refunds and debt
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

// Count returns the usage of the given kind; 0 for a kind without a daily count
func (u Usage) Count(kind Kind) int64 {
	if kind < numKinds {
		return u.counts[kind]
	}
	return 0
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

// Value returns the key's daily count of the kind (0 for a kind without one)
func (l *limiter) Value() int64 {
	return l.s.Usage(l.key).Count(l.rule.Kind)
}

// Reset fills the bucket and zeroes the key's usage of the kind
func (l *limiter) Reset() {
	l.s.reset(l.key, l.rule.Kind)
}
