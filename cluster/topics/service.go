// Package topics persists per-topic state in the shared cluster database, so decisions that
// today depend on one node's in-memory topic object (UnifiedPush rate-visitor billing, the
// Matrix pushkey rejection heuristic, topic liveness) can be made consistently on any node and
// survive restarts.
//
// Nothing uses this yet: it is one piece of multi-node ntfy, landed on its own so it can be
// reviewed without the server changes that will read from it.
//
// Write cadence: rate-visitor assignment writes happen on subscribe (rare); subscriber and
// publish liveness updates are meant to be throttled by the caller (one write per topic per
// interval), so the table stays off the hot path.
package topics

import (
	"database/sql"
	"errors"
	"slices"
	"time"

	"heckel.io/ntfy/v2/db"
	"heckel.io/ntfy/v2/db/schema"
)

const (
	schemaStoreKey = "topic" // Store name in the shared schema_version table (see db/schema)
	schemaVersion  = 1

	// RateVisitorTTL is how long after the rate visitor's last observed activity the
	// assignment remains valid. Subscriber keepalives refresh it (throttled), so a connected
	// subscriber keeps the assignment alive indefinitely; only truly departed subscribers
	// expire, matching the in-memory visitor staleness rule.
	RateVisitorTTL = 24 * time.Hour

	// lastActivityChunkSize caps the number of topic IDs per LastActivity query
	lastActivityChunkSize = 1000

	// retention is how long rows without any recorded activity are kept before the leader
	// prunes them. Far longer than any decision window that reads the table.
	retention = 30 * 24 * time.Hour
)

const (
	createTable = `
		CREATE TABLE IF NOT EXISTS topic (
			id TEXT PRIMARY KEY,
			created_at BIGINT NOT NULL,
			last_subscribed_at BIGINT NOT NULL DEFAULT 0,
			last_subscriber_key TEXT NOT NULL DEFAULT '',
			last_published_at BIGINT NOT NULL DEFAULT 0,
			last_publisher_key TEXT NOT NULL DEFAULT '',
			rate_visitor_key TEXT NOT NULL DEFAULT '',
			rate_visitor_uid TEXT NOT NULL DEFAULT '',
			rate_visitor_seen_at BIGINT NOT NULL DEFAULT 0
		);
	`
	upsertSubscriberQuery = `
		INSERT INTO topic (id, created_at, last_subscribed_at, last_subscriber_key)
		VALUES ($1, $2, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
			last_subscribed_at = EXCLUDED.last_subscribed_at,
			last_subscriber_key = EXCLUDED.last_subscriber_key,
			rate_visitor_seen_at = CASE WHEN topic.rate_visitor_key = EXCLUDED.last_subscriber_key
				THEN EXCLUDED.last_subscribed_at ELSE topic.rate_visitor_seen_at END
	`
	upsertPublishQuery = `
		INSERT INTO topic (id, created_at, last_published_at, last_publisher_key)
		VALUES ($1, $2, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
			last_published_at = EXCLUDED.last_published_at,
			last_publisher_key = EXCLUDED.last_publisher_key
	`
	upsertRateVisitorQuery = `
		INSERT INTO topic (id, created_at, rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at)
		VALUES ($1, $2, $3, $4, $2)
		ON CONFLICT (id) DO UPDATE SET
			rate_visitor_key = EXCLUDED.rate_visitor_key,
			rate_visitor_uid = EXCLUDED.rate_visitor_uid,
			rate_visitor_seen_at = EXCLUDED.rate_visitor_seen_at
	`
	selectTopicQuery = `
		SELECT id, created_at, last_subscribed_at, last_subscriber_key, last_published_at, last_publisher_key,
		       rate_visitor_key, rate_visitor_uid, rate_visitor_seen_at
		FROM topic WHERE id = $1
	`
	selectLastActivityQuery = `
		SELECT id, GREATEST(created_at, last_subscribed_at, last_published_at, rate_visitor_seen_at)
		FROM topic WHERE id = ANY($1)
	`
	pruneQuery = `DELETE FROM topic WHERE GREATEST(created_at, last_subscribed_at, last_published_at, rate_visitor_seen_at) < $1`
)

// ErrNotFound means the topic has no row (it has never been recorded on any node)
var ErrNotFound = errors.New("topic not recorded")

// Info is a topic's durable, cluster-shared state
type Info struct {
	ID                string
	CreatedAt         time.Time
	LastSubscribedAt  time.Time
	LastSubscriberKey string
	LastPublishedAt   time.Time
	LastPublisherKey  string
	RateVisitorKey    string // Empty if no rate visitor was ever assigned
	RateVisitorUID    string // User ID for user-keyed rate visitors, empty otherwise
	RateVisitorSeenAt time.Time
}

// Store persists per-topic state in the shared cluster database
type Store struct {
	pool *db.DB
	now  func() time.Time // Injectable clock, for tests
}

// New creates the store and sets up its schema
func New(pool *db.DB) (*Store, error) {
	if err := schema.Migrate(pool.Primary(), schema.Postgres, schemaStoreKey, schemaVersion, schema.AsMigrateFunc(createTable), nil); err != nil {
		return nil, err
	}
	return &Store{pool: pool, now: time.Now}, nil
}

// RecordSubscriber records that the given visitor is (still) subscribed to the topic. If the
// visitor is the topic's rate visitor, its liveness is refreshed too, keeping the assignment
// valid for as long as the subscriber sticks around. The server throttles calls per topic.
func (s *Store) RecordSubscriber(topic, visitorKey string) error {
	_, err := s.pool.Exec(upsertSubscriberQuery, topic, s.now().Unix(), visitorKey)
	return err
}

// RecordPublish records a successful publish to the topic and who sent it. The server
// throttles calls per topic.
func (s *Store) RecordPublish(topic, visitorKey string) error {
	_, err := s.pool.Exec(upsertPublishQuery, topic, s.now().Unix(), visitorKey)
	return err
}

// SetRateVisitor assigns the visitor the topic bills to (e.g. the UnifiedPush subscriber);
// an existing assignment is replaced
func (s *Store) SetRateVisitor(topic, visitorKey, userID string) error {
	_, err := s.pool.Exec(upsertRateVisitorQuery, topic, s.now().Unix(), visitorKey, userID)
	return err
}

// RateVisitor returns the topic's rate-visitor assignment if it is still live (the visitor was
// seen within RateVisitorTTL), or ErrNotFound
func (s *Store) RateVisitor(topic string) (visitorKey string, userID string, err error) {
	info, err := s.Get(topic)
	if err != nil {
		return "", "", err
	}
	if info.RateVisitorKey == "" || s.now().Sub(info.RateVisitorSeenAt) > RateVisitorTTL {
		return "", "", ErrNotFound
	}
	return info.RateVisitorKey, info.RateVisitorUID, nil
}

// Get returns everything recorded about the topic, or ErrNotFound
func (s *Store) Get(topic string) (*Info, error) {
	row := s.pool.QueryRow(selectTopicQuery, topic)
	var createdAt, lastSubscribedAt, lastPublishedAt, rateVisitorSeenAt int64
	info := &Info{}
	err := row.Scan(&info.ID, &createdAt, &lastSubscribedAt, &info.LastSubscriberKey, &lastPublishedAt, &info.LastPublisherKey,
		&info.RateVisitorKey, &info.RateVisitorUID, &rateVisitorSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	info.CreatedAt = time.Unix(createdAt, 0)
	info.LastSubscribedAt = time.Unix(lastSubscribedAt, 0)
	info.LastPublishedAt = time.Unix(lastPublishedAt, 0)
	info.RateVisitorSeenAt = time.Unix(rateVisitorSeenAt, 0)
	return info, nil
}

// Prune deletes rows without any activity within the retention; only the cluster leader
// calls this (like the other singleton database jobs)
func (s *Store) Prune() error {
	_, err := s.pool.Exec(pruneQuery, s.now().Add(-retention).Unix())
	return err
}

// LastActivity returns the most recent recorded activity for each of the given topics. Topics
// without a row are absent from the result.
func (s *Store) LastActivity(ids []string) (map[string]time.Time, error) {
	activity := make(map[string]time.Time)
	for chunk := range slices.Chunk(ids, lastActivityChunkSize) {
		if err := s.lastActivityChunk(chunk, activity); err != nil {
			return nil, err
		}
	}
	return activity, nil
}

func (s *Store) lastActivityChunk(ids []string, activity map[string]time.Time) error {
	rows, err := s.pool.Query(selectLastActivityQuery, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var lastActivity int64
		if err := rows.Scan(&id, &lastActivity); err != nil {
			return err
		}
		activity[id] = time.Unix(lastActivity, 0)
	}
	return rows.Err()
}
