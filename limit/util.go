package limit

import (
	"time"
)

const (
	fnvOffset     = 2166136261 // FNV-1a 32-bit offset basis
	fnvPrime      = 16777619   // FNV-1a 32-bit prime
	oneDay        = 24 * time.Hour
	periodSeconds = int64(oneDay / time.Second)
)

// shardIndex returns the shard a key belongs to
func shardIndex(key string) int {
	return int(hashKey(key) % shardCount)
}

// keyOffset returns where in the UTC day the key's usage periods start, in seconds. It is a
// hash of the key, so the daily resets of all keys are spread over the day instead of all
// happening at midnight; it is stable (unseeded), so every node and every restart agree.
func keyOffset(key string) int64 {
	return int64(hashKey(key)) % periodSeconds
}

// periodOf returns the number of the key's usage period that t falls in: periods are 24 hours
// long and start at the key's offset (see keyOffset)
func periodOf(key string, t time.Time) int64 {
	return (t.Unix() - keyOffset(key)) / periodSeconds
}

// periodEnd returns when the key's given period ends, i.e. when its daily counts reset
func periodEnd(key string, period int64) time.Time {
	return time.Unix((period+1)*periodSeconds+keyOffset(key), 0).UTC()
}

// hashKey returns the FNV-1a hash of the key (no allocation)
func hashKey(key string) uint32 {
	h := uint32(fnvOffset)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= fnvPrime
	}
	return h
}
