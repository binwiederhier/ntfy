package limit

import (
	"time"
)

const (
	fnvOffset   = 2166136261           // FNV-1a 32-bit offset basis
	fnvPrime    = 16777619             // FNV-1a 32-bit prime
	fnvOffset64 = 14695981039346656037 // FNV-1a 64-bit offset basis
	fnvPrime64  = 1099511628211        // FNV-1a 64-bit prime
	oneDay      = 24 * time.Hour
	daySeconds  = int64(oneDay / time.Second)
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

// dailyKinds are the kinds with a counter, in the order of the counter's arrays (see dailyIndex)
var dailyKinds = [numDaily]Kind{KindMessages, KindEmails, KindCalls}

// dailyIndex returns the index of the kind in a counter's arrays, or -1 if the kind has no daily count
func dailyIndex(kind Kind) int {
	switch kind {
	case KindMessages:
		return 0
	case KindEmails:
		return 1
	case KindCalls:
		return 2
	}
	return -1
}

// keyOffset returns the key's reset time of day (UTC). It is a hash of the key, so the resets of
// all keys are spread over the day; it is unseeded, so every node and every restart agree.
func keyOffset(key string) time.Duration {
	h := uint64(fnvOffset64)
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= fnvPrime64
	}
	return time.Duration(h%uint64(daySeconds)) * time.Second
}

// periodStart returns the unix second the key's current 24h period started at t: the latest
// day + keyOffset(key) at or before t. Nodes with synced clocks agree on it.
func periodStart(key string, t time.Time) int64 {
	offset := int64(keyOffset(key) / time.Second)
	since := t.Unix() - offset
	days := since / daySeconds
	if since < 0 && since%daySeconds != 0 {
		days-- // Round toward the past, not toward zero
	}
	return days*daySeconds + offset
}

// periodEnd returns when the key's period that t falls in ends, i.e. when its daily counts reset
func periodEnd(key string, t time.Time) time.Time {
	return time.Unix(periodStart(key, t)+daySeconds, 0).UTC()
}
