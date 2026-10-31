//go:build !noserver && !windows

package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLI_Backend_TrustedAdminSocketOwner(t *testing.T) {
	require.True(t, trustedAdminSocketOwner(0, []int{0, 1000}))     // Root
	require.True(t, trustedAdminSocketOwner(1000, []int{0, 1000}))  // Me
	require.True(t, trustedAdminSocketOwner(999, []int{0, 0, 999})) // The server's user (owns the directory or the auth-file)
	require.False(t, trustedAdminSocketOwner(1001, []int{0, 1000, 999}))
}
