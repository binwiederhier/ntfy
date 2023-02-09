package user

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPermission(t *testing.T) {
	require.Equal(t, PermissionReadWrite, NewPermission(true, true))
	require.Equal(t, PermissionRead, NewPermission(true, false))
	require.Equal(t, PermissionWrite, NewPermission(false, true))
	require.Equal(t, PermissionDenyAll, NewPermission(false, false))
}
