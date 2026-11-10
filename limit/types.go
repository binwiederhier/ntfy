package limit

import (
	"time"
)

// Config configures the limit service
type Config struct {
	Store       *Store // Shares the daily counts and keeps them across restarts; nil for memory only
	TrackDeltas bool   // Record the tokens used per key and kind, for BucketDeltas to hand to the peers; off on a single node, where nothing would drain them
}

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
// it could be (0 if unknown, e.g. when n exceeds the burst). The other fields describe the limit
// that decided, for the RateLimit headers: the daily cap if the rule has one, else the bucket.
type Decision struct {
	OK         bool          // Whether the usage was admitted; no header
	RetryAfter time.Duration // Retry-After (on a 429): how long until the usage could be admitted
	Kind       Kind          // RateLimit-Policy and RateLimit: the policy name ("messages"), the rule's kind
	Quota      int64         // RateLimit-Policy q=: the daily cap or the burst; 0 if the rule has neither (no headers)
	Window     time.Duration // RateLimit-Policy w=: a day, or the time the bucket takes to refill completely
	Remaining  int64         // RateLimit r=: units left after this call
	Reset      time.Duration // RateLimit t=: until the period ends, or until the bucket's next token (0 if it never refills)
}

// BucketUsages is what the nodes of a cluster exchange about their token buckets, per key and kind
// (see Service.BucketDeltas and Service.BucketSnapshot)
type BucketUsages map[string]map[Kind]*BucketUsage

// Buckets is what a node sends its peers about its token buckets as of Time: what it used since its
// last round (Service.BucketDeltas), or what is used below each bucket's burst, for a peer that starts
// (Service.BucketSnapshot), which then contains the node's rounds up to Time. Time orders a node's
// rounds, also across its restarts (with clocks in sync, see the package comment), so a receiver can
// skip a round that came late or that a snapshot it took over already contains.
type Buckets struct {
	Time   int64        `json:"t"` // When the round was taken, unix nanoseconds; above the node's last round even if its clock went back
	Usages BucketUsages `json:"b"`
}

// BucketUsage is one bucket's used tokens: since the last exchange (a delta), or below its burst (a
// snapshot); with the bucket's rule, so a node that never saw the key can create it
type BucketUsage struct {
	Used   float64       `json:"u"`
	Burst  int64         `json:"b"`
	Refill time.Duration `json:"r"` // Nanoseconds on the wire
}

// Usage is a key's daily counts in its current period
type Usage struct {
	counts [numDaily]int64 // Indexed like a counter's arrays (see dailyIndex)
}

// String returns the kind's stable name
func (k Kind) String() string {
	if k < numKinds {
		return kindNames[k]
	}
	return "unknown"
}

// MarshalText returns the kind's stable name, so kinds are names on the wire (also as map keys)
func (k Kind) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText parses a kind's name. A name this node does not know (a newer peer's kind) is not
// an error, so the rest of the message still counts; it becomes an invalid kind that callers skip.
func (k *Kind) UnmarshalText(text []byte) error {
	kind, ok := ParseKind(string(text))
	if !ok {
		kind = numKinds
	}
	*k = kind
	return nil
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
	if i := dailyIndex(kind); i >= 0 {
		return u.counts[i]
	}
	return 0
}
