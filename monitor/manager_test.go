package monitor

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestManager(t *testing.T) *Manager {
	m, err := NewSQLiteManager(filepath.Join(t.TempDir(), "monitor.db"), "")
	require.Nil(t, err)
	t.Cleanup(func() { m.Close() })
	return m
}

func TestManager_AddMonitor_Success(t *testing.T) {
	m := newTestManager(t)
	mon, err := m.AddMonitor("u_alice", "scanner", 300, 60, "alerts", DefaultAlertPriority)
	require.Nil(t, err)
	require.NotZero(t, mon.ID)
	require.Equal(t, "u_alice", mon.UserID)
	require.Equal(t, "scanner", mon.Key)
	require.Equal(t, int64(300), mon.PeriodSeconds)
	require.Equal(t, int64(60), mon.GraceSeconds)
	require.Equal(t, "alerts", mon.AlertTopic)
	require.Equal(t, DefaultAlertPriority, mon.AlertPriority)
	require.Equal(t, StatePending, mon.State)
	require.Zero(t, mon.LastSeenAt)
	require.NotZero(t, mon.CreatedAt)
}

func TestManager_AddMonitor_DuplicateKeySameUser(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 300, 60, "alerts", 3)
	require.Nil(t, err)
	_, err = m.AddMonitor("u_alice", "scanner", 300, 60, "alerts", 3)
	require.ErrorIs(t, err, ErrMonitorExists)
}

func TestManager_AddMonitor_DuplicateKeyAcrossUsersAllowed(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 300, 60, "alerts", 3)
	require.Nil(t, err)
	_, err = m.AddMonitor("u_bob", "scanner", 300, 60, "alerts", 3)
	require.Nil(t, err)
}

func TestManager_AddMonitor_Validation(t *testing.T) {
	m := newTestManager(t)
	cases := []struct {
		name    string
		userID  string
		key     string
		period  int64
		grace   int64
		topic   string
		prio    int
		wantErr error
	}{
		{"empty user", "", "k", 60, 30, "alerts", 3, ErrInvalidUserID},
		{"bad key", "u_a", "bad/key", 60, 30, "alerts", 3, ErrInvalidKey},
		{"period too small", "u_a", "k", 1, 30, "alerts", 3, ErrInvalidPeriod},
		{"period too large", "u_a", "k", 1_000_000, 30, "alerts", 3, ErrInvalidPeriod},
		{"grace too small", "u_a", "k", 60, 0, "alerts", 3, ErrInvalidGrace},
		{"grace too large", "u_a", "k", 60, 1_000_000, "alerts", 3, ErrInvalidGrace},
		{"bad topic", "u_a", "k", 60, 30, "bad/topic", 3, ErrInvalidAlertTopic},
		{"prio too low", "u_a", "k", 60, 30, "alerts", 0, ErrInvalidAlertPriority},
		{"prio too high", "u_a", "k", 60, 30, "alerts", 6, ErrInvalidAlertPriority},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.AddMonitor(tc.userID, tc.key, tc.period, tc.grace, tc.topic, tc.prio)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestManager_GetMonitor_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.GetMonitor("u_nobody", "ghost")
	require.ErrorIs(t, err, ErrMonitorNotFound)
}

func TestManager_GetMonitor_RoundTrip(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 300, 60, "alerts", 4)
	require.Nil(t, err)
	got, err := m.GetMonitor("u_alice", "scanner")
	require.Nil(t, err)
	require.Equal(t, "scanner", got.Key)
	require.Equal(t, "u_alice", got.UserID)
}

func TestManager_ListMonitorsByUser_ScopedAndSorted(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "z", 60, 30, "a", 3)
	require.Nil(t, err)
	_, err = m.AddMonitor("u_alice", "a", 60, 30, "a", 3)
	require.Nil(t, err)
	_, err = m.AddMonitor("u_bob", "x", 60, 30, "a", 3)
	require.Nil(t, err)

	list, err := m.ListMonitorsByUser("u_alice")
	require.Nil(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "a", list[0].Key)
	require.Equal(t, "z", list[1].Key)
}

func TestManager_DeleteMonitor(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 60, 30, "a", 3)
	require.Nil(t, err)
	require.Nil(t, m.DeleteMonitor("u_alice", "scanner"))
	_, err = m.GetMonitor("u_alice", "scanner")
	require.ErrorIs(t, err, ErrMonitorNotFound)
	require.ErrorIs(t, m.DeleteMonitor("u_alice", "scanner"), ErrMonitorNotFound)
}

func TestManager_RecordHeartbeat_PendingToUp(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 60, 30, "alerts", 3)
	require.Nil(t, err)

	mon, prev, err := m.RecordHeartbeat("u_alice", "scanner")
	require.Nil(t, err)
	require.Equal(t, StatePending, prev)
	require.Equal(t, StateUp, mon.State)
	require.NotZero(t, mon.LastSeenAt)
}

func TestManager_RecordHeartbeat_UpStaysUp(t *testing.T) {
	m := newTestManager(t)
	_, err := m.AddMonitor("u_alice", "scanner", 60, 30, "alerts", 3)
	require.Nil(t, err)
	_, _, err = m.RecordHeartbeat("u_alice", "scanner")
	require.Nil(t, err)

	_, prev, err := m.RecordHeartbeat("u_alice", "scanner")
	require.Nil(t, err)
	require.Equal(t, StateUp, prev) // already up
}

func TestManager_RecordHeartbeat_DownToUp(t *testing.T) {
	m := newTestManager(t)
	mon, err := m.AddMonitor("u_alice", "scanner", 60, 30, "alerts", 3)
	require.Nil(t, err)
	require.Nil(t, m.MarkDown(mon.ID))

	_, prev, err := m.RecordHeartbeat("u_alice", "scanner")
	require.Nil(t, err)
	require.Equal(t, StateDown, prev) // caller should fire UP recovery alert
}

func TestManager_RecordHeartbeat_NotFound(t *testing.T) {
	m := newTestManager(t)
	_, _, err := m.RecordHeartbeat("u_alice", "ghost")
	require.ErrorIs(t, err, ErrMonitorNotFound)
}

func TestManager_StaleMonitors_ExcludesPendingAndDown(t *testing.T) {
	m := newTestManager(t)
	// Pending monitor — never alerts even though "overdue"
	_, err := m.AddMonitor("u_alice", "pending-one", 60, 30, "a", 3)
	require.Nil(t, err)

	// Down monitor — already alerted, should not appear again
	down, err := m.AddMonitor("u_alice", "down-one", 60, 30, "a", 3)
	require.Nil(t, err)
	require.Nil(t, m.MarkDown(down.ID))

	// Up monitor with stale last_seen — should appear
	up, err := m.AddMonitor("u_alice", "up-stale", 60, 30, "a", 3)
	require.Nil(t, err)
	_, _, err = m.RecordHeartbeat("u_alice", "up-stale")
	require.Nil(t, err)
	// Backdate last_seen so now > last_seen + period + grace
	_, err = m.db.Exec(`UPDATE monitor SET last_seen_at = ? WHERE id = ?`, time.Now().Unix()-200, up.ID)
	require.Nil(t, err)

	// Up monitor with fresh last_seen — should NOT appear
	fresh, err := m.AddMonitor("u_alice", "up-fresh", 60, 30, "a", 3)
	require.Nil(t, err)
	_, _, err = m.RecordHeartbeat("u_alice", "up-fresh")
	require.Nil(t, err)
	_ = fresh

	stale, err := m.StaleMonitors()
	require.Nil(t, err)
	require.Len(t, stale, 1)
	require.Equal(t, "up-stale", stale[0].Key)
}

func TestManager_MarkDown(t *testing.T) {
	m := newTestManager(t)
	mon, err := m.AddMonitor("u_alice", "scanner", 60, 30, "a", 3)
	require.Nil(t, err)
	require.Nil(t, m.MarkDown(mon.ID))
	got, err := m.GetMonitor("u_alice", "scanner")
	require.Nil(t, err)
	require.Equal(t, StateDown, got.State)
}
