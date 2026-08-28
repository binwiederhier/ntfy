package util_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/util"
)

func TestBloomFilter_NoFalseNegatives(t *testing.T) {
	// The property routing correctness relies on: an added element is ALWAYS reported present
	b := util.NewBloomFilter(1000, 0.01)
	for i := 0; i < 1000; i++ {
		b.Add(fmt.Sprintf("topic-%d", i))
	}
	for i := 0; i < 1000; i++ {
		require.True(t, b.Contains(fmt.Sprintf("topic-%d", i)))
	}
}

func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	b := util.NewBloomFilter(10000, 0.01)
	for i := 0; i < 10000; i++ {
		b.Add(fmt.Sprintf("added-%d", i))
	}
	falsePositives := 0
	for i := 0; i < 10000; i++ {
		if b.Contains(fmt.Sprintf("absent-%d", i)) {
			falsePositives++
		}
	}
	require.Less(t, falsePositives, 300, "expected ~1%% false positives, got %d/10000", falsePositives)
}

func TestBloomFilter_EmptyContainsNothing(t *testing.T) {
	b := util.NewBloomFilter(100, 0.01)
	require.False(t, b.Contains("anything"))
}

func TestBloomFilter_MarshalRoundTrip(t *testing.T) {
	b := util.NewBloomFilter(500, 0.01)
	for i := 0; i < 500; i++ {
		b.Add(fmt.Sprintf("topic-%d", i))
	}
	data, err := b.MarshalBinary()
	require.Nil(t, err)
	b2, err := util.UnmarshalBloomFilter(data)
	require.Nil(t, err)
	for i := 0; i < 500; i++ {
		require.True(t, b2.Contains(fmt.Sprintf("topic-%d", i)))
	}
	require.False(t, b2.Contains("never-added-topic"))
}

func TestBloomFilter_UnmarshalGarbage(t *testing.T) {
	_, err := util.UnmarshalBloomFilter([]byte{})
	require.Error(t, err)
	_, err = util.UnmarshalBloomFilter([]byte{1, 2, 3})
	require.Error(t, err)
}
