package limit

import (
	"context"
	"database/sql"

	"heckel.io/ntfy/v2/db"
)

// Same as the Postgres queries (see there), one row per statement: SQLite is a local file, so
// a prepared statement per row in one transaction costs no round trips
const (
	sqliteSelectSinceQuery = `SELECT key, period_start, messages, emails, calls FROM usage WHERE updated_at >= ?`
	sqliteUpsertQuery      = `
		INSERT INTO usage (key, period_start, messages, emails, calls, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET
			messages = CASE WHEN period_start >= excluded.period_start THEN messages + excluded.messages ELSE excluded.messages END,
			emails = CASE WHEN period_start >= excluded.period_start THEN emails + excluded.emails ELSE excluded.emails END,
			calls = CASE WHEN period_start >= excluded.period_start THEN calls + excluded.calls ELSE excluded.calls END,
			period_start = MAX(period_start, excluded.period_start),
			updated_at = excluded.updated_at
	`
	sqlitePruneQuery = `DELETE FROM usage WHERE updated_at < ?1 AND key IN (SELECT key FROM usage WHERE updated_at < ?1 LIMIT ?2)`
)

// Same as the Postgres payer queries (see there), one row per statement
const (
	sqliteSelectPayersSinceQuery = `SELECT topic, payer_key, payer_user_id, seen_at FROM topic_payer WHERE updated_at >= ?`
	sqliteUpsertPayerQuery       = `
		INSERT INTO topic_payer (topic, payer_key, payer_user_id, seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (topic) DO UPDATE SET
			payer_key = CASE WHEN excluded.payer_key = '' THEN payer_key ELSE excluded.payer_key END,
			payer_user_id = CASE WHEN excluded.payer_key = '' THEN payer_user_id ELSE excluded.payer_user_id END,
			seen_at = CASE WHEN excluded.payer_key = '' THEN MAX(seen_at, excluded.seen_at) ELSE excluded.seen_at END,
			updated_at = excluded.updated_at
	`
	sqlitePrunePayersQuery = `DELETE FROM topic_payer WHERE updated_at < ?1 AND topic IN (SELECT topic FROM topic_payer WHERE updated_at < ?1 LIMIT ?2)`
)

// NewSQLiteStore returns the store on the user database (auth-file), which holds the usage
// table (see package user, which creates and migrates it)
func NewSQLiteStore(d *db.DB) *Store {
	s := &Store{
		db: d,
		queries: &queries{
			selectSince:       sqliteSelectSinceQuery,
			upsert:            sqliteUpsertQuery,
			prune:             sqlitePruneQuery,
			selectPayersSince: sqliteSelectPayersSinceQuery,
			upsertPayer:       sqliteUpsertPayerQuery,
			prunePayers:       sqlitePrunePayersQuery,
		},
	}
	s.writeChunk = s.writeSQLite
	s.writePayerChunk = s.writePayersSQLite
	return s
}

// writeSQLite writes the rows in one transaction, one prepared statement per row
func (s *Store) writeSQLite(ctx context.Context, rows []usageRow, now int64) error {
	return db.ExecTx(s.db, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, s.queries.upsert)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx, r.key, r.periodStart, r.counts[0], r.counts[1], r.counts[2], now); err != nil {
				return err
			}
		}
		return nil
	})
}

// writePayersSQLite writes the payer rows in one transaction, one prepared statement per row
func (s *Store) writePayersSQLite(ctx context.Context, rows []payerRow, now int64) error {
	return db.ExecTx(s.db, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, s.queries.upsertPayer)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx, r.topic, r.key, r.userID, r.seenAt, now); err != nil {
				return err
			}
		}
		return nil
	})
}
