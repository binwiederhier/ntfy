package server

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/cluster/quota"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
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

func TestVisitor_BurnPeerUsage_Bandwidth(t *testing.T) {
	// Bandwidth consumed on other nodes must reduce this node's bandwidth bucket
	c := newTestConfig(t, "")
	c.VisitorAttachmentDailyBandwidthLimit = 1000
	v := newVisitor(c, newMemTestCache(t), nil, nil, netip.MustParseAddr("1.2.3.4"), nil)
	require.True(t, v.BandwidthAllowed(400))
	v.BurnPeerUsage(quota.Counters{BandwidthBytes: 500})
	require.False(t, v.BandwidthAllowed(400)) // 400 + 500 burned leaves only 100
	require.True(t, v.BandwidthAllowed(100))
}

func TestVisitor_BurnPeerUsage_Requests(t *testing.T) {
	c := newTestConfig(t, "")
	c.VisitorRequestLimitBurst = 10
	c.VisitorRequestLimitReplenish = time.Minute
	v := newVisitor(c, newMemTestCache(t), nil, nil, netip.MustParseAddr("1.2.3.4"), nil)
	v.BurnPeerUsage(quota.Counters{Requests: 8})
	for i := 0; i < 2; i++ {
		require.True(t, v.RequestAllowed())
	}
	require.False(t, v.RequestAllowed())
}

func TestVisitor_ReseedBucketsFromUsageOnCreation(t *testing.T) {
	// Rehydration (260829-topic-visitor-tables.md, piece 1): a visitor created on a freshly
	// booted node must not start with full request/bandwidth buckets when the shared usage
	// table already records consumption for its key today (restart amnesty). The reseed burns
	// min(usage, burst) -- capped at an EMPTY bucket, never boot-time debt (unlike the
	// peer-usage burn, which may take a live bucket one burst into debt).
	schemaDSN := dbtest.CreateTestPostgresSchema(t)

	// Simulate the pre-restart cluster: another node recorded today's usage
	host, err := pg.Open(schemaDSN)
	require.Nil(t, err)
	pool := db.New(host, nil)
	defer pool.Close()
	seeder, err := quota.New(&quota.Config{FlushInterval: 50 * time.Millisecond}, pool)
	require.Nil(t, err)
	seeder.Inc("ip:9.9.9.9", quota.Counters{Requests: 60, BandwidthBytes: 400})
	waitFor(t, func() bool { // Wait until the deltas are flushed to the shared table
		fresh, err := quota.New(&quota.Config{FlushInterval: time.Hour}, pool)
		require.Nil(t, err)
		defer fresh.Close()
		return fresh.Totals("ip:9.9.9.9").Requests == 60
	})
	require.Nil(t, seeder.Close())

	// "Restart": a new server boots against the same schema and sees the visitor for the
	// first time
	conf := newTestConfig(t, schemaDSN)
	conf.ClusterNodeID = "node-a"
	conf.ClusterListen = "127.0.0.1:1"
	conf.ClusterSecret = "s3cret"
	conf.ClusterAdvertiseURL = "http://127.0.0.1:1"
	conf.VisitorRequestLimitBurst = 20
	conf.VisitorRequestLimitReplenish = time.Minute
	conf.VisitorAttachmentDailyBandwidthLimit = 1000
	s := newTestServer(t, conf)
	v := s.visitor(netip.MustParseAddr("9.9.9.9"), nil)

	// 60 requests consumed today, burst 20: the bucket is reseeded to exactly empty --
	// drained (usage >= burst), but never negative (no boot-time debt)
	tokens := v.requestLimiter.Tokens()
	require.LessOrEqual(t, tokens, 0.5, "request bucket must be drained by the reseed")
	require.GreaterOrEqual(t, tokens, -0.5, "reseed must not put a fresh bucket into debt")

	// 400 of 1000 bandwidth bytes consumed today: 600 remain
	require.True(t, v.BandwidthAllowed(600))
	require.False(t, v.BandwidthAllowed(50))
}
