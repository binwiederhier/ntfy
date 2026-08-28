package quota

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/pg"
	dbtest "heckel.io/ntfy/v2/db/test"
)

func openTestPool(t *testing.T, dsn string) *db.DB {
	t.Helper()
	host, err := pg.Open(dsn)
	require.Nil(t, err)
	d := db.New(host, nil)
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestTracker creates a tracker with a huge flush interval, so tests drive
// flushAndPull manually and deterministically
func newTestTracker(t *testing.T, pool *db.DB, conf *Config) *Tracker {
	t.Helper()
	if conf == nil {
		conf = &Config{}
	}
	conf.FlushInterval = time.Hour
	tracker, err := New(conf, pool)
	require.Nil(t, err)
	t.Cleanup(func() { tracker.Close() })
	return tracker
}

func TestTracker_IncAndTotals_Local(t *testing.T) {
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	tracker := newTestTracker(t, pool, nil)
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 2})
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 1, Emails: 3})
	tracker.Inc("user:u_abc", Counters{Calls: 1})
	require.Equal(t, Counters{Messages: 3, Emails: 3}, tracker.Totals("ip:1.2.3.4"))
	require.Equal(t, Counters{Calls: 1}, tracker.Totals("user:u_abc"))
	require.Equal(t, Counters{}, tracker.Totals("ip:9.9.9.9"))
}

func TestTracker_FlushAndPull_ConvergesAcrossNodes(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	a := newTestTracker(t, poolA, nil)
	b := newTestTracker(t, poolB, nil)
	a.Inc("ip:1.2.3.4", Counters{Messages: 2, Requests: 10})
	b.Inc("ip:1.2.3.4", Counters{Messages: 3, Requests: 5})
	require.Nil(t, a.flushAndPull())
	require.Nil(t, b.flushAndPull())
	require.Nil(t, a.flushAndPull()) // A pulls B's flushed usage
	require.Equal(t, Counters{Messages: 5, Requests: 15}, a.Totals("ip:1.2.3.4"))
	require.Equal(t, Counters{Messages: 5, Requests: 15}, b.Totals("ip:1.2.3.4"))
	// Local increments on top of pulled totals are reflected immediately
	a.Inc("ip:1.2.3.4", Counters{Messages: 1})
	require.Equal(t, Counters{Messages: 6, Requests: 15}, a.Totals("ip:1.2.3.4"))
}

func TestTracker_PullReportsPeerUsage(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	poolA, poolB := openTestPool(t, schemaDSN), openTestPool(t, schemaDSN)
	peerUsage := make(map[Key]Counters)
	a := newTestTracker(t, poolA, &Config{PeerUsageFunc: func(key Key, delta Counters) {
		c := peerUsage[key]
		c.add(delta)
		peerUsage[key] = c
	}})
	b := newTestTracker(t, poolB, nil)

	// A's own usage must never be reported back to A as peer usage
	a.Inc("ip:1.2.3.4", Counters{Requests: 7})
	require.Nil(t, a.flushAndPull())
	require.Empty(t, peerUsage)

	// B's usage is reported to A exactly once, as a delta
	b.Inc("ip:1.2.3.4", Counters{Requests: 5, BandwidthBytes: 100})
	require.Nil(t, b.flushAndPull())
	require.Nil(t, a.flushAndPull())
	require.Equal(t, Counters{Requests: 5, BandwidthBytes: 100}, peerUsage["ip:1.2.3.4"])
	require.Nil(t, a.flushAndPull())
	require.Equal(t, Counters{Requests: 5, BandwidthBytes: 100}, peerUsage["ip:1.2.3.4"]) // No double report

	// Only the delta since the last pull is reported
	b.Inc("ip:1.2.3.4", Counters{Requests: 2})
	require.Nil(t, b.flushAndPull())
	require.Nil(t, a.flushAndPull())
	require.Equal(t, Counters{Requests: 7, BandwidthBytes: 100}, peerUsage["ip:1.2.3.4"])
}

func TestTracker_DayRollover(t *testing.T) {
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	tracker := newTestTracker(t, pool, nil)
	now := time.Date(2026, 8, 28, 23, 59, 0, 0, time.UTC)
	tracker.mu.Lock()
	tracker.now = func() time.Time { return now }
	tracker.day = tracker.currentDay()
	tracker.mu.Unlock()

	tracker.Inc("ip:1.2.3.4", Counters{Messages: 5})
	require.Nil(t, tracker.flushAndPull())
	require.Equal(t, Counters{Messages: 5}, tracker.Totals("ip:1.2.3.4"))

	// After midnight, totals reset; the old day's rows remain in the table
	now = time.Date(2026, 8, 29, 0, 1, 0, 0, time.UTC)
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 1}) // Still counted into the rolling maps
	require.Nil(t, tracker.flushAndPull())
	require.Equal(t, Counters{}, tracker.Totals("ip:1.2.3.4"))
	var oldDayMessages int64
	require.Nil(t, pool.QueryRow(`SELECT messages FROM visitor_usage WHERE key = 'ip:1.2.3.4' AND day = '2026-08-28'`).Scan(&oldDayMessages))
	require.GreaterOrEqual(t, oldDayMessages, int64(5))
}

func TestTracker_FailOpen_RetainsDeltasAndServesStaleTotals(t *testing.T) {
	schemaDSN := dbtest.CreateTestPostgresSchema(t)
	pool := openTestPool(t, schemaDSN)
	tracker := newTestTracker(t, pool, nil)
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 2})
	require.Nil(t, tracker.flushAndPull())

	// Break the database connection; increments must be retained and totals served stale
	pool.Close()
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 3})
	require.NotNil(t, tracker.flushAndPull())
	require.Equal(t, Counters{Messages: 5}, tracker.Totals("ip:1.2.3.4"))

	// A fresh pool sees only the flushed part; the retained deltas flush on the next cycle
	pool2 := openTestPool(t, schemaDSN)
	tracker.pool = pool2
	require.Nil(t, tracker.flushAndPull())
	var messages int64
	require.Nil(t, pool2.QueryRow(`SELECT messages FROM visitor_usage WHERE key = 'ip:1.2.3.4'`).Scan(&messages))
	require.Equal(t, int64(5), messages)
}

func TestTracker_Prune(t *testing.T) {
	pool := openTestPool(t, dbtest.CreateTestPostgresSchema(t))
	tracker := newTestTracker(t, pool, nil)
	_, err := pool.Exec(upsertUsageQuery, "ip:1.2.3.4", "2020-01-01", 1, 1, 0, 0, 0, time.Now().Unix())
	require.Nil(t, err)
	tracker.Inc("ip:1.2.3.4", Counters{Messages: 1})
	require.Nil(t, tracker.flushAndPull())
	require.Nil(t, tracker.Prune())
	var count int
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM visitor_usage WHERE day = '2020-01-01'`).Scan(&count))
	require.Equal(t, 0, count)
	require.Nil(t, pool.QueryRow(`SELECT COUNT(*) FROM visitor_usage`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestDayFor_ResetTimeBoundary(t *testing.T) {
	midnight := time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)
	twoAM := time.Date(0, 0, 0, 2, 0, 0, 0, time.UTC)
	require.Equal(t, "2026-08-28", dayFor(time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), midnight))
	require.Equal(t, "2026-08-28", dayFor(time.Date(2026, 8, 29, 1, 59, 0, 0, time.UTC), twoAM))
	require.Equal(t, "2026-08-29", dayFor(time.Date(2026, 8, 29, 2, 1, 0, 0, time.UTC), twoAM))
}
