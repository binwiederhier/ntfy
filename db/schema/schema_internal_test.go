package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMinVersionOf(t *testing.T) {
	breaking := Breaking(Nop)
	cases := []struct {
		name       string
		migrations map[MigrationKey]Migration
		expected   int
	}{
		{"no steps", map[MigrationKey]Migration{}, 1},
		{"only additive steps", map[MigrationKey]Migration{{From: 1, To: 2}: Nop, {From: 2, To: 3}: Nop, {From: 3, To: 4}: Nop}, 1},
		{"one breaking step", map[MigrationKey]Migration{{From: 1, To: 2}: Nop, {From: 2, To: 3}: Nop, {From: 3, To: 4}: breaking, {From: 4, To: 5}: Nop, {From: 5, To: 6}: Nop}, 4},
		{"newest breaking step wins", map[MigrationKey]Migration{{From: 1, To: 2}: Nop, {From: 2, To: 3}: breaking, {From: 3, To: 4}: Nop, {From: 4, To: 5}: Nop, {From: 5, To: 6}: breaking, {From: 6, To: 7}: Nop}, 6},
		{"breaking last step", map[MigrationKey]Migration{{From: 4, To: 5}: Nop, {From: 5, To: 6}: breaking}, 6},
		{"breaking first step", map[MigrationKey]Migration{{From: 1, To: 2}: breaking, {From: 2, To: 3}: Nop}, 2},
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
		migrations map[MigrationKey]Migration
		expected   int
	}{
		{"no steps", nil, 1},
		{"one step", map[MigrationKey]Migration{{From: 1, To: 2}: Nop}, 2},
		{"steps starting later", map[MigrationKey]Migration{{From: 14, To: 15}: Nop, {From: 15, To: 16}: Nop, {From: 16, To: 17}: Nop}, 17}, // A store added to Postgres at version 14
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
	_, err := targetVersionOf(map[MigrationKey]Migration{{From: 1, To: 2}: Nop, {From: 3, To: 4}: Nop, {From: 4, To: 5}: Nop})
	require.ErrorContains(t, err, "missing migration step from version 2 to 3")
	_, err = targetVersionOf(map[MigrationKey]Migration{{From: 0, To: 1}: Nop, {From: 1, To: 2}: Nop})
	require.ErrorContains(t, err, "migration step from version 0 is invalid")

	// A step goes exactly one version: a key skipping one would make fresh installs record a
	// version that existing installs never reach
	_, err = targetVersionOf(map[MigrationKey]Migration{{From: 1, To: 2}: Nop, {From: 2, To: 4}: Nop})
	require.ErrorContains(t, err, "migration step from version 2 to 4 is invalid, a step goes one version at a time")
	_, err = targetVersionOf(map[MigrationKey]Migration{{From: 3, To: 3}: Nop})
	require.ErrorContains(t, err, "migration step from version 3 to 3 is invalid, a step goes one version at a time")
}
