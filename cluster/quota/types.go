package quota

// Key identifies a visitor in the usage table, e.g. "ip:1.2.3.4" or "user:u_abc". It is the
// visitor ID the server already uses to key rate limiters, so cluster-wide counters follow
// the exact same identity as the local ones.
type Key string

// Counters holds one visitor's usage within one quota day. All fields are monotonic within
// the day; Requests and BandwidthBytes feed the token-bucket burn-down (peer usage), the
// others are enforced against the daily limits.
type Counters struct {
	Requests       int64
	Messages       int64
	Emails         int64
	Calls          int64
	BandwidthBytes int64
}

// Add adds the other counters to this one
func (c *Counters) Add(other Counters) {
	c.Requests += other.Requests
	c.Messages += other.Messages
	c.Emails += other.Emails
	c.Calls += other.Calls
	c.BandwidthBytes += other.BandwidthBytes
}

// sub subtracts the other counters from this one, clamping each field at zero
func (c *Counters) sub(other Counters) {
	c.Requests = zeroIfNegative(c.Requests - other.Requests)
	c.Messages = zeroIfNegative(c.Messages - other.Messages)
	c.Emails = zeroIfNegative(c.Emails - other.Emails)
	c.Calls = zeroIfNegative(c.Calls - other.Calls)
	c.BandwidthBytes = zeroIfNegative(c.BandwidthBytes - other.BandwidthBytes)
}

// zero reports whether all counters are zero
func (c *Counters) zero() bool {
	return c.Requests == 0 && c.Messages == 0 && c.Emails == 0 && c.Calls == 0 && c.BandwidthBytes == 0
}

func zeroIfNegative(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
