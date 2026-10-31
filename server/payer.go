package server

import (
	"errors"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/user"
)

const (
	// unifiedPushAbandonedAfter is how long a UnifiedPush topic may go without subscriber activity
	// (subscribe, poll, open stream) before a push to it answers 404 instead of 507: RFC 8030
	// section 7.3 signals an expired subscription with 404, and app servers like Mastodon then
	// delete it (#664). It is long enough for a phone that is offline for a few days.
	unifiedPushAbandonedAfter = 7 * oneDay

	// payerUserCacheTTL is how long a payer's user is cached (see payerUserCache)
	payerUserCacheTTL = time.Minute
)

// payerUserCache caches the users of the topics' payers by ID, so a push does not read the user
// database; a change to the user (a new tier) applies to pushes within payerUserCacheTTL
type payerUserCache struct {
	lookup  func(id string) (*user.User, error) // userManager.UserByID, nil without a user manager; replaced in tests
	entries map[string]*payerUserEntry
	mu      sync.Mutex // Protects entries
}

// payerUserEntry is a cached user; nil if it does not exist (anymore)
type payerUserEntry struct {
	user    *user.User
	fetched time.Time
}

func newPayerUserCache(lookup func(id string) (*user.User, error)) *payerUserCache {
	return &payerUserCache{
		lookup:  lookup,
		entries: make(map[string]*payerUserEntry),
	}
}

// payerVisitor returns the visitor of the topic's live payer, or nil if it has none. A payer is
// live while the topic has a stream open here, or for visitorExpungeAfter after the subscriber was
// last seen on any node (the manager of a node with an open stream touches it). A deleted
// user pays for nothing; a failed user lookup is returned, so the publisher retries.
func (s *Server) payerVisitor(t *topic) (*visitor, error) {
	if !s.config.VisitorSubscriberRateLimiting || !isUnifiedPushTopic(t.ID) {
		return nil, nil
	}
	p := s.limiter.Payer(t.ID)
	if p == nil || p.Key == "" {
		return nil, nil
	} else if t.SubscribersCount() == 0 && time.Since(p.SeenAt) > visitorExpungeAfter {
		return nil, nil
	}
	var u *user.User
	if p.UserID != "" {
		var err error
		if u, err = s.payerUsers.User(p.UserID); err != nil {
			return nil, err
		} else if u == nil {
			return nil, nil
		}
	}
	return newPayerVisitor(s.visitors, p.Key, u), nil
}

// unifiedPushTopicAbandoned returns true if the topic has had no subscriber activity, on any node,
// for unifiedPushAbandonedAfter, counted from its first refused push if it never had any
func (s *Server) unifiedPushTopicAbandoned(t *topic) bool {
	if !isUnifiedPushTopic(t.ID) || t.SubscribersCount() > 0 {
		return false
	}
	p := s.limiter.Payer(t.ID)
	return p != nil && time.Since(p.SeenAt) > unifiedPushAbandonedAfter
}

// touchPayers marks subscriber activity on the UnifiedPush topics now, keeping their payers
func (s *Server) touchPayers(topics []*topic) {
	if !s.config.VisitorSubscriberRateLimiting {
		return
	}
	for _, t := range topics {
		if isUnifiedPushTopic(t.ID) {
			s.limiter.TouchPayer(t.ID)
		}
	}
}

// User returns the user with the given ID, from the cache if it was looked up within
// payerUserCacheTTL, or nil if it does not exist or is deleted
func (c *payerUserCache) User(id string) (*user.User, error) {
	c.mu.Lock()
	e, ok := c.entries[id]
	c.mu.Unlock()
	if ok && time.Since(e.fetched) < payerUserCacheTTL {
		return e.user, nil
	}
	var u *user.User
	if c.lookup != nil {
		var err error
		u, err = c.lookup(id)
		if errors.Is(err, user.ErrUserNotFound) {
			u = nil
		} else if err != nil {
			return nil, err
		} else if u.Deleted {
			u = nil
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = &payerUserEntry{
		user:    u,
		fetched: time.Now(),
	}
	return u, nil
}

// Prune forgets the users looked up longer than payerUserCacheTTL ago
func (c *payerUserCache) Prune() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, e := range c.entries {
		if time.Since(e.fetched) >= payerUserCacheTTL {
			delete(c.entries, id)
		}
	}
}

// isUnifiedPushTopic returns true if the topic may have a payer: an "up..." topic of the
// UnifiedPush length
func isUnifiedPushTopic(id string) bool {
	return strings.HasPrefix(id, unifiedPushTopicPrefix) && len(id) == unifiedPushTopicLength
}
