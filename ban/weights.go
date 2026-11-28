package ban

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Weights maps a matcher key to a strike weight for the abuse ban-feed (see ParseWeights, WeightFor).
type Weights map[string]float64

// ParseWeights normalizes a list like ["42909:10","403:2","50701:0.05","*:1"] into a Weights map. A key is an
// exact ntfy code, a family ("429*"), a bare HTTP status ("403" -> "403*"), or "*"; weights are ints
// >= 0 in steps of 0.001 (0 = exempt). Malformed entries are rejected so misconfiguration fails at startup.
func ParseWeights(entries []string) (Weights, error) {
	out := make(Weights, len(entries))
	for _, entry := range entries {
		key, weightStr, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("invalid ban-weight %q, want KEY:WEIGHT", entry)
		}
		weight, err := strconv.ParseFloat(strings.TrimSpace(weightStr), 64)
		if err != nil || weight < 0 || math.IsInf(weight, 0) || math.IsNaN(weight) {
			return nil, fmt.Errorf("invalid ban-weight value in %q, want a non-negative number", entry)
		} else if weight > 0 && weight < 1.0/strikeUnits {
			return nil, fmt.Errorf("invalid ban-weight value in %q, want 0 or at least %g", entry, 1.0/strikeUnits)
		} else if weight > MaxWeight {
			return nil, fmt.Errorf("invalid ban-weight value in %q, want at most %d", entry, MaxWeight)
		}
		key = strings.TrimSpace(key)
		if !validWeightKey(key) {
			return nil, fmt.Errorf("invalid ban-weight key in %q, want %q, an ntfy code, an HTTP status, or a PREFIX*", entry, "*")
		}
		// A bare 3-digit HTTP status is shorthand for the whole family (e.g. "403" -> "403*").
		if len(key) == 3 && isAllDigits(key) {
			key += "*"
		}
		out[key] = weight
	}
	return out, nil
}

// WeightFor returns the strike weight for an ntfy error code, longest-match-wins (exact > family > "*").
// If nothing matches (no "*" catch-all) it returns the implied default 1, so a forgotten "*" still
// bans; use "*:0" to exempt everything not explicitly weighted.
func (w Weights) WeightFor(errorCode int) float64 {
	code := strconv.Itoa(errorCode)
	weight, bestLen, matched := 0.0, -1, false
	for key, wt := range w {
		matchLen := -1
		switch {
		case key == "*":
			matchLen = 0
		case strings.HasSuffix(key, "*"):
			if prefix := strings.TrimSuffix(key, "*"); strings.HasPrefix(code, prefix) {
				matchLen = len(prefix)
			}
		case key == code:
			matchLen = len(code)
		}
		if matchLen > bestLen {
			weight, bestLen, matched = wt, matchLen, true
		}
	}
	if !matched {
		return 1
	}
	return weight
}

// validWeightKey reports whether key is a legal matcher: "*", an all-digits code, or DIGITS*.
func validWeightKey(key string) bool {
	if key == "*" {
		return true
	}
	digits := strings.TrimSuffix(key, "*")
	return digits != "" && isAllDigits(digits)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
