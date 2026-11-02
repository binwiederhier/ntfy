package ban

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseWeights(t *testing.T) {
	// Exact codes, a bare 3-digit HTTP status (normalized to a family), an exempt code, and "*".
	weights, err := ParseWeights([]string{"42909:10", "403:2", "42908:0", "*:1"})
	require.NoError(t, err)
	require.Equal(t, Weights{"42909": 10, "403*": 2, "42908": 0, "*": 1}, weights)

	// A bare 3-digit HTTP status normalizes to its family.
	weights, err = ParseWeights([]string{"429:5"})
	require.NoError(t, err)
	require.Equal(t, Weights{"429*": 5}, weights)

	// An explicit family key stays as-is.
	weights, err = ParseWeights([]string{"429*:5"})
	require.NoError(t, err)
	require.Equal(t, Weights{"429*": 5}, weights)

	// Weight 0 is valid and means exempt.
	weights, err = ParseWeights([]string{"42908:0"})
	require.NoError(t, err)
	require.Equal(t, Weights{"42908": 0}, weights)

	_, err = ParseWeights([]string{"401"}) // Missing weight
	require.Error(t, err)
	_, err = ParseWeights([]string{"401:-1"}) // Negative weight
	require.Error(t, err)
	_, err = ParseWeights([]string{"401:abc"}) // Non-integer weight
	require.Error(t, err)
	_, err = ParseWeights([]string{"abc:10"}) // Non-numeric key
	require.Error(t, err)
	_, err = ParseWeights([]string{"4*3:10"}) // Star not at the end
	require.Error(t, err)
}

func TestParseWeights_Fractional(t *testing.T) {
	weights, err := ParseWeights([]string{"50701:0.05", "42909:2"})
	require.Nil(t, err)
	require.Equal(t, 0.05, weights.WeightFor(50701))
	require.Equal(t, 2.0, weights.WeightFor(42909))
	weights, err = ParseWeights([]string{"50701:0.001"})
	require.Nil(t, err)
	require.Equal(t, 0.001, weights.WeightFor(50701))
	_, err = ParseWeights([]string{"50701:0.0001"}) // Below the 0.001 resolution
	require.Error(t, err)
	_, err = ParseWeights([]string{"42909:1e16"}) // Would overflow once counted in thousandths
	require.Error(t, err)
}

func TestWeights_WeightFor(t *testing.T) {
	weights, err := ParseWeights([]string{"42908:0", "42903:0", "42905:0", "42910:0", "42909:10", "429*:1", "403*:2", "4*:1", "5*:1"})
	require.NoError(t, err)
	// Longest-match-wins: exact 5-digit beats "429*" beats "4*" beats "*".
	require.Equal(t, 0.0, weights.WeightFor(42908))
	require.Equal(t, 10.0, weights.WeightFor(42909))
	require.Equal(t, 1.0, weights.WeightFor(42901))
	require.Equal(t, 2.0, weights.WeightFor(40311))
	require.Equal(t, 1.0, weights.WeightFor(40011))
	require.Equal(t, 1.0, weights.WeightFor(50312))
	// No rule matches (this config has 4*/5* but no "*"), so the implied default weight 1 applies.
	require.Equal(t, 1.0, weights.WeightFor(30012))
}

func TestWeights_WeightFor_NoStarRuleImpliesWeight1(t *testing.T) {
	// With no "*" rule, a code that matches nothing defaults to weight 1 (can be banned), so the
	// feature can't be silently turned into a no-op by forgetting "*". Explicit codes still win.
	weights, err := ParseWeights([]string{"42908:0", "42909:10"})
	require.NoError(t, err)
	require.Equal(t, 0.0, weights.WeightFor(42908))  // explicitly exempt
	require.Equal(t, 10.0, weights.WeightFor(42909)) // explicit
	require.Equal(t, 1.0, weights.WeightFor(42901))  // unmatched -> implied 1
	require.Equal(t, 1.0, weights.WeightFor(40001))  // unmatched -> implied 1
}

func TestWeights_WeightFor_ExplicitStarZeroExemptsAll(t *testing.T) {
	// An explicit "*:0" is the opt-out: exempt everything not otherwise weighted.
	weights, err := ParseWeights([]string{"42909:10", "*:0"})
	require.NoError(t, err)
	require.Equal(t, 10.0, weights.WeightFor(42909)) // explicit
	require.Equal(t, 0.0, weights.WeightFor(42901))  // *:0 -> exempt everything else
}
