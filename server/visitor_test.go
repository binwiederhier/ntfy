package server

import (
	"io"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
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

func TestVisitor_SharedKeyCountsOnce(t *testing.T) {
	// Two visitor objects with the same user key (the user's own visitor, and the IP-keyed one the
	// billing callback promotes with SetUser) share one count in the limiter, but each persists
	// only what it admitted itself, so the user's stats are not double counted
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthStatsQueueWriterInterval = 100 * time.Millisecond
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		v1 := s.visitor(netip.MustParseAddr("1.1.1.1"), u)
		v2 := s.visitor(netip.MustParseAddr("2.2.2.2"), nil)
		v2.SetUser(u)
		require.NotSame(t, v1, v2)
		for i := 0; i < 3; i++ {
			require.True(t, v1.MessageAllowed())
		}
		for i := 0; i < 2; i++ {
			require.True(t, v2.MessageAllowed())
		}
		require.Equal(t, int64(5), v1.Stats().Messages)
		require.Equal(t, int64(5), v2.Stats().Messages)

		// Persisted once: 3 + 2, not 5 + 5
		v1.EnqueueUserStatsDelta()
		v2.EnqueueUserStatsDelta()
		v1.EnqueueUserStatsDelta() // Nothing new since the last one
		waitFor(t, func() bool {
			u, err := s.userManager.User("phil")
			require.Nil(t, err)
			return u.Stats.Messages > 0
		})
		time.Sleep(300 * time.Millisecond) // Any double counting would have been flushed by now
		u, err = s.userManager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(5), u.Stats.Messages)

		// The account sees the shared count, and the day's remaining messages are shared too
		response := request(t, s, "GET", "/v1/account", "", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, response.Code)
		account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
		require.Nil(t, err)
		require.Equal(t, int64(5), account.Stats.Messages)
		require.Equal(t, int64(5), account.Stats.MessagesRemaining)
		for i := 0; i < 5; i++ {
			require.True(t, v2.MessageAllowed())
		}
		require.False(t, v1.MessageAllowed())
	})
}

func TestVisitor_ExpungedVisitorKeepsCounts(t *testing.T) {
	// Expunging a stale visitor object from memory is not a reset: the limiter keeps the key's
	// counts until the key itself goes idle, so a re-created visitor continues where it left off
	c := newTestConfig(t, "")
	c.VisitorMessageDailyLimit = 3
	s := newTestServer(t, c)
	ip := netip.MustParseAddr("9.9.9.9")
	v := s.visitor(ip, nil)
	require.True(t, v.MessageAllowed())
	require.True(t, v.MessageAllowed())
	v.mu.Lock()
	v.seen = time.Now().Add(-2 * visitorExpungeAfter)
	v.mu.Unlock()
	s.pruneVisitors()
	s.mu.Lock()
	require.Empty(t, s.visitors)
	s.mu.Unlock()
	require.Equal(t, 1, s.limiter.Size()) // The key was used just now, so it stays
	v2 := s.visitor(ip, nil)
	require.NotSame(t, v, v2)
	require.Equal(t, int64(2), v2.Stats().Messages)
	require.True(t, v2.MessageAllowed())
	require.False(t, v2.MessageAllowed())
}

func TestVisitor_DailyResetKeepsEmailBucket(t *testing.T) {
	// The daily reset starts the day's counts over, but it does not refill the email bucket: a
	// visitor who emptied it right before the reset still has to wait for the refill
	c := newTestConfig(t, "")
	c.VisitorMessageDailyLimit = 2
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
	response := request(t, s, "PUT", "/mytopic", "test", nil)
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42908, toHTTPError(t, response.Body.String()).Code)

	// After the reset: messages are back, emails are not
	s.resetStats()
	response = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)
	require.Equal(t, int64(2), account.Stats.MessagesRemaining)
	require.Equal(t, int64(0), account.Stats.Emails)
	require.Equal(t, int64(0), account.Stats.EmailsRemaining)
	response = request(t, s, "PUT", "/mytopic", "test", nil)
	require.Equal(t, 200, response.Code)
	response = request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Email": "test@email.com",
	})
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42902, toHTTPError(t, response.Body.String()).Code)
}

func TestVisitor_RequestLimiterZeroBurstAdmitsNothing(t *testing.T) {
	// A request burst of zero admits nothing, as the old rate.Limiter did; the limit service
	// reads a zero burst as "no bucket", so the visitor must not hand it one. A tier's burst is
	// floored at the config's, so a small tier gets zero too.
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorRequestLimitBurst = 0
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "small",
		MessageLimit: 10, // Burst 10 * 5% = 0
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "small"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")
	require.False(t, s.visitor(ip, nil).RequestAllowed())
	require.False(t, s.visitor(ip, u).RequestAllowed())
}

func TestVisitor_TopicCreationLimiterDisabledByZero(t *testing.T) {
	// The topic creation limiter is off when either its burst or its replenish interval is zero;
	// a bucket that never refills would otherwise lock a visitor out of new topics for good
	topicsAllowed := func(burst int, replenish time.Duration) bool {
		c := newTestConfig(t, "")
		c.VisitorTopicCreationLimitBurst = burst
		c.VisitorTopicCreationLimitReplenish = replenish
		s := newTestServer(t, c)
		v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
		for i := 0; i < 3; i++ {
			require.True(t, v.TopicCreationAllowed())
		}
		return v.TopicCreationAllowed() // The 4th: denied only if a 3-burst limiter is on
	}
	require.True(t, topicsAllowed(0, time.Hour))
	require.True(t, topicsAllowed(3, 0))
	require.False(t, topicsAllowed(3, time.Hour))
}

func TestVisitor_TierChangeDoesNotReseedCounts(t *testing.T) {
	// The persisted stats seed a key only on its first sight. On a tier change (and for a new
	// visitor object of a known key), the live counts win over the user's stale database row,
	// which lags behind by whatever the stats queue has not flushed yet.
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthStatsQueueWriterInterval = time.Hour // Never flushed during the test: the row stays stale
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "small",
			MessageLimit: 5,
		}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "large",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "small"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		ip := netip.MustParseAddr("9.9.9.9")
		v := s.visitor(ip, u)
		for i := 0; i < 4; i++ {
			require.True(t, v.MessageAllowed())
		}
		require.Nil(t, s.userManager.ChangeTier("phil", "large"))
		u, err = s.userManager.User("phil")
		require.Nil(t, err)
		require.Equal(t, int64(0), u.Stats.Messages) // Stale
		require.Same(t, v, s.visitor(ip, u))
		require.Equal(t, int64(4), v.Stats().Messages)
		for i := 0; i < 6; i++ {
			require.True(t, v.MessageAllowed())
		}
		require.False(t, v.MessageAllowed())
		fresh := newVisitor(s.config, s.limiter, s.messageCache, s.userManager, ip, u)
		require.Equal(t, int64(10), fresh.Stats().Messages)
		require.False(t, fresh.MessageAllowed())
	})
}

func TestVisitor_IPPrefixKeying(t *testing.T) {
	// Addresses in one prefix share the IP key and its counts, other prefixes do not, and a user
	// without a tier counts against the IP key like an anonymous request from the same address
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorPrefixBitsIPv4 = 24
	c.VisitorMessageDailyLimit = 3
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("1.2.3.4"), nil)
	require.True(t, v.MessageAllowed())
	require.True(t, v.MessageAllowed())
	peer := s.visitor(netip.MustParseAddr("1.2.3.200"), nil)
	require.Same(t, v, peer)
	require.Equal(t, int64(2), peer.Stats().Messages)
	require.True(t, s.visitor(netip.MustParseAddr("1.2.4.1"), nil).MessageAllowed())
	vu := s.visitor(netip.MustParseAddr("1.2.3.77"), u)
	require.Same(t, v, vu)
	require.True(t, vu.MessageAllowed())
	require.False(t, vu.MessageAllowed())
	require.False(t, s.visitor(netip.MustParseAddr("1.2.3.4"), nil).MessageAllowed())

	// IPv6: the default /64
	v6 := s.visitor(netip.MustParseAddr("2001:db8:1:2::1"), nil)
	require.Same(t, v6, s.visitor(netip.MustParseAddr("2001:db8:1:2:ffff::1"), nil))
	require.NotSame(t, v6, s.visitor(netip.MustParseAddr("2001:db8:1:3::1"), nil))
}

func TestVisitor_TierDowngradeReturnsToIPBucket(t *testing.T) {
	// Gaining a tier moves the user to their own key with fresh counts, losing it moves them back
	// to the IP key with whatever anonymous usage it has, and regaining it finds the user key's
	// counts as they were left
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorMessageDailyLimit = 3
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "test",
		MessageLimit: 10,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	ip := netip.MustParseAddr("9.9.9.9")
	anon := s.visitor(ip, nil)
	require.True(t, anon.MessageAllowed())
	require.True(t, anon.MessageAllowed())

	// Up: fresh counts on the user key
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	vu := s.visitor(ip, u)
	require.NotSame(t, anon, vu)
	require.Equal(t, int64(0), vu.Stats().Messages)
	for i := 0; i < 5; i++ {
		require.True(t, vu.MessageAllowed())
	}

	// Down: back on the IP key, with the anonymous usage
	require.Nil(t, s.userManager.ResetTier("phil"))
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, u.Tier)
	vd := s.visitor(ip, u)
	require.Same(t, anon, vd)
	require.Equal(t, int64(2), vd.Stats().Messages)
	require.True(t, vd.MessageAllowed())
	require.False(t, vd.MessageAllowed())

	// Up again: what was spent on the tier stays spent
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	vr := s.visitor(ip, u)
	require.Same(t, vu, vr)
	require.Equal(t, int64(5), vr.Stats().Messages)
}

func TestVisitor_ConcurrentSharedKey(t *testing.T) {
	// Visitor objects are shared per key and mutated by SetUser on every request. Under
	// contention, the daily count must stay exact (no over-admission) and the reads must not
	// race the user swaps; run with -race.
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorRequestLimitBurst = 10000
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "test",
		MessageLimit: 50,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false)) // No tier: shares the IP key
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	noTier, err := s.userManager.User("ben")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")
	const goroutines, iterations = 32, 25
	var wg sync.WaitGroup
	var admitted atomic.Int64
	errs := make(chan error, goroutines*iterations)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				v := s.visitor(ip, u)
				if v.MessageAllowed() {
					admitted.Add(1)
				}
				v.RequestAllowed()
				v.Stats()
				v.Context()
				vip := s.visitor(ip, nil)
				vip.RequestAllowed()
				vip.AuthAllowed()
				vip.Context()
				vn := s.visitor(ip, noTier)
				vn.MessageAllowed()
				vn.Context()
				if i%5 == 0 {
					if _, err := v.Info(); err != nil {
						errs <- err
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.Nil(t, err)
	}
	require.Equal(t, int64(50), admitted.Load())
	require.Equal(t, int64(50), s.visitor(ip, u).Stats().Messages)
}

// Fails if negative bandwidth AllowN becomes a no-op; a later reader limiter's rejection must refund the shared quota.
func TestVisitor_BandwidthReaderRollback(t *testing.T) {
	c := newTestConfig(t, "")
	c.VisitorAttachmentDailyBandwidthLimit = 10
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	l := v.BandwidthLimiter()
	r := util.NewLimitReader(strings.NewReader("12345678"), l, util.NewFixedLimiter(0))
	n, err := r.Read(make([]byte, 8))
	require.ErrorIs(t, err, util.ErrLimitReached)
	require.Zero(t, n)
	require.Zero(t, l.Value())
	require.True(t, v.BandwidthAllowed(10))
	require.False(t, l.Allow())
}

func TestVisitor_TierWithoutEmailsAdmitsNone(t *testing.T) {
	// A tier whose daily email limit is zero sends no emails, even though its bucket inherits the
	// server's burst; the limit service reads a daily limit of zero as "no cap"
	c := newTestConfigWithAuthFile(t, "")
	s := newTestServer(t, c)
	s.mailer = &testMailer{}
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "noemails",
		MessageLimit: 10,
		EmailLimit:   0,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "noemails"))
	response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"Email":         "test@email.com",
	})
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42902, toHTTPError(t, response.Body.String()).Code)
}

func TestVisitor_TierFieldChangeFlushesStats(t *testing.T) {
	// A tier edited in place (same ID, other limits) resets the visitor's local counts like a tier
	// change does, so what was admitted but not yet enqueued must be flushed first, or the user's
	// persisted stats lose it
	c := newTestConfigWithAuthFile(t, "")
	c.AuthStatsQueueWriterInterval = 100 * time.Millisecond
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "test",
		MessageLimit: 10,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), u)
	require.True(t, v.MessageAllowed()) // Admitted, not yet enqueued
	tier, err := s.userManager.Tier("test")
	require.Nil(t, err)
	tier.MessageLimit = 20
	require.Nil(t, s.userManager.UpdateTier(tier))
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	v.SetUser(u)
	v.EnqueueUserStatsDelta()
	time.Sleep(500 * time.Millisecond)
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(1), u.Stats.Messages)
}
