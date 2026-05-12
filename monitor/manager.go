package monitor

import (
	"database/sql"
	"errors"
	"time"

	"heckel.io/ntfy/v2/db"
)

// Manager owns the monitor database connection and exposes CRUD + state operations.
type Manager struct {
	db *db.DB
}

// AddMonitor inserts a new monitor in StatePending. Returns ErrMonitorExists when
// (user_id, key) already exists.
func (m *Manager) AddMonitor(userID, key string, periodSeconds, graceSeconds int64, alertTopic string, alertPriority int) (*Monitor, error) {
	if userID == "" {
		return nil, ErrInvalidUserID
	}
	if err := ValidateKey(key); err != nil {
		return nil, err
	}
	if err := ValidatePeriod(periodSeconds); err != nil {
		return nil, err
	}
	if err := ValidateGrace(graceSeconds); err != nil {
		return nil, err
	}
	if err := ValidateAlertTopic(alertTopic); err != nil {
		return nil, err
	}
	if err := ValidateAlertPriority(alertPriority); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	res, err := m.db.Exec(sqliteInsertMonitorQuery, userID, key, periodSeconds, graceSeconds, alertTopic, alertPriority, StatePending, 0, now)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrMonitorExists
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Monitor{
		ID:            id,
		UserID:        userID,
		Key:           key,
		PeriodSeconds: periodSeconds,
		GraceSeconds:  graceSeconds,
		AlertTopic:    alertTopic,
		AlertPriority: alertPriority,
		State:         StatePending,
		LastSeenAt:    0,
		CreatedAt:     now,
	}, nil
}

// GetMonitor returns the monitor for (userID, key) or ErrMonitorNotFound.
func (m *Manager) GetMonitor(userID, key string) (*Monitor, error) {
	row := m.db.ReadOnly().QueryRow(sqliteSelectMonitorByUserAndKeyQuery, userID, key)
	mon, err := scanMonitor(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMonitorNotFound
	}
	return mon, err
}

// ListMonitorsByUser returns all monitors owned by userID, sorted by key.
func (m *Manager) ListMonitorsByUser(userID string) ([]*Monitor, error) {
	rows, err := m.db.ReadOnly().Query(sqliteSelectMonitorsByUserQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMonitors(rows)
}

// DeleteMonitor removes the monitor for (userID, key). Returns ErrMonitorNotFound
// when nothing was deleted.
func (m *Manager) DeleteMonitor(userID, key string) error {
	res, err := m.db.Exec(sqliteDeleteMonitorQuery, userID, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMonitorNotFound
	}
	return nil
}

// RecordHeartbeat marks a heartbeat for (userID, key) and returns the updated monitor along
// with the previous state. Pending or Down monitors transition to Up. Up monitors stay Up.
// The caller decides whether to publish an UP recovery alert based on the previous state.
func (m *Manager) RecordHeartbeat(userID, key string) (mon *Monitor, prevState string, err error) {
	err = db.ExecTx(m.db, func(tx *sql.Tx) error {
		row := tx.QueryRow(sqliteSelectMonitorByUserAndKeyQuery, userID, key)
		current, scanErr := scanMonitor(row)
		if errors.Is(scanErr, sql.ErrNoRows) {
			return ErrMonitorNotFound
		} else if scanErr != nil {
			return scanErr
		}
		prevState = current.State
		now := time.Now().Unix()
		if _, execErr := tx.Exec(sqliteUpdateMonitorHeartbeatQuery, now, StateUp, current.ID); execErr != nil {
			return execErr
		}
		current.LastSeenAt = now
		current.State = StateUp
		mon = current
		return nil
	})
	return mon, prevState, err
}

// StaleMonitors returns Up monitors whose last_seen_at + period + grace is in the past.
// Pending monitors are intentionally excluded; they never alert until the first ping arrives.
func (m *Manager) StaleMonitors() ([]*Monitor, error) {
	now := time.Now().Unix()
	rows, err := m.db.ReadOnly().Query(sqliteSelectStaleMonitorsQuery, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMonitors(rows)
}

// MarkDown transitions a monitor to StateDown. No-op if the monitor was already Down.
func (m *Manager) MarkDown(id int64) error {
	_, err := m.db.Exec(sqliteUpdateMonitorStateQuery, StateDown, id)
	return err
}

// SetLastSeenAt overrides last_seen_at for a monitor. Useful for tests and admin tooling.
// It does not change the monitor state.
func (m *Manager) SetLastSeenAt(id int64, unixSeconds int64) error {
	_, err := m.db.Exec(`UPDATE monitor SET last_seen_at = ? WHERE id = ?`, unixSeconds, id)
	return err
}

// Close closes the underlying database.
func (m *Manager) Close() error {
	return m.db.Close()
}

func scanMonitor(row interface{ Scan(...any) error }) (*Monitor, error) {
	var mon Monitor
	if err := row.Scan(&mon.ID, &mon.UserID, &mon.Key, &mon.PeriodSeconds, &mon.GraceSeconds, &mon.AlertTopic, &mon.AlertPriority, &mon.State, &mon.LastSeenAt, &mon.CreatedAt); err != nil {
		return nil, err
	}
	return &mon, nil
}

func scanMonitors(rows *sql.Rows) ([]*Monitor, error) {
	monitors := make([]*Monitor, 0)
	for rows.Next() {
		mon, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, mon)
	}
	return monitors, rows.Err()
}
