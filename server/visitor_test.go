package server

import (
	"io"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/limit"
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
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "small", MessageLimit: 10}))
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "large", MessageLimit: 10000})) // Burst 500
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
		require.Equal(t, 429, response.Code) // Still empty: the larger burst refills nothing
	})
}

func TestVisitor_TierFieldChangeAppliesAtOnce(t *testing.T) {
	// A changed limit on the user's existing tier applies on the next request, without the tier
	// ID changing; it used to need a visitor rebuild
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "test", MessageLimit: 2}))
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

// The tests below are the single-node cases of plans/261025-shared-rate-limits.md (cases 1, 3, 4,
// 6 and 7) at the HTTP level, on both backends. The behavior itself is pinned red/green in
// package limit; these confirm the server wiring.

func TestVisitor_UsageSurvivesRestart(t *testing.T) {
	// Case 1: a graceful restart keeps the quota; a crash loses at most what was not flushed
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorMessageDailyLimit = 3
		c.UsageFlushInterval = time.Hour // Nothing is flushed but by Stop
		s := newTestServer(t, c)
		for i := 0; i < 3; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		}
		require.Equal(t, 429, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		s.Stop() // Graceful: writes the counts

		// The new server never saw the visitor; the account page reads its row, and the quota holds
		s = newTestServer(t, c)
		rr := request(t, s, "GET", "/v1/account", "", nil)
		require.Equal(t, 200, rr.Code)
		account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, int64(3), account.Stats.Messages)
		require.Equal(t, int64(0), account.Stats.MessagesRemaining)
		require.Equal(t, 429, request(t, s, "PUT", "/mytopic", "test", nil).Code)

		// A crash (no Stop) loses what was not flushed: the next server starts at the written count
		crashed := newTestServer(t, c)
		require.Equal(t, 200, request(t, crashed, "PUT", "/othertopic", "test", nil, func(r *http.Request) {
			r.RemoteAddr = "5.5.5.5:1234"
		}).Code)
		fresh := newTestServer(t, c)
		rr = request(t, fresh, "GET", "/v1/account", "", nil, func(r *http.Request) {
			r.RemoteAddr = "5.5.5.5:1234"
		})
		require.Equal(t, 200, rr.Code)
		account, err = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, int64(0), account.Stats.Messages)
	})
}

func TestVisitor_EvictedBelowThresholdResumesAtZero(t *testing.T) {
	// Case 3: a key without a row (below 1% of the quota) is evicted after a few minutes and
	// comes back at zero, as designed
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorMessageDailyLimit = 1000 // Threshold 10
		s := newTestServer(t, c)
		for i := 0; i < 2; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		}
		evictVisitor(t, s, "ip:9.9.9.9")
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		waitFor(t, func() bool {
			return s.visitor(netip.MustParseAddr("9.9.9.9"), nil).Stats().Messages == 1
		})
	})
}

func TestVisitor_EvictedPastThresholdContinues(t *testing.T) {
	// Case 4: a key with a row comes back with its count
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorMessageDailyLimit = 100 // Threshold 1: every message counts
		s := newTestServer(t, c)
		for i := 0; i < 5; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		}
		evictVisitor(t, s, "ip:9.9.9.9")
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		waitFor(t, func() bool {
			return s.visitor(netip.MustParseAddr("9.9.9.9"), nil).Stats().Messages == 6
		})
	})
}

func TestVisitor_EmailQuotaSurvivesEviction(t *testing.T) {
	// Case 6: the reason visitors were kept 24 h; the usage row covers it now, and emails read
	// their row before the first decision, so not even one slips through
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfig(t, databaseURL)
		c.VisitorEmailLimitBurst = 5
		c.VisitorEmailLimitReplenish = 24 * time.Hour / 5 // Daily limit 5
		s := newTestServer(t, c)
		s.mailer = &testMailer{}
		for i := 0; i < 5; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", map[string]string{"Email": "test@email.com"}).Code)
		}
		require.Equal(t, 429, request(t, s, "PUT", "/mytopic", "test", map[string]string{"Email": "test@email.com"}).Code)
		evictVisitor(t, s, "ip:9.9.9.9")
		rr := request(t, s, "PUT", "/mytopic", "test", map[string]string{"Email": "test@email.com"})
		require.Equal(t, 429, rr.Code)
		require.Equal(t, 42902, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestVisitor_AccountUsageFromNodeThatNeverSawTheKey(t *testing.T) {
	// Case 7: the account page shows the usage table's numbers
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{Code: "test", MessageLimit: 100, EmailLimit: 10, CallLimit: 3}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		other := newOtherNodeLimiter(t, s)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindMessages}, 42).OK)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindEmails}, 7).OK)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindCalls}, 1).OK)
		require.Nil(t, other.Close())
		rr := request(t, s, "GET", "/v1/account", "", map[string]string{"Authorization": util.BasicAuth("phil", "phil")})
		require.Equal(t, 200, rr.Code)
		account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, int64(42), account.Stats.Messages)
		require.Equal(t, int64(58), account.Stats.MessagesRemaining)
		require.Equal(t, int64(7), account.Stats.Emails)
		require.Equal(t, int64(1), account.Stats.Calls)
	})
}

// evictVisitor ages the visitor past the eviction time and runs the pruner, then forgets the key
// in the limiter once its counts are written (the limiter keeps a key with a row for an hour,
// see limit.Service.Prune, which a test cannot wait for)
func evictVisitor(t *testing.T, s *Server, key string) {
	t.Helper()
	s.mu.Lock()
	v, ok := s.visitors[key]
	s.mu.Unlock()
	require.True(t, ok)
	v.mu.Lock()
	v.seen = time.Now().Add(-2 * visitorExpungeAfterPersisted)
	v.mu.Unlock()
	s.pruneVisitors()
	s.mu.RLock()
	_, still := s.visitors[key]
	s.mu.RUnlock()
	require.False(t, still)
	waitFor(t, func() bool {
		return s.limiter.Forget(key)
	})
}
