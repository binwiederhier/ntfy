package visitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
)

func testConfig() *Config {
	return &Config{
		PrefixBitsIPv4:                32,
		PrefixBitsIPv6:                64,
		RequestLimitBurst:             60,
		RequestLimitReplenish:         5 * time.Second,
		MessageDailyLimit:             0,
		EmailLimitBurst:               16,
		EmailLimitReplenish:           time.Hour,
		AttachmentTotalSizeLimit:      100 * 1024 * 1024,
		AttachmentFileSizeLimit:       15 * 1024 * 1024,
		AttachmentExpiryDuration:      3 * time.Hour,
		AttachmentDailyBandwidthLimit: 500 * 1024 * 1024,
		MessageExpiryDuration:         12 * time.Hour,
	}
}

// TestLimitsFor_Config pins the config-based resolution: the daily message and email limits are
// derived from the replenish intervals unless set, calls and reservations are zero for IP-keyed
// visitors (phone numbers must be verified, reservations are a paid feature).
func TestLimitsFor_Config(t *testing.T) {
	conf := testConfig()
	limits := LimitsFor(conf, nil)
	require.Equal(t, BasisIP, limits.Basis)
	require.Equal(t, 60, limits.RequestLimitBurst)
	require.Equal(t, 5*time.Second, limits.RequestLimitReplenish)
	require.Equal(t, int64(17280), limits.MessageLimit) // 24h / 5s, approximated from the request replenish
	require.Equal(t, 12*time.Hour, limits.MessageExpiryDuration)
	require.Equal(t, int64(24), limits.EmailLimit) // 24h / 1h
	require.Equal(t, 16, limits.EmailLimitBurst)
	require.Equal(t, time.Hour, limits.EmailLimitReplenish)
	require.Equal(t, int64(0), limits.CallLimit)
	require.Equal(t, int64(0), limits.ReservationsLimit)
	require.Equal(t, int64(100*1024*1024), limits.AttachmentTotalSizeLimit)
	require.Equal(t, int64(15*1024*1024), limits.AttachmentFileSizeLimit)
	require.Equal(t, 3*time.Hour, limits.AttachmentExpiryDuration)
	require.Equal(t, int64(500*1024*1024), limits.AttachmentBandwidthLimit)

	// An explicit daily message limit wins over the derived one; no email burst means no emails
	conf.MessageDailyLimit = 250
	conf.EmailLimitBurst = 0
	limits = LimitsFor(conf, nil)
	require.Equal(t, int64(250), limits.MessageLimit)
	require.Equal(t, int64(0), limits.EmailLimit)
}

// TestLimitsFor_Tier pins the tier-based resolution: the request bucket is derived from the daily
// message limit but never below the config values, the email burst likewise, and the email refill
// comes from the tier alone.
func TestLimitsFor_Tier(t *testing.T) {
	conf := testConfig()
	tier := &user.Tier{
		ID:                       "ti_123",
		MessageLimit:             10000,
		MessageExpiryDuration:    48 * time.Hour,
		EmailLimit:               200,
		CallLimit:                10,
		ReservationLimit:         5,
		AttachmentFileSizeLimit:  50 * 1024 * 1024,
		AttachmentTotalSizeLimit: 1024 * 1024 * 1024,
		AttachmentExpiryDuration: 24 * time.Hour,
		AttachmentBandwidthLimit: 5 * 1024 * 1024 * 1024,
	}
	limits := LimitsFor(conf, tier)
	require.Equal(t, BasisTier, limits.Basis)
	require.Equal(t, 500, limits.RequestLimitBurst)                    // 5% of 10,000
	require.Equal(t, 24*time.Hour/20000, limits.RequestLimitReplenish) // 2 x 10,000 per day, faster than 1 per 5s
	require.Equal(t, int64(10000), limits.MessageLimit)
	require.Equal(t, 48*time.Hour, limits.MessageExpiryDuration)
	require.Equal(t, int64(200), limits.EmailLimit)
	require.Equal(t, 40, limits.EmailLimitBurst) // 20% of 200
	require.Equal(t, 24*time.Hour/200, limits.EmailLimitReplenish)
	require.Equal(t, int64(10), limits.CallLimit)
	require.Equal(t, int64(5), limits.ReservationsLimit)
	require.Equal(t, int64(1024*1024*1024), limits.AttachmentTotalSizeLimit)
	require.Equal(t, int64(50*1024*1024), limits.AttachmentFileSizeLimit)
	require.Equal(t, 24*time.Hour, limits.AttachmentExpiryDuration)
	require.Equal(t, int64(5*1024*1024*1024), limits.AttachmentBandwidthLimit)

	// A small tier never lowers the config's request bucket or email burst (floor), and a huge
	// one is capped at the burst maximum
	small := &user.Tier{MessageLimit: 100, EmailLimit: 10}
	limits = LimitsFor(conf, small)
	require.Equal(t, 60, limits.RequestLimitBurst)
	require.Equal(t, 5*time.Second, limits.RequestLimitReplenish)
	require.Equal(t, 16, limits.EmailLimitBurst)
	require.Equal(t, 24*time.Hour/10, limits.EmailLimitReplenish) // Not clamped: a tiny tier really refills slowly
	huge := &user.Tier{MessageLimit: 1000000, EmailLimit: 100000}
	limits = LimitsFor(conf, huge)
	require.Equal(t, 1000, limits.RequestLimitBurst)
	require.Equal(t, 150, limits.EmailLimitBurst)

	// A tier without emails never refills the email bucket; a tier without messages keeps the
	// config's request refill
	none := &user.Tier{MessageLimit: 0, EmailLimit: 0}
	limits = LimitsFor(conf, none)
	require.Equal(t, time.Duration(0), limits.EmailLimitReplenish)
	require.Equal(t, 5*time.Second, limits.RequestLimitReplenish)
}
