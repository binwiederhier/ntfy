package visitor

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
)

func TestKey(t *testing.T) {
	confWithDefaults := &Config{
		PrefixBitsIPv4: 32,
		PrefixBitsIPv6: 64,
	}
	confWithShortenedPrefixes := &Config{
		PrefixBitsIPv4: 16,
		PrefixBitsIPv6: 56,
	}
	userWithTier := &user.User{
		ID:   "u_123",
		Tier: &user.Tier{},
	}
	userWithoutTier := &user.User{
		ID: "u_456",
	}
	require.Equal(t, "ip:1.2.3.4", Key(netip.MustParseAddr("1.2.3.4"), nil, confWithDefaults))
	require.Equal(t, "ip:2a01:599:b26:2397::", Key(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), nil, confWithDefaults))
	require.Equal(t, "ip:2001:db8:25:86::", Key(netip.MustParseAddr("2001:db8:25:86:1::1"), nil, confWithDefaults))
	require.Equal(t, "ip:2001:db8:25:86::", Key(netip.MustParseAddr("2001:db8:25:86:2::1"), nil, confWithDefaults))

	// Only a tier promotes a visitor to a user key; a user without one shares the IP key
	require.Equal(t, "user:u_123", Key(netip.MustParseAddr("1.2.3.4"), userWithTier, confWithDefaults))
	require.Equal(t, "user:u_123", Key(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), userWithTier, confWithDefaults))
	require.Equal(t, "ip:1.2.3.4", Key(netip.MustParseAddr("1.2.3.4"), userWithoutTier, confWithDefaults))

	require.Equal(t, "ip:1.2.0.0", Key(netip.MustParseAddr("1.2.3.4"), nil, confWithShortenedPrefixes))
	require.Equal(t, "ip:2a01:599:b26:2300::", Key(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), nil, confWithShortenedPrefixes))
}
