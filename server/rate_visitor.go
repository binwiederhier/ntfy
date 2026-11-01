package server

import (
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

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

// rateVisitor returns the visitor the topic's pushes are charged to, or nil if the topic has no
// live rate visitor. A rate visitor is live while the topic has a stream open here, or for
// rateVisitorLiveFor after the subscriber was last active on any node (the manager of a node
// with an open stream keeps it active). It is charged like the subscriber would be now: on its
// user key with its tier's limits if the user has a tier, else on its IP key with the anonymous
// limits. The user is read like an authenticated publish reads its own; a deleted user is
// charged on the IP, and a failed read is an error, so the publisher retries.
func (s *Server) rateVisitor(t *topic) (*visitor, error) {
	if !s.config.VisitorSubscriberRateLimiting || !isUnifiedPushTopic(t.ID) {
		return nil, nil
	}
	state := s.topicService.State(t.ID)
	if state == nil || state.RateVisitor == nil {
		return nil, nil
	} else if t.SubscribersCount() == 0 && time.Since(state.SubscribedAt) > rateVisitorLiveFor {
		return nil, nil
	}
	ip, err := netip.ParseAddr(state.RateVisitor.IP)
	if err != nil {
		return nil, nil
	}
	var u *user.User
	if state.RateVisitor.UserID != "" && s.userManager != nil {
		u, err = s.userManager.UserByID(state.RateVisitor.UserID)
		if errors.Is(err, user.ErrUserNotFound) {
			u = nil // Removed: the IP pays
		} else if err != nil {
			return nil, err
		} else if u.Deleted {
			u = nil // Marked deleted: the IP pays
		}
	}
	return newVisitor(s.visitors, ip, u), nil
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
	state := s.topicService.State(t.ID)
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

// topicAbandoned returns true if the topic's newest observation (a subscriber, or its first
// refused push) is older than unifiedPushAbandonedAfter
func topicAbandoned(state *tp.State) bool {
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
