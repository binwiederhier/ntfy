package util

import (
	"errors"
	"hash/fnv"
	"math"
)

// BloomFilter is a fixed-size probabilistic set: Contains may return false positives (bounded by
// the target rate the filter was sized for) but never false negatives. Elements cannot be
// removed; rebuild the filter from scratch instead.
type BloomFilter struct {
	bits []uint64
	k    int // number of hash probes per element, derived via double hashing
}

// NewBloomFilter creates a filter sized for n elements at the given false-positive rate.
func NewBloomFilter(n int, fpRate float64) *BloomFilter {
	if n < 1 {
		n = 1
	}
	if fpRate <= 0 || fpRate >= 1 {
		fpRate = 0.01
	}
	m := int(math.Ceil(-float64(n) * math.Log(fpRate) / (math.Ln2 * math.Ln2))) // bits
	k := int(math.Round(float64(m) / float64(n) * math.Ln2))                    // probes
	if k < 1 {
		k = 1
	}
	return &BloomFilter{
		bits: make([]uint64, (m+63)/64),
		k:    k,
	}
}

// Add inserts an element into the filter.
func (b *BloomFilter) Add(s string) {
	h1, h2 := hashPair(s)
	m := uint64(len(b.bits)) * 64
	for i := 0; i < b.k; i++ {
		bit := (h1 + uint64(i)*h2) % m
		b.bits[bit/64] |= 1 << (bit % 64)
	}
}

// Contains reports whether the element may be in the set. A false result is definitive: the
// element was never added.
func (b *BloomFilter) Contains(s string) bool {
	h1, h2 := hashPair(s)
	m := uint64(len(b.bits)) * 64
	for i := 0; i < b.k; i++ {
		bit := (h1 + uint64(i)*h2) % m
		if b.bits[bit/64]&(1<<(bit%64)) == 0 {
			return false
		}
	}
	return true
}

// MarshalBinary serializes the filter as [k][bits...], with 64-bit little-endian words.
func (b *BloomFilter) MarshalBinary() ([]byte, error) {
	data := make([]byte, 1+len(b.bits)*8)
	data[0] = byte(b.k)
	for i, word := range b.bits {
		for j := 0; j < 8; j++ {
			data[1+i*8+j] = byte(word >> (8 * j))
		}
	}
	return data, nil
}

// UnmarshalBloomFilter deserializes a filter produced by MarshalBinary.
func UnmarshalBloomFilter(data []byte) (*BloomFilter, error) {
	if len(data) < 9 || (len(data)-1)%8 != 0 {
		return nil, errors.New("invalid bloom filter data")
	}
	b := &BloomFilter{
		bits: make([]uint64, (len(data)-1)/8),
		k:    int(data[0]),
	}
	if b.k < 1 {
		return nil, errors.New("invalid bloom filter hash count")
	}
	for i := range b.bits {
		var word uint64
		for j := 0; j < 8; j++ {
			word |= uint64(data[1+i*8+j]) << (8 * j)
		}
		b.bits[i] = word
	}
	return b, nil
}

// hashPair derives the two independent hash values used for double hashing (probe i uses
// h1 + i*h2), from FNV-1a over the element and a domain-separated variant of it.
func hashPair(s string) (uint64, uint64) {
	f := fnv.New64a()
	f.Write([]byte(s))
	h1 := f.Sum64()
	f.Write([]byte{0xff}) // Domain separation for the second hash
	h2 := f.Sum64() | 1   // Odd, so probes cycle through all bit positions
	return h1, h2
}
