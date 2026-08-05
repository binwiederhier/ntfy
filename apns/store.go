package apns

import (
	"errors"
	"net/netip"
	"time"

	"heckel.io/ntfy/v2/db"
)

const (
	schemaStore = "apns"
)

// Errors returned by the store
var (
	ErrAPNSTokenCannotBeEmpty = errors.New("APNs token cannot be empty")
	ErrAPNSTopicCannotBeEmpty = errors.New("APNs topic cannot be empty")
)

// Store holds the database connection and queries for APNs subscriptions.
type Store struct {
	db      *db.DB
	queries queries
}

// queries holds the database-specific SQL queries.
type queries struct {
	upsertSubscription           string
	deleteSubscription           string
	deleteSubscriptionByToken    string
	selectSubscriptionsForTopic  string
}

// Register adds or updates an APNs subscription for the given token and topic.
func (s *Store) Register(token, topic, userID string, subscriberIP netip.Addr) error {
	if token == "" {
		return ErrAPNSTokenCannotBeEmpty
	}
	if topic == "" {
		return ErrAPNSTopicCannotBeEmpty
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(
		s.queries.upsertSubscription,
		token,
		topic,
		userID,
		subscriberIP.String(),
		now,
	)
	return err
}

// Unregister removes the APNs subscription for the given token and topic.
func (s *Store) Unregister(token, topic string) error {
	if token == "" {
		return ErrAPNSTokenCannotBeEmpty
	}
	if topic == "" {
		return ErrAPNSTopicCannotBeEmpty
	}
	_, err := s.db.Exec(s.queries.deleteSubscription, token, topic)
	return err
}

// GetTokens returns all registered APNs device tokens for the given topic.
func (s *Store) GetTokens(topic string) ([]string, error) {
	if topic == "" {
		return nil, ErrAPNSTopicCannotBeEmpty
	}
	rows, err := s.db.ReadOnly().Query(s.queries.selectSubscriptionsForTopic, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []string
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}
