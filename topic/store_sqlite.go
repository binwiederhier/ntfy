package topic

import (
	"context"
	"database/sql"

	"heckel.io/ntfy/v2/db"
)

// Same as the Postgres queries, one row per statement: SQLite is a local file, so a prepared
// statement per row in one transaction costs no round trips
const (
	sqliteSelectSinceQuery = `SELECT topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at FROM topic WHERE updated_at >= ?`
	sqliteSelectOneQuery   = `SELECT topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at FROM topic WHERE topic = ?`
	sqliteUpsertQuery      = `
		INSERT INTO topic (topic, first_seen_at, subscribed_at, rate_visitor_user_id, rate_visitor_ip, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (topic) DO UPDATE SET
			first_seen_at = MIN(first_seen_at, excluded.first_seen_at),
			subscribed_at = MAX(subscribed_at, excluded.subscribed_at),
			rate_visitor_user_id = CASE WHEN excluded.rate_visitor_ip = '' THEN rate_visitor_user_id ELSE excluded.rate_visitor_user_id END,
			rate_visitor_ip = CASE WHEN excluded.rate_visitor_ip = '' THEN rate_visitor_ip ELSE excluded.rate_visitor_ip END,
			updated_at = excluded.updated_at
	`
	sqlitePruneQuery = `DELETE FROM topic WHERE updated_at < ?1 AND topic IN (SELECT topic FROM topic WHERE updated_at < ?1 LIMIT ?2)`
)

// NewSQLiteStore returns the store on the user database (auth-file); package user creates and migrates the table
func NewSQLiteStore(d *db.DB) *Store {
	s := &Store{
		db: d,
		queries: &queries{
			selectSince: sqliteSelectSinceQuery,
			selectOne:   sqliteSelectOneQuery,
			prune:       sqlitePruneQuery,
		},
	}
	s.writeChunk = s.writeSQLite
	return s
}

// writeSQLite writes the rows in one transaction, one prepared statement per row
func (s *Store) writeSQLite(ctx context.Context, rows []*State, now int64) error {
	return db.ExecTx(s.db, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, sqliteUpsertQuery)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range rows {
			if _, err := stmt.ExecContext(ctx, r.Topic, r.FirstSeenAt, r.SubscribedAt, r.RateVisitorUserID, r.RateVisitorIP, now); err != nil {
				return err
			}
		}
		return nil
	})
}
