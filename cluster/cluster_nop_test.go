package cluster

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNop_NeverInMaintenanceAndNoPeers(t *testing.T) {
	// A single node has nowhere to send its clients, so it never enters maintenance mode
	c := &nopCluster{}
	c.SetMaintenance(true)
	require.False(t, c.Maintenance())
	require.False(t, c.HasActivePeer())
	require.Equal(t, RoleActive, c.Role())
	require.True(t, c.Ready())
}
