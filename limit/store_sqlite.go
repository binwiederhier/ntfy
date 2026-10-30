package limit

import (
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	sqliteSelectClockQuery    = `SELECT CAST(strftime('%s', 'now') AS INTEGER)`
	sqliteSelectSinceQuery    = `SELECT key, period, kind, count FROM usage WHERE updated_at >= ?`
	sqliteSelectKeysInQuery   = `SELECT key, period, kind, count FROM usage WHERE key IN (` // Completed per chunk
	sqliteUpsertPrefixQuery   = `INSERT INTO usage (key, period, kind, count, updated_at) VALUES `
	sqliteUpsertRowQuery      = `(?, ?, ?, ?, CAST(strftime('%s', 'now') AS INTEGER))`
	sqliteUpsertConflictQuery = ` ON CONFLICT (key, period, kind) DO UPDATE SET count = count + excluded.count, updated_at = excluded.updated_at`
	sqlitePruneQuery          = `DELETE FROM usage WHERE updated_at < CAST(strftime('%s', 'now') AS INTEGER) - ?`

	sqliteUpsertTopicSubscriberQuery = `
		INSERT INTO topic (id, created_at, last_subscribed_at, last_subscriber_key) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			last_subscribed_at = excluded.last_subscribed_at,
			last_subscriber_key = excluded.last_subscriber_key
	`
	sqliteUpsertTopicPublishQuery = `
		INSERT INTO topic (id, created_at, last_published_at, last_publisher_key) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET last_published_at = excluded.last_published_at, last_publisher_key = excluded.last_publisher_key
	`
	sqliteUpsertTopicRateVisitorQuery = `
		INSERT INTO topic (id, created_at, rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET rate_visitor_key = excluded.rate_visitor_key, rate_visitor_uid = excluded.rate_visitor_uid, rate_visitor_seen_at = excluded.rate_visitor_seen_at
	`
	sqliteInsertTopicQuery = `INSERT INTO topic (id, created_at) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`
	sqliteSelectTopicQuery = `
		SELECT id, created_at, last_subscribed_at, last_subscriber_key, last_published_at, last_publisher_key, rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at
		FROM topic WHERE id = ?
	`
	sqlitePruneTopicsQuery = `DELETE FROM topic WHERE MAX(created_at, last_subscribed_at, last_published_at, rate_visitor_seen_at) < ?`
)

// NewSQLiteStore returns the store on the user database (auth-file), which holds the usage table
// (see package user, which creates and migrates it)
func NewSQLiteStore(d *db.DB) *Store {
	return &Store{
		db:      d,
		dialect: schema.SQLite,
		queries: queries{
			selectClock:    sqliteSelectClockQuery,
			selectSince:    sqliteSelectSinceQuery,
			selectKeysIn:   sqliteSelectKeysInQuery,
			upsertPrefix:   sqliteUpsertPrefixQuery,
			upsertRow:      sqliteUpsertRowQuery,
			upsertConflict: sqliteUpsertConflictQuery,
			prune:          sqlitePruneQuery,
			topics: topicQueries{
				upsertSubscriber:  sqliteUpsertTopicSubscriberQuery,
				upsertPublish:     sqliteUpsertTopicPublishQuery,
				upsertRateVisitor: sqliteUpsertTopicRateVisitorQuery,
				insert:            sqliteInsertTopicQuery,
				selectTopic:       sqliteSelectTopicQuery,
				prune:             sqlitePruneTopicsQuery,
			},
		},
	}
}
