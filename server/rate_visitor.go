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
	// before a push to it is told to go away (410), so app servers like Mastodon delete the
	// subscription (#664); below the row's prune (7 days), which ends the 410s
	unifiedPushAbandonedAfter = 4 * oneDay

	// rateVisitorLiveFor is how long a rate visitor pays after its subscriber was last active on
	// any node; a stream open here keeps it live regardless
	rateVisitorLiveFor = oneDay
)

// rateVisitor returns the visitor the topic's pushes are charged to, or nil if it has no live
// rate visitor. The user is read on every push, like an authenticated publish reads its own: a
// tier change applies right away, a deleted user's IP pays, and a failed read is an error.
func (s *Server) rateVisitor(t *topic) (*visitor, error) {
	if !s.config.VisitorSubscriberRateLimiting || !isUnifiedPushTopic(t.ID) {
		return nil, nil
	}
	state := s.topicService.State(t.ID)
	if state == nil || state.RateVisitorIP == "" {
		return nil, nil
	} else if t.SubscribersCount() == 0 && time.Since(time.Unix(state.SubscribedAt, 0)) > rateVisitorLiveFor {
		return nil, nil
	}
	ip, err := netip.ParseAddr(state.RateVisitorIP)
	if err != nil {
		return nil, nil
	}
	var u *user.User
	if state.RateVisitorUserID != "" && s.userManager != nil {
		u, err = s.userManager.UserByID(state.RateVisitorUserID)
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

// unifiedPushTopicAbandoned records a refused push to a UnifiedPush topic (see topic.Service.Note)
// and returns true if the topic had no subscriber activity, on any node, for
// unifiedPushAbandonedAfter. Before a yes, it reads the row, which may be fresher than memory; a
// failed read is a no, so the publisher retries.
func (s *Server) unifiedPushTopicAbandoned(r *http.Request, t *topic) bool {
	if !isUnifiedPushTopic(t.ID) {
		return false
	}
	state := s.topicService.Note(t.ID)
	if state == nil || t.SubscribersCount() > 0 || !topicAbandoned(state) {
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

// topicAbandoned returns true if the topic's newest observation (a subscriber, or its first
// refused push) is older than unifiedPushAbandonedAfter
func topicAbandoned(state *tp.State) bool {
	return time.Since(time.Unix(max(state.FirstSeenAt, state.SubscribedAt), 0)) > unifiedPushAbandonedAfter
}

// isUnifiedPushTopic returns true if the topic may have a rate visitor: an "up..." topic of the
// UnifiedPush length
func isUnifiedPushTopic(id string) bool {
	return strings.HasPrefix(id, unifiedPushTopicPrefix) && len(id) == unifiedPushTopicLength
}

// maskedIP returns the visitor's IP masked to the configured prefix bits, which is what a rate
// visitor keeps of a subscriber
func (v *visitor) maskedIP() string {
	return vis.Mask(v.ip, v.env.limitsConfig).String()
}
