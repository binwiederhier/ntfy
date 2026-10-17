package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinVersionOf(t *testing.T) {
	breaking := Breaking(Nop)
	cases := []struct {
		name       string
		migrations map[int]Migration
		expected   int
	}{
		{"no steps", map[int]Migration{}, 1},
		{"only additive steps", map[int]Migration{1: Nop, 2: Nop, 3: Nop}, 1},
		{"one breaking step", map[int]Migration{1: Nop, 2: Nop, 3: breaking, 4: Nop, 5: Nop}, 4},
		{"newest breaking step wins", map[int]Migration{1: Nop, 2: breaking, 3: Nop, 4: Nop, 5: breaking, 6: Nop}, 6},
		{"breaking last step", map[int]Migration{4: Nop, 5: breaking}, 6},
		{"breaking first step", map[int]Migration{1: breaking, 2: Nop}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, minVersionOf(c.migrations))
		})
	}
}

func TestTargetVersionOf(t *testing.T) {
	cases := []struct {
		name       string
		migrations map[int]Migration
		expected   int
	}{
		{"no steps", nil, 1},
		{"one step", map[int]Migration{1: Nop}, 2},
		{"steps starting later", map[int]Migration{14: Nop, 15: Nop, 16: Nop}, 17}, // A store added to Postgres at version 14
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, err := targetVersionOf(c.migrations)
			require.Nil(t, err)
			require.Equal(t, c.expected, version)
		})
	}
}

func TestTargetVersionOf_Invalid(t *testing.T) {
	_, err := targetVersionOf(map[int]Migration{1: Nop, 3: Nop, 4: Nop})
	require.ErrorContains(t, err, "missing migration step from version 2 to 3")
	_, err = targetVersionOf(map[int]Migration{0: Nop, 1: Nop})
	require.ErrorContains(t, err, "migration step 0 is invalid")
}
