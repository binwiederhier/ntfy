package server

import (
	"io"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestVisitor_UserStatsDeltaFlush(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthStatsQueueWriterInterval = 300 * time.Millisecond
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 100,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))

		// Publish 3 messages; the visitor enqueues a stats delta after each publish
		for i := 0; i < 3; i++ {
			response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, response.Code)
		}

		// After two flush intervals, the persisted count must be exactly 3: per-publish
		// enqueues must be deltas, not absolute snapshots (snapshots would add up to 1+2+3=6)
		time.Sleep(time.Second)
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(3), u.Stats.Messages)
	})
}

func TestVisitor_SetUser_DoesNotChargeAnonymousUsage(t *testing.T) {
	// A user who gets a tier takes over their IP-keyed visitor (e.g. the billing callback). What
	// that visitor counted anonymously must not land on the account.
	c := newTestConfigWithAuthFile(t, "")
	c.AuthStatsQueueWriterInterval = 100 * time.Millisecond
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "test", MessageLimit: 100}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	for i := 0; i < 3; i++ {
		require.True(t, v.MessageAllowed())
	}

	v.SetUser(u)
	time.Sleep(500 * time.Millisecond)
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(0), u.Stats.Messages)
}

func TestVisitor_BandwidthRefund(t *testing.T) {
	// A revert on the bandwidth limiter (what util.LimitReader and util.LimitWriter do when a
	// later limiter rejects a chunk) hands the bytes back; it used to be a no-op
	c := newTestConfig(t, "")
	c.VisitorAttachmentDailyBandwidthLimit = 1000
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	l := v.BandwidthLimiter()
	require.True(t, l.AllowN(900))
	require.False(t, l.AllowN(200))
	require.True(t, l.AllowN(-500))
	require.True(t, l.AllowN(200))
	require.False(t, v.BandwidthAllowed(500))
	require.True(t, v.BandwidthAllowed(400))
}

func TestVisitor_TierChangeKeepsBuckets(t *testing.T) {
	// Moving between tiers re-resolves the limits on the next request, but the request bucket is
	// not handed back full: what the user spent on the old tier stays spent
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorRequestLimitBurst = 3
		c.VisitorRequestLimitReplenish = time.Hour
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "small",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "large",
			MessageLimit: 10000,
		})) // Burst 500
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "small"))
		for i := 0; i < 3; i++ {
			response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, response.Code)
		}
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 429, response.Code)
		require.Nil(t, s.userManager.ChangeTier("phil", "large"))
		response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code) // 4 of 500 spent, not a full bucket
		require.Nil(t, s.userManager.ChangeTier("phil", "small"))
		response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 429, response.Code) // 4 of 3 spent: in debt
	})
}

func TestVisitor_AccountEmailsRemainingFollowsBucket(t *testing.T) {
	// The account page shows what the user can send now: no more than the email bucket holds,
	// even when the daily limit has room left (e.g. right after the daily reset)
	c := newTestConfig(t, "")
	c.VisitorEmailLimitBurst = 2
	c.VisitorEmailLimitReplenish = time.Hour // Daily limit 24
	s := newTestServer(t, c)
	s.mailer = &testMailer{}
	for i := 0; i < 2; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Email": "test@email.com",
		})
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(2), account.Stats.Emails)
	require.Equal(t, int64(0), account.Stats.EmailsRemaining)
	response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Email": "test@email.com",
	})
	require.Equal(t, 429, response.Code)
}

func TestVisitor_TierFieldChangeAppliesAtOnce(t *testing.T) {
	// A changed limit on the user's existing tier applies on the next request, without the tier
	// ID changing; it used to need a visitor rebuild
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 2,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))
		for i := 0; i < 2; i++ {
			response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
				"Authorization": util.BasicAuth("phil", "phil"),
			})
			require.Equal(t, 200, response.Code)
		}
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 429, response.Code)
		tier, err := s.userManager.Tier("test")
		require.Nil(t, err)
		tier.MessageLimit = 3
		require.Nil(t, s.userManager.UpdateTier(tier))
		response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
		response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 429, response.Code)
	})
}

func TestVisitor_EmailDailyLimitCapsBelowBurst(t *testing.T) {
	// The daily email limit (what the account page shows) is enforced, not only the bucket: with
	// a burst of 16 and a refill every 12 hours, the day's limit is 2, and the third email is
	// denied even though the bucket has tokens
	c := newTestConfig(t, "")
	c.VisitorEmailLimitBurst = 16
	c.VisitorEmailLimitReplenish = 12 * time.Hour
	s := newTestServer(t, c)
	s.mailer = &testMailer{}
	for i := 0; i < 2; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Email": "test@email.com",
		})
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Email": "test@email.com",
	})
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42902, toHTTPError(t, response.Body.String()).Code)
}

func TestVisitor_ZeroLimitsAdmitNothing(t *testing.T) {
	// A limit of zero admits nothing, as main's FixedLimiter(0) and bytes limiter did; the limit
	// service reads a zero cap as "no cap", so the visitor must not hand it a zero (Codex review #3:
	// a user downgraded to a tier without calls kept calling)
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:                     "zero",
			MessageLimit:             0,
			CallLimit:                0,
			AttachmentBandwidthLimit: 0,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "zero"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		v := s.visitor(netip.MustParseAddr("9.9.9.9"), u)
		require.False(t, v.MessageAllowed())
		require.False(t, v.CallAllowed())
		require.False(t, v.BandwidthAllowed(1))
		require.False(t, v.BandwidthLimiter().AllowN(1))
	})
}

func TestVisitor_AuthLimiter_NotBypassedByConcurrentLogin(t *testing.T) {
	// A user without a tier shares the IP-keyed visitor with anonymous requests from the same IP.
	// Their login must not switch off the auth failure limiter for a concurrent guesser (#1051).
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorAuthFailureLimitBurst = 3
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")
	vip := s.visitor(ip, nil) // The guesser's anonymous visitor (see maybeAuthenticate)

	// The user logs in between the guesser's lookup and each failed guess
	for i := 0; i < 3; i++ {
		s.visitor(ip, u)
		require.True(t, vip.AuthAllowed())
		vip.AuthFailed()
	}
	s.visitor(ip, u)
	require.False(t, vip.AuthAllowed())
}

func TestVisitor_AccountLimiter_NotDeniedByConcurrentLogin(t *testing.T) {
	// The other direction: a concurrent login on the same IP must not deny an anonymous signup
	c := newTestConfigWithAuthFile(t, "")
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")
	vip := s.visitor(ip, nil)

	s.visitor(ip, u)
	require.True(t, vip.AccountActionAllowed())
}
