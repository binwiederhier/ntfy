package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinVersionOf(t *testing.T) {
	breaking := Breaking(Nop)
	cases := []struct {
		name       string
		target     int
		migrations map[int]Migration
		expected   int
	}{
		{"no steps", 1, map[int]Migration{}, 1},
		{"only additive steps", 4, map[int]Migration{1: Nop, 2: Nop, 3: Nop}, 1},
		{"one breaking step", 6, map[int]Migration{1: Nop, 2: Nop, 3: breaking, 4: Nop, 5: Nop}, 4},
		{"newest breaking step wins", 7, map[int]Migration{1: Nop, 2: breaking, 3: Nop, 4: Nop, 5: breaking, 6: Nop}, 6},
		{"breaking step right before the target", 6, map[int]Migration{4: Nop, 5: breaking}, 6},
		{"breaking first step", 3, map[int]Migration{1: breaking, 2: Nop}, 2},
		{"breaking steps at or past the target are not applied", 6, map[int]Migration{3: Nop, 6: breaking, 9: breaking}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, minVersionOf(c.target, c.migrations))
		})
	}
}
