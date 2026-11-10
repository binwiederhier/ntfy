package limit

// limiter is the util.Limiter view of one key's bucket for one rule (see Service.Limiter)
type limiter struct {
	s    *Service
	key  string
	rule Rule
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

// Value returns the key's daily count of the kind in its current period, 0 for a kind without
// one (see Usage); the bucket's level is what Service.Tokens reports
func (l *limiter) Value() int64 {
	return l.s.Usage(l.key).Count(l.rule.Kind)
}

// Reset fills the bucket and zeroes the key's usage of the kind
func (l *limiter) Reset() {
	l.s.reset(l.key, l.rule.Kind)
}
