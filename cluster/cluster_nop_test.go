package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNop_NeverDrainsAndHasNoPeers(t *testing.T) {
	// A single node has nowhere to send its clients, so it never drains
	c := &nopCluster{}
	c.SetDraining(true)
	require.False(t, c.Draining())
	require.Empty(t, c.Peers())
	require.False(t, c.Peers().HasActive())
	require.Equal(t, RoleActive, c.Role())
	require.True(t, c.Ready())
}
