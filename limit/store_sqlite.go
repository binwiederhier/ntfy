package limit

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	sqliteSelectClockQuery    = `SELECT CAST(strftime('%s', 'now') AS INTEGER)`
	sqliteSelectSinceQuery    = `SELECT key, kind, count FROM usage WHERE day = ? AND updated_at >= ?`
	sqliteSelectKeysInQuery   = `SELECT key, kind, count FROM usage WHERE day = ? AND key IN (` // Completed per chunk
	sqliteUpsertPrefixQuery   = `INSERT INTO usage (key, day, kind, count, updated_at) VALUES `
	sqliteUpsertRowQuery      = `(?, ?, ?, ?, CAST(strftime('%s', 'now') AS INTEGER))`
	sqliteUpsertConflictQuery = ` ON CONFLICT (key, day, kind) DO UPDATE SET count = count + excluded.count, updated_at = excluded.updated_at`
	sqlitePruneQuery          = `DELETE FROM usage WHERE day < ?`

	sqliteUpsertTopicSubscriberQuery = `
		INSERT INTO topic (id, created_at, last_subscribed_at, last_subscriber_key) VALUES (?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			last_subscribed_at = excluded.last_subscribed_at,
			last_subscriber_key = excluded.last_subscriber_key,
			rate_visitor_seen_at = CASE WHEN topic.rate_visitor_key = excluded.last_subscriber_key THEN excluded.last_subscribed_at ELSE topic.rate_visitor_seen_at END
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

// The topic table: who a topic bills to (the UnifiedPush rate visitor), when it was created,
// last subscribed to and last published on (see topics.go)
const (
	sqliteCreateTopicTableQuery = `
		CREATE TABLE IF NOT EXISTS topic (
			id TEXT PRIMARY KEY,
			created_at INT NOT NULL,
			last_subscribed_at INT NOT NULL DEFAULT 0,
			last_subscriber_key TEXT NOT NULL DEFAULT '',
			last_published_at INT NOT NULL DEFAULT 0,
			last_publisher_key TEXT NOT NULL DEFAULT '',
			rate_visitor_key TEXT NOT NULL DEFAULT '',
			rate_visitor_uid TEXT NOT NULL DEFAULT '',
			rate_visitor_seen_at INT NOT NULL DEFAULT 0
		);
	`
)

// Schema
var (
	sqliteCreateTables = schema.AsMigrateFunc(`
		CREATE TABLE IF NOT EXISTS usage (
			key TEXT NOT NULL,
			day TEXT NOT NULL,
			kind TEXT NOT NULL,
			count INT NOT NULL DEFAULT 0,
			updated_at INT NOT NULL,
			PRIMARY KEY (key, day, kind)
		);
		CREATE INDEX IF NOT EXISTS idx_usage_day_updated_at ON usage (day, updated_at);
	` + sqliteCreateTopicTableQuery)
	sqliteMigrations = schema.Migrations{
		{From: 1, To: 2}: schema.Query(sqliteCreateTopicTableQuery), // The topic table (who a topic bills to, when it was last subscribed to)
	}
)

// NewSQLiteStore opens (or creates) the usage database in the given file
func NewSQLiteStore(filename string) (*Store, error) {
	d, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	if err := schema.Migrate(d, schema.SQLite, schemaStore, sqliteCreateTables, sqliteMigrations); err != nil {
		return nil, err
	}
	return &Store{
		db:      db.New(&db.Host{DB: d}, nil),
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
		closer: d,
	}, nil
}
