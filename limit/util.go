package limit

import (
	"time"

	"heckel.io/ntfy/v2/util"
)

const (
	fnvOffset = 2166136261 // FNV-1a 32-bit offset basis
	fnvPrime  = 16777619   // FNV-1a 32-bit prime
	oneDay    = 24 * time.Hour
)

// shardIndex returns the shard a key belongs to (FNV-1a, no allocation)
func shardIndex(key string) int {
	h := uint32(fnvOffset)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= fnvPrime
	}
	return int(h % shardCount)
}

// dayFor returns the usage day of the given time: the calendar date (UTC) shifted so that the
// day rolls over at the reset wall-clock time instead of midnight. resetTime carries only a
// wall-clock time of day (like the server's visitor-stats-reset-time).
func dayFor(now time.Time, resetTime time.Time) string {
	offset := time.Duration(resetTime.Hour())*time.Hour + time.Duration(resetTime.Minute())*time.Minute + time.Duration(resetTime.Second())*time.Second
	return now.UTC().Add(-offset).Format("2006-01-02")
}

// untilReset returns how long until the next daily reset after now; a full day if now is the
// reset time itself
func untilReset(now time.Time, resetTime time.Time) time.Duration {
	next := util.NextOccurrenceUTC(resetTime, now)
	if !next.After(now) {
		next = next.Add(oneDay)
	}
	return next.Sub(now)
}
