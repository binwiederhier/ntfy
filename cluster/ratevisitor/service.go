// Package ratevisitor persists the topic -> rate-visitor assignment for UnifiedPush
// subscriber-based rate limiting across cluster nodes. In-memory, the assignment lives on the
// topic (topic.rateVisitor) and only works when publisher and subscriber hit the same node;
// this store lets a publish on node A resolve the subscriber that registered on node B.
package ratevisitor

import (
	"database/sql"
	"errors"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	schemaStoreKey = "topic_rate_visitor" // Store name in the shared schema_version table (see db/schema)
	schemaVersion  = 1
)

const (
	createTable = `
		CREATE TABLE IF NOT EXISTS topic_rate_visitor (
			topic TEXT PRIMARY KEY,
			visitor_key TEXT NOT NULL,
			user_id TEXT NOT NULL,
			expires BIGINT NOT NULL
		);
	`
	upsertQuery = `
		INSERT INTO topic_rate_visitor (topic, visitor_key, user_id, expires)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (topic) DO UPDATE SET
			visitor_key = EXCLUDED.visitor_key,
			user_id = EXCLUDED.user_id,
			expires = EXCLUDED.expires
	`
	selectQuery = `SELECT visitor_key, user_id FROM topic_rate_visitor WHERE topic = $1 AND expires >= $2`
	pruneQuery  = `DELETE FROM topic_rate_visitor WHERE expires < $1`
)

// ErrNotFound means no unexpired rate visitor is registered for the topic
var ErrNotFound = errors.New("no rate visitor for topic")

// Store persists topic -> rate-visitor assignments in the shared cluster database
type Store struct {
	pool *db.DB
}

// New creates the store and sets up its schema
func New(pool *db.DB) (*Store, error) {
	if err := schema.Migrate(pool.Primary(), schema.Postgres, schemaStoreKey, schemaVersion, schema.AsMigrateFunc(createTable), nil); err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// Set registers the visitor identified by visitorKey (and, for tier'd users, userID) as the
// topic's rate visitor until expires; an existing assignment is replaced
func (s *Store) Set(topic, visitorKey, userID string, expires time.Time) error {
	_, err := s.pool.Exec(upsertQuery, topic, visitorKey, userID, expires.Unix())
	return err
}

// Get returns the topic's unexpired rate-visitor assignment, or ErrNotFound
func (s *Store) Get(topic string) (visitorKey string, userID string, err error) {
	row := s.pool.QueryRow(selectQuery, topic, time.Now().Unix())
	if err := row.Scan(&visitorKey, &userID); errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	} else if err != nil {
		return "", "", err
	}
	return visitorKey, userID, nil
}

// Prune deletes expired assignments; only the cluster leader calls this
func (s *Store) Prune() error {
	_, err := s.pool.Exec(pruneQuery, time.Now().Unix())
	return err
}
