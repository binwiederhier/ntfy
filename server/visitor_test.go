package server

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestVisitor_TierUserDoesNotGetAnonymousUsage(t *testing.T) {
	// What a user's IP counted anonymously must not land on the account once the user (with a
	// tier) shows up from that IP.
	s := newTestServer(t, newTestConfigWithAuthFile(t, ""))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "test",
		MessageLimit: 100,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	for i := 0; i < 3; i++ {
		require.True(t, v.MessageAllowed())
	}

	require.Equal(t, int64(0), s.visitor(netip.MustParseAddr("9.9.9.9"), u).Usage().Count(limit.KindMessages))
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

func TestVisitor_AccountEmailsRemainingCountsTheDay(t *testing.T) {
	// The account shows the day's remaining emails, like it does for messages. The email bucket
	// may still refuse a send while the day has room left.
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
	require.Equal(t, int64(22), account.Stats.EmailsRemaining)
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
	// A tier's daily email limit (what the account page shows) is enforced, not only the bucket:
	// with a daily limit of 2 and a burst of 16, the third email is denied even though the bucket
	// has tokens
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorEmailLimitBurst = 16
	s := newTestServer(t, c)
	s.mailer = &testMailer{}
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "test",
		MessageLimit: 10,
		EmailLimit:   2,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	for i := 0; i < 2; i++ {
		response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
			"Email":         "test@email.com",
		})
		require.Equal(t, 200, response.Code)
	}
	response := request(t, s, "PUT", "/mytopic", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"Email":         "test@email.com",
	})
	require.Equal(t, 429, response.Code)
	require.Equal(t, 42902, toHTTPError(t, response.Body.String()).Code)
}

// The tests below are the single-node cases of plans/261025-shared-rate-limits.md (cases 1, 3, 4,
// 6 and 7) at the HTTP level, on both backends. The behavior itself is pinned red/green in
// package limit; these confirm the server wiring.

func TestVisitor_UsageSurvivesRestart(t *testing.T) {
	// Case 1: a graceful restart keeps the quota; a crash loses at most what was not flushed. On
	// SQLite, the usage table lives in the user database (auth-file).
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
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
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorMessageDailyLimit = 1000 // Threshold 10
		s := newTestServer(t, c)
		for i := 0; i < 2; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		}
		evictVisitor(t, s, "ip:9.9.9.9")
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		waitFor(t, func() bool {
			return s.visitor(netip.MustParseAddr("9.9.9.9"), nil).Usage().Count(limit.KindMessages) == 1
		})
	})
}

func TestVisitor_EvictedPastThresholdContinues(t *testing.T) {
	// Case 4: a key with a row comes back with its count
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.VisitorMessageDailyLimit = 100 // Threshold 1: every message counts
		s := newTestServer(t, c)
		for i := 0; i < 5; i++ {
			require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		}
		evictVisitor(t, s, "ip:9.9.9.9")
		require.Equal(t, 200, request(t, s, "PUT", "/mytopic", "test", nil).Code)
		waitFor(t, func() bool {
			return s.visitor(netip.MustParseAddr("9.9.9.9"), nil).Usage().Count(limit.KindMessages) == 6
		})
	})
}

func TestVisitor_AccountUsageFromNodeThatNeverSawTheKey(t *testing.T) {
	// Case 7: the account page shows the usage table's numbers
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 100,
			EmailLimit:   10,
			CallLimit:    3,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		other := newOtherNodeLimiter(t, s)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindMessages}, 42).OK)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindEmails}, 7).OK)
		require.True(t, other.Allow("user:"+u.ID, limit.Rule{Kind: limit.KindCalls}, 1).OK)
		other.Close()
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

// evictVisitor forgets the key in the limiter once its counts are written, as the pruner does after
// visitorExpungeAfterPersisted (the limiter keeps a key with a row for an hour, see
// limit.Service.Prune, which a test cannot wait for). Visitors are per request, so the key is all
// there is to evict.
func evictVisitor(t *testing.T, s *Server, key string) {
	t.Helper()
	waitFor(t, func() bool {
		return s.limiter.Forget(key)
	})
}

func TestVisitor_LookupAllocations(t *testing.T) {
	// The visitor lookup is on every request: it must not rebuild the keying config, and an
	// IP-keyed visitor shares the config-based limits instead of resolving its own copy
	s := newTestServer(t, newTestConfig(t, ""))
	ip := netip.MustParseAddr("1.2.3.4")
	s.visitor(ip, nil)
	allocs := testing.AllocsPerRun(1000, func() {
		s.visitor(ip, nil)
	})
	require.LessOrEqual(t, allocs, 3.0) // The visitor (built per request) and its key string ("ip:" + address)
	other := s.visitor(netip.MustParseAddr("5.6.7.8"), nil)
	require.Same(t, s.visitor(ip, nil).Limits(), other.Limits())
}

func BenchmarkVisitor_Lookup(b *testing.B) {
	conf := NewConfig()
	conf.CacheDuration = 0
	s, err := New(conf)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Stop()
	ip := netip.MustParseAddr("1.2.3.4")
	s.visitor(ip, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.visitor(ip, nil)
	}
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

func TestVisitor_SlowUsageLoadDoesNotHoldTheVisitorLock(t *testing.T) {
	// Codex review #4: EmailAllowed and CallAllowed read the key's usage row before the first
	// decision, and held the visitor's lock meanwhile. Keepalive, called with the server lock held,
	// then waited too, and with it every other request's visitor lookup. Visitors have no lock
	// since they are per request; this still pins that the read holds no lock Keepalive needs.
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		s := newTestServer(t, c)
		release := lockUsageTable(t, s)
		v := s.visitor(netip.MustParseAddr("7.7.7.7"), nil)
		go v.EmailAllowed() // Blocks on the locked table
		time.Sleep(200 * time.Millisecond)
		start := time.Now()
		v.Keepalive()
		require.Less(t, time.Since(start), time.Second)
		release()
	})
}

// lockUsageTable locks the usage table (or the given tables of the user database) from another
// connection until the returned func is called (or the test ends), so every access blocks. On
// SQLite the whole file is locked.
func lockUsageTable(t *testing.T, s *Server, tables ...string) func() {
	t.Helper()
	ctx := context.Background()
	var d *sql.DB
	if len(tables) == 0 {
		tables = []string{"usage"}
	}
	statements := []string{`BEGIN`, `LOCK TABLE ` + strings.Join(tables, ", ") + ` IN ACCESS EXCLUSIVE MODE`}
	if s.db != nil {
		d = s.db.Primary()
	} else {
		var err error
		d, err = sql.Open("sqlite3", s.config.AuthFile) // The usage table is in the user database
		require.Nil(t, err)
		t.Cleanup(func() { d.Close() })
		statements = []string{`BEGIN EXCLUSIVE`}
	}
	conn, err := d.Conn(ctx)
	require.Nil(t, err)
	for _, statement := range statements {
		_, err := conn.ExecContext(ctx, statement)
		require.Nil(t, err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			conn.ExecContext(ctx, `ROLLBACK`)
			conn.Close()
		})
	}
	t.Cleanup(release) // Runs before the server's cleanup (last in, first out), which would wait for the lock
	return release
}

func TestVisitor_UserIsPerRequest(t *testing.T) {
	// Untiered users and anonymous requests from one IP share a limiter key, but each request
	// must keep its own user (#1051)
	s := newTestServer(t, newTestConfigWithAuthFile(t, ""))
	require.Nil(t, s.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")

	// The concurrent requests, made deterministic
	authed := s.visitor(ip, u)
	anon := s.visitor(ip, nil)
	require.NotNil(t, authed.User())
	require.Equal(t, u.ID, authed.User().ID)
	require.Nil(t, anon.User())
	authedAgain := s.visitor(ip, u)
	require.Nil(t, anon.User())
	require.NotNil(t, authedAgain.User())
	require.Equal(t, u.ID, authedAgain.User().ID)

	// Limits are still shared per IP: requests and the node-local subscription gauge
	for authed.RequestAllowed() {
	}
	require.False(t, anon.RequestAllowed())
	for authed.SubscriptionAllowed() {
	}
	require.False(t, anon.SubscriptionAllowed())
	authed.RemoveSubscription()
	require.True(t, anon.SubscriptionAllowed())
}

func TestVisitor_SubscriptionLimitSharedPerKey(t *testing.T) {
	c := newTestConfig(t, "")
	c.VisitorSubscriptionLimit = 2
	s := newTestServer(t, c)
	ip := netip.MustParseAddr("9.9.9.9")
	v1 := s.visitor(ip, nil)
	v2 := s.visitor(ip, nil) // A second request from the same IP shares the slots
	require.True(t, v1.SubscriptionAllowed())
	require.True(t, v2.SubscriptionAllowed())
	require.False(t, v1.SubscriptionAllowed())
	require.True(t, s.visitor(netip.MustParseAddr("1.2.3.4"), nil).SubscriptionAllowed())
	v1.RemoveSubscription()
	require.True(t, v2.SubscriptionAllowed())
}

func TestVisitor_FirebasePenaltyIsSharedPerKey(t *testing.T) {
	s := newTestServer(t, newTestConfig(t, ""))
	ip := netip.MustParseAddr("9.9.9.9")
	v1 := s.visitor(ip, nil)
	v2 := s.visitor(ip, nil)
	require.True(t, v2.FirebaseAllowed())
	v1.FirebaseTemporarilyDeny()
	require.False(t, v2.FirebaseAllowed())
	require.True(t, s.visitor(netip.MustParseAddr("1.2.3.4"), nil).FirebaseAllowed())
}

func TestVisitor_FirebasePenaltyExpires(t *testing.T) {
	c := newTestConfig(t, "")
	c.FirebaseQuotaExceededPenaltyDuration = 300 * time.Millisecond
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	v.FirebaseTemporarilyDeny()
	require.False(t, v.FirebaseAllowed())
	require.Eventually(t, v.FirebaseAllowed, 3*time.Second, 50*time.Millisecond) // Sends resume once the penalty has passed
}

func TestVisitor_AuthLimiter_NotBypassedByConcurrentLogin(t *testing.T) {
	// A user without a tier shares the IP limiter key with anonymous requests from the same IP.
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

func TestVisitor_RateVisitorKeptForExemptSubscriber(t *testing.T) {
	// A subscriber from a request-limit-exempt prefix spends nothing before it becomes a topic's
	// rate visitor; the topic must still keep it. Fails if a new rate visitor counts as stale.
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	c.VisitorRequestExemptPrefixes = []netip.Prefix{netip.MustParsePrefix("9.9.9.0/24")}
	s := newTestServer(t, c)
	_, err := s.topicsFromIDs(s.visitor(netip.MustParseAddr("1.2.3.4"), nil), "mytopic") // The topic exists already
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	topics, err := s.topicsFromIDs(v, "mytopic")
	require.Nil(t, err)
	s.setRateVisitors(httptest.NewRequest("GET", "/mytopic/json", nil), v, topics)
	require.NotNil(t, topics[0].RateVisitor())
}

func TestVisitor_AuthAndAccountLimitsIgnoreUser(t *testing.T) {
	// The auth and account limits are per key and do not look at the user (as on main since #2119):
	// a logged-in user may request a password reset, and is charged like everyone on the key
	c := newTestConfigWithAuthFile(t, "")
	c.VisitorAuthFailureLimitBurst = 1
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), u)
	require.True(t, v.AccountActionAllowed())
	require.True(t, v.AuthAllowed())
	v.AuthFailed()
	require.False(t, v.AuthAllowed())
}

func TestVisitor_SharedKeyCountsOnce(t *testing.T) {
	// Two visitors with the same user key (the user from two addresses) share one count in the
	// limiter, and each admitted unit is persisted once, so the user's stats are not double counted
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		require.Nil(t, s.userManager.AddTier(&user.Tier{
			Code:         "test",
			MessageLimit: 10,
		}))
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		require.Nil(t, s.userManager.ChangeTier("phil", "test"))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)
		v1 := s.visitor(netip.MustParseAddr("1.1.1.1"), u)
		v2 := s.visitor(netip.MustParseAddr("2.2.2.2"), u)
		require.Equal(t, v1.key, v2.key)
		for i := 0; i < 3; i++ {
			require.True(t, v1.MessageAllowed())
		}
		for i := 0; i < 2; i++ {
			require.True(t, v2.MessageAllowed())
		}
		require.Equal(t, int64(5), v1.Usage().Count(limit.KindMessages))
		require.Equal(t, int64(5), v2.Usage().Count(limit.KindMessages))

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

func TestVisitor_RolloverKeepsEmailBucket(t *testing.T) {
	// The daily rollover starts the counts over, but it does not refill the email bucket: a
	// visitor who emptied it right before the rollover still has to wait for the refill
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

	// After the rollover: messages are back, emails are not
	s.limiter.ShiftClockForTest(24 * time.Hour)
	response = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, response.Code)
	account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(response.Body))
	require.Nil(t, err)
	require.Equal(t, int64(0), account.Stats.Messages)
	require.Equal(t, int64(2), account.Stats.MessagesRemaining)
	require.Equal(t, int64(0), account.Stats.Emails)
	require.Equal(t, int64(24), account.Stats.EmailsRemaining)
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
	// A tier change keeps the key's counts: the next request's visitor carries the new tier's
	// limits and continues where the old one left off, nothing is read from the user row.
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
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
		v = s.visitor(ip, u) // The next request carries the new tier
		require.Equal(t, int64(4), v.Usage().Count(limit.KindMessages))
		for i := 0; i < 6; i++ {
			require.True(t, v.MessageAllowed())
		}
		require.False(t, v.MessageAllowed())
		fresh := s.visitor(ip, u)
		require.Equal(t, int64(10), fresh.Usage().Count(limit.KindMessages))
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
	require.Equal(t, v.key, peer.key)
	require.Equal(t, int64(2), peer.Usage().Count(limit.KindMessages))
	require.True(t, s.visitor(netip.MustParseAddr("1.2.4.1"), nil).MessageAllowed())
	vu := s.visitor(netip.MustParseAddr("1.2.3.77"), u)
	require.Equal(t, v.key, vu.key)
	require.True(t, vu.MessageAllowed())
	require.False(t, vu.MessageAllowed())
	require.False(t, s.visitor(netip.MustParseAddr("1.2.3.4"), nil).MessageAllowed())

	// IPv6: the default /64
	v6 := s.visitor(netip.MustParseAddr("2001:db8:1:2::1"), nil)
	require.Equal(t, v6.key, s.visitor(netip.MustParseAddr("2001:db8:1:2:ffff::1"), nil).key)
	require.NotEqual(t, v6.key, s.visitor(netip.MustParseAddr("2001:db8:1:3::1"), nil).key)
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
	require.NotEqual(t, anon.key, vu.key)
	require.Equal(t, int64(0), vu.Usage().Count(limit.KindMessages))
	for i := 0; i < 5; i++ {
		require.True(t, vu.MessageAllowed())
	}

	// Down: back on the IP key, with the anonymous usage
	require.Nil(t, s.userManager.ResetTier("phil"))
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, u.Tier)
	vd := s.visitor(ip, u)
	require.Equal(t, anon.key, vd.key)
	require.Equal(t, int64(2), vd.Usage().Count(limit.KindMessages))
	require.True(t, vd.MessageAllowed())
	require.False(t, vd.MessageAllowed())

	// Up again: what was spent on the tier stays spent
	require.Nil(t, s.userManager.ChangeTier("phil", "test"))
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	vr := s.visitor(ip, u)
	require.Equal(t, vu.key, vr.key)
	require.Equal(t, int64(5), vr.Usage().Count(limit.KindMessages))
}

func TestVisitor_ConcurrentSharedKey(t *testing.T) {
	// Every request builds its own visitor on a shared key. Under contention, the daily count
	// must stay exact (no over-admission) and the reads must not race the writes; run with -race.
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
				v.Usage()
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
	require.Equal(t, int64(50), s.visitor(ip, u).Usage().Count(limit.KindMessages))
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

func TestVisitor_RateVisitorLivenessIsTheSubscribers(t *testing.T) {
	// A topic keeps its rate visitor for as long as the subscriber is around (its keepalive), not
	// for as long as its limiter key is in use: the limiter forgetting the key must not drop it,
	// and publishers charging the key must not keep it
	c := newTestConfig(t, "")
	c.VisitorSubscriberRateLimiting = true
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	topics, err := s.topicsFromIDs(v, "mytopic")
	require.Nil(t, err)
	s.setRateVisitors(httptest.NewRequest("GET", "/mytopic/json", nil), v, topics)
	s.limiter.Prune(0) // The key is gone, the subscriber is not
	require.NotNil(t, topics[0].RateVisitor())
	ageRateVisitor(topics[0], 2*visitorExpungeAfter)               // The subscriber is gone
	require.True(t, s.limiter.Allow(v.key, v.messageRule(), 1).OK) // A publisher charges the key
	require.Nil(t, topics[0].RateVisitor())
}

func TestVisitor_EmailsWithoutTierHaveNoDailyCap(t *testing.T) {
	// Without a tier, emails are limited by the email bucket alone, as before: the daily limit derived
	// from the refill (24 with a refill of an hour) is not a cap, so a full burst of 30 can be sent
	c := newTestConfig(t, "")
	c.VisitorEmailLimitBurst = 30
	c.VisitorEmailLimitReplenish = time.Hour
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	for i := 0; i < 30; i++ {
		require.True(t, v.EmailAllowed(), "email %d", i)
	}
	require.False(t, v.EmailAllowed())
}

func TestVisitor_EmailsWithoutTierAllowedWithSlowRefill(t *testing.T) {
	// A refill slower than a day derives a daily limit of zero, which must not turn off email
	// without a tier: the bucket alone limits it
	c := newTestConfig(t, "")
	c.VisitorEmailLimitBurst = 2
	c.VisitorEmailLimitReplenish = 48 * time.Hour
	s := newTestServer(t, c)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)
	require.True(t, v.EmailAllowed())
	require.True(t, v.EmailAllowed())
	require.False(t, v.EmailAllowed())
}
