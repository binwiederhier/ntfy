package server

import (
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	tp "heckel.io/ntfy/v2/topic"
	"heckel.io/ntfy/v2/user"
	vis "heckel.io/ntfy/v2/visitor"
)

const (
	// unifiedPushAbandonedAfter is how long a UnifiedPush topic may go without subscriber activity
	// (subscribe, poll, open stream) before a push to it answers 404 instead of 507: RFC 8030
	// section 7.3 signals an expired subscription with 404, and app servers like Mastodon then
	// delete it (#664). It is long enough for a phone that is offline for a few days. A topic
	// that never had a subscriber counts from its first refused push.
	unifiedPushAbandonedAfter = 7 * oneDay

	// rateVisitorLiveFor is how long a topic's rate visitor pays for its pushes after the
	// subscriber was last active on any node (see Server.rateVisitor); a stream open here keeps
	// it live regardless
	rateVisitorLiveFor = oneDay
)

// rateVisitorUserCache holds the users of the live rate visitors by ID, so a push never reads the
// user database: the server refreshes them in the background (see Server.refreshRateVisitorUsers),
// and a subscribe puts the subscriber's user in right away
type rateVisitorUserCache struct {
	lookup func(ids []string) ([]*user.User, error) // userManager.UsersByIDs, nil without a user manager; replaced in tests
	users  map[string]*user.User                    // By ID; a deleted user is not in here
	mu     sync.Mutex                               // Protects users
}

func newRateVisitorUserCache(lookup func(ids []string) ([]*user.User, error)) *rateVisitorUserCache {
	return &rateVisitorUserCache{
		lookup: lookup,
		users:  make(map[string]*user.User),
	}
}

// rateVisitor returns the visitor the topic's pushes are charged to, or nil if the topic has no
// live rate visitor. A rate visitor is live while the topic has a stream open here, or for
// rateVisitorLiveFor after the subscriber was last active on any node (the manager of a node
// with an open stream keeps it active). It is charged like the subscriber would be now: on its
// user key with its tier's limits if the user has a tier (as of the last refresh), else on its
// IP key with the anonymous limits. A deleted user, or one not looked up yet, is charged on the IP.
func (s *Server) rateVisitor(t *topic) *visitor {
	if !s.config.VisitorSubscriberRateLimiting || !isUnifiedPushTopic(t.ID) {
		return nil
	}
	state := s.topicService.Get(t.ID)
	if state == nil || state.RateVisitor == nil {
		return nil
	} else if t.SubscribersCount() == 0 && time.Since(state.SubscribedAt) > rateVisitorLiveFor {
		return nil
	}
	ip, err := netip.ParseAddr(state.RateVisitor.IP)
	if err != nil {
		return nil
	}
	var u *user.User
	if state.RateVisitor.UserID != "" {
		u = s.rateVisitorUsers.User(state.RateVisitor.UserID)
	}
	return newVisitor(s.visitors, ip, u)
}

// unifiedPushTopicAbandoned returns true if the topic has had no subscriber activity, on any node,
// for unifiedPushAbandonedAfter, counted from its first refused push if it never had any. Before
// saying so, it reads the topic's row from the database, since a subscriber may have come back
// on another node and the row be fresher than what this node has read back; if that read fails,
// the topic is not abandoned (the publisher retries).
func (s *Server) unifiedPushTopicAbandoned(r *http.Request, t *topic) bool {
	if !isUnifiedPushTopic(t.ID) || t.SubscribersCount() > 0 {
		return false
	}
	state := s.topicService.Get(t.ID)
	if state == nil || !topicAbandoned(state) {
		return false
	}
	state, err := s.topicService.Refresh(r.Context(), t.ID)
	if err != nil {
		logr(r).Tag(tagPublish).With(t).Err(err).Warn("Cannot read topic %s to confirm it is abandoned; answering as if it were not", t.ID)
		return false
	}
	return state != nil && topicAbandoned(state)
}

// touchTopics records subscriber activity on the UnifiedPush topics now, e.g. a stream that
// closed, keeping their rate visitors
func (s *Server) touchTopics(topics []*topic) {
	if !s.config.VisitorSubscriberRateLimiting {
		return
	}
	for _, t := range topics {
		if isUnifiedPushTopic(t.ID) {
			s.topicService.Touch(t.ID)
		}
	}
}

// keepaliveTopics records that the UnifiedPush topics have a stream open here, so the other nodes
// see their subscribers as active too
func (s *Server) keepaliveTopics(topics []*topic) {
	if !s.config.VisitorSubscriberRateLimiting {
		return
	}
	for _, t := range topics {
		if isUnifiedPushTopic(t.ID) {
			s.topicService.Keepalive(t.ID)
		}
	}
}

// refreshRateVisitorUsers looks up the users of the live rate visitors again, so a tier change (or
// a deleted account) applies to the pushes within a manager interval
func (s *Server) refreshRateVisitorUsers() {
	if !s.config.VisitorSubscriberRateLimiting || s.userManager == nil {
		return
	}
	ids := s.topicService.RateVisitorUserIDs(time.Now().Add(-rateVisitorLiveFor))
	if err := s.rateVisitorUsers.Refresh(ids); err != nil {
		log.Tag(tagManager).Err(err).Warn("Cannot refresh the users of %d rate visitor(s); keeping the cached ones", len(ids))
	}
}

// User returns the cached user with the given ID, or nil if it is not cached (not a live rate
// visitor's, not looked up yet, or deleted)
func (c *rateVisitorUserCache) User(id string) *user.User {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.users[id]
}

// Put caches the user, e.g. the subscriber's on a subscribe, so its pushes are charged with its
// tier before the next refresh
func (c *rateVisitorUserCache) Put(u *user.User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users[u.ID] = u
}

// Refresh replaces the cached users with fresh lookups of the given IDs, dropping the others. On
// an error the cache is left as it is: stale users are better than none.
func (c *rateVisitorUserCache) Refresh(ids []string) error {
	users := make(map[string]*user.User, len(ids))
	if len(ids) > 0 && c.lookup != nil {
		looked, err := c.lookup(ids)
		if err != nil {
			return err
		}
		for _, u := range looked {
			if !u.Deleted {
				users[u.ID] = u
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users = users
	return nil
}

// topicAbandoned returns true if the topic's newest observation (a subscriber, or its first
// refused push) is older than unifiedPushAbandonedAfter
func topicAbandoned(state *tp.Topic) bool {
	newest := state.FirstSeenAt
	if state.SubscribedAt.After(newest) {
		newest = state.SubscribedAt
	}
	return time.Since(newest) > unifiedPushAbandonedAfter
}

// isUnifiedPushTopic returns true if the topic may have a rate visitor: an "up..." topic of the
// UnifiedPush length
func isUnifiedPushTopic(id string) bool {
	return strings.HasPrefix(id, unifiedPushTopicPrefix) && len(id) == unifiedPushTopicLength
}

// maskedIP returns the visitor's IP masked to the configured prefix bits, as a string, which is
// what is kept of a subscriber (see topic.RateVisitor)
func (v *visitor) maskedIP() string {
	return vis.Mask(v.ip, v.env.limitsConfig).String()
}
