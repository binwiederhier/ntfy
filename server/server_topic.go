package server

import (
	"errors"
	"net/netip"
	"strings"
	"time"

	"heckel.io/ntfy/v2/limit"
	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/user"
)

// The topic table (see limit.TopicInfo) remembers what an in-memory topic used to: who a
// UnifiedPush topic bills to, and when it was last subscribed to. With it, topics are evicted
// from memory after minutes, a node that never saw a topic knows its rate visitor, and a push to
// a topic nobody subscribed to for days is told to go away. Without it (no usage file, no
// database), everything stays in memory as before.

const (
	tagTopic             = "topic"
	visitorKeyIPPrefix   = "ip:"   // See visitor.Key
	visitorKeyUserPrefix = "user:" // See visitor.Key
)

// topicExpungeAfter returns how long a topic without subscribers stays in memory: minutes when
// the topic table remembers it, 16 hours otherwise
func (s *Server) topicExpungeAfter() time.Duration {
	if s.limiter.Persistent() {
		return topicExpungeAfterPersisted
	}
	return topicExpungeAfter
}

// recordSubscribers writes to the topic table that the visitor is (still) subscribed to the
// topics, throttled per topic, in the background
func (s *Server) recordSubscribers(v *visitor, topics []*topic) {
	if !s.limiter.Persistent() {
		return
	}
	key := v.Key()
	for _, t := range topics {
		if t.ShouldRecord(false) {
			go s.logTopicError(t.ID, s.limiter.RecordTopicSubscriber(t.ID, key))
		}
	}
}

// recordPublish writes a successful publish to the topic table, throttled per topic, in the background
func (s *Server) recordPublish(v *visitor, t *topic) {
	if s.limiter.Persistent() && t.ShouldRecord(true) {
		key := v.Key()
		go s.logTopicError(t.ID, s.limiter.RecordTopicPublish(t.ID, key))
	}
}

// recordRateVisitor writes the topic's new rate visitor to the topic table, in the background
func (s *Server) recordRateVisitor(v *visitor, t *topic) {
	if s.limiter.Persistent() {
		key, userID := v.Key(), v.MaybeUserID()
		go s.logTopicError(t.ID, s.limiter.SetTopicRateVisitor(t.ID, key, userID))
	}
}

// noteTopic writes the topic's existence to the topic table if it has no row yet, in the
// background; a refused UnifiedPush push uses it, so the topic ages from its first push on
func (s *Server) noteTopic(t *topic) {
	if s.limiter.Persistent() && t.ShouldRecord(true) {
		go s.logTopicError(t.ID, s.limiter.NoteTopic(t.ID))
	}
}

// maybeLoadRateVisitor sets the topic's rate visitor from the topic table if it has none in
// memory (a fresh node, an evicted topic, or a subscriber on another node). It reads the table at
// most once per topicTableInfoTTL per topic; a failed read is logged and treated as no row (fail
// open: the push gets a 507, which publishers retry).
func (s *Server) maybeLoadRateVisitor(t *topic) {
	if !s.limiter.Persistent() || t.RateVisitor() != nil {
		return
	}
	info := s.topicTableInfo(t)
	if info == nil || !info.RateVisitorLive(time.Now()) {
		return
	}
	v, err := s.visitorFromKey(info.RateVisitorKey, info.RateVisitorUID)
	if err != nil {
		log.Tag(tagTopic).Err(err).Debug("Cannot restore rate visitor %s of topic %s", info.RateVisitorKey, t.ID)
		return
	}
	t.SetRateVisitor(v)
}

// topicIdleSince returns when the topic last showed a sign of a subscriber: from the topic table
// if there is one and it has a row, from the in-memory topic otherwise
func (s *Server) topicIdleSince(t *topic) time.Time {
	if info := s.topicTableInfo(t); info != nil {
		return info.LastSubscriberActivity()
	}
	return t.LastAccess()
}

// topicTableInfo returns the topic's table row, read at most once per topicTableInfoTTL; nil if
// there is no table, no row, or the read failed
func (s *Server) topicTableInfo(t *topic) *limit.TopicInfo {
	if !s.limiter.Persistent() {
		return nil
	}
	if info, ok := t.TableInfo(); ok {
		return info
	}
	info, err := s.limiter.Topic(t.ID)
	if err != nil && !errors.Is(err, limit.ErrTopicNotFound) {
		log.Tag(tagTopic).Err(err).Warn("Cannot read topic %s from the topic table", t.ID)
	}
	t.SetTableInfo(info) // Nil on a miss or an error: both are cached, so a flood of pushes costs one read
	return info
}

// visitorFromKey returns the visitor with the given limiter key (see visitor.Key)
func (s *Server) visitorFromKey(key, userID string) (*visitor, error) {
	if strings.HasPrefix(key, visitorKeyUserPrefix) {
		if s.userManager == nil || userID == "" {
			return nil, errors.New("user-keyed rate visitor without a user")
		}
		u, err := s.userManager.UserByID(userID)
		if err != nil {
			return nil, err
		}
		return s.visitor(netip.IPv4Unspecified(), u), nil
	}
	ip, err := netip.ParseAddr(strings.TrimPrefix(key, visitorKeyIPPrefix))
	if err != nil {
		return nil, err
	}
	return s.visitor(ip, (*user.User)(nil)), nil
}

func (s *Server) logTopicError(topicID string, err error) {
	if err != nil {
		log.Tag(tagTopic).Err(err).Warn("Cannot write topic %s to the topic table", topicID)
	}
}
