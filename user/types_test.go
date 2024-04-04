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
	require.True(t, PermissionReadWrite.IsReadWrite())
	require.True(t, PermissionReadWrite.IsRead())
	require.True(t, PermissionReadWrite.IsWrite())
	require.True(t, PermissionRead.IsRead())
	require.True(t, PermissionWrite.IsWrite())
}

func TestParsePermission(t *testing.T) {
	_, err := ParsePermission("no")
	require.NotNil(t, err)

	p, err := ParsePermission("read-write")
	require.Nil(t, err)
	require.Equal(t, PermissionReadWrite, p)

	p, err = ParsePermission("rw")
	require.Nil(t, err)
	require.Equal(t, PermissionReadWrite, p)

	p, err = ParsePermission("read-only")
	require.Nil(t, err)
	require.Equal(t, PermissionRead, p)

	p, err = ParsePermission("WRITE")
	require.Nil(t, err)
	require.Equal(t, PermissionWrite, p)

	p, err = ParsePermission("deny-all")
	require.Nil(t, err)
	require.Equal(t, PermissionDenyAll, p)
}

func TestAllowedTier(t *testing.T) {
	require.False(t, AllowedTier("  no"))
	require.True(t, AllowedTier("yes"))
}

func TestTierContext(t *testing.T) {
	tier := &Tier{
		ID:                   "ti_abc",
		Code:                 "pro",
		StripeMonthlyPriceID: "price_123",
		StripeYearlyPriceID:  "price_456",
	}
	context := tier.Context()
	require.Equal(t, "ti_abc", context["tier_id"])
	require.Equal(t, "pro", context["tier_code"])
	require.Equal(t, "price_123", context["stripe_monthly_price_id"])
	require.Equal(t, "price_456", context["stripe_yearly_price_id"])

}

func TestUsernameRegex(t *testing.T) {
	username := "phil"
	username_email := "phil@ntfy.sh"
	username_email_alias := "phil+alias@ntfy.sh"
	username_invalid := "phil\rocks"

	require.True(t, AllowedUsername(username))
	require.True(t, AllowedUsername(username_email))
	require.True(t, AllowedUsername(username_email_alias))
	require.False(t, AllowedUsername(username_invalid))
}
