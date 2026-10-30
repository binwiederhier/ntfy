package server

import (
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/user"
)

// topicPayers remembers who pays for the publishes to a UnifiedPush topic: its subscriber, not the
// publisher (see visitor-subscriber-rate-limiting). It keeps the subscriber's identity, not its visitor,
// so that every publish is billed with the subscriber's current user and tier (see Resolve).
//
// A payer is live while the topic has an open stream, any stream, and for visitorExpungeAfter after
// the payer subscribed or the last stream closed.
type topicPayers struct {
	config      *Config
	visitors    *visitorEnv
	userManager *user.Manager                       // May be nil
	userByID    func(id string) (*user.User, error) // userManager.UserByID, if there is a user manager; replaced in tests
	payers      map[string]payer                    // Topic ID -> payer
	streams     map[string]int                      // Topic ID -> open streams, for UnifiedPush topics only
	mu          sync.Mutex                          // Protects payers and streams
}

// payer is the identity of the subscriber that pays for a topic
type payer struct {
	ip     netip.Addr // The subscriber's IP address
	userID string     // The subscriber's user ID, or "" if anonymous
	seen   time.Time  // When the payer subscribed, or the topic's last stream closed; publishes do not count
}

func newTopicPayers(conf *Config, visitors *visitorEnv, userManager *user.Manager) *topicPayers {
	p := &topicPayers{
		config:      conf,
		visitors:    visitors,
		userManager: userManager,
		payers:      make(map[string]payer),
		streams:     make(map[string]int),
	}
	if userManager != nil {
		p.userByID = userManager.UserByID
	}
	return p
}

// Subscribe makes v the payer of the UnifiedPush topics it may pay for (see mayPay); the latest
// subscriber wins. It returns those topics.
func (p *topicPayers) Subscribe(v *visitor, topicIDs []string) ([]string, error) {
	payerTopicIDs := make([]string, 0)
	for _, topicID := range p.unifiedPushTopics(topicIDs) {
		ok, err := p.mayPay(v.User(), topicID)
		if err != nil {
			return nil, err
		} else if ok {
			payerTopicIDs = append(payerTopicIDs, topicID)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, topicID := range payerTopicIDs {
		p.payers[topicID] = payer{
			ip:     v.ip,
			userID: v.MaybeUserID(),
			seen:   time.Now(),
		}
	}
	return payerTopicIDs, nil
}

// Open counts a stream on the topics as open; it keeps their payers live until it is closed (see Close)
func (p *topicPayers) Open(topicIDs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, topicID := range p.unifiedPushTopics(topicIDs) {
		p.streams[topicID]++
	}
}

// Close counts a stream on the topics as closed, and marks their payers as seen
func (p *topicPayers) Close(topicIDs []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, topicID := range p.unifiedPushTopics(topicIDs) {
		if p.streams[topicID] <= 1 {
			delete(p.streams, topicID)
		} else {
			p.streams[topicID]--
		}
		if py, ok := p.payers[topicID]; ok {
			py.seen = time.Now()
			p.payers[topicID] = py
		}
	}
}

// Resolve returns a fresh visitor for the topic's payer, built with the subscriber's current user and
// tier, or nil if the topic has no live payer. A deleted user pays for nothing; its IP does not pay
// in its place. Other lookup errors are returned, so the publisher retries.
func (p *topicPayers) Resolve(topicID string) (*visitor, error) {
	p.mu.Lock()
	py, ok := p.payers[topicID]
	live := ok && p.liveNoLock(topicID, py)
	p.mu.Unlock()
	if !live {
		return nil, nil
	}
	u, exists, err := p.currentUser(py.userID)
	if err != nil {
		return nil, err
	} else if !exists {
		return nil, nil
	}
	return newVisitor(p.visitors, py.ip, u), nil
}

// Live returns true if the topic has a live payer; the manager keeps such topics in memory
func (p *topicPayers) Live(topicID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	py, ok := p.payers[topicID]
	return ok && p.liveNoLock(topicID, py)
}

// Prune forgets the payers that are not live, and returns how many
func (p *topicPayers) Prune() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	pruned := 0
	for topicID, py := range p.payers {
		if !p.liveNoLock(topicID, py) {
			delete(p.payers, topicID)
			pruned++
		}
	}
	return pruned
}

// unifiedPushTopics returns the topics that may have a payer: UnifiedPush topics, if the feature is on
func (p *topicPayers) unifiedPushTopics(topicIDs []string) []string {
	if !p.config.VisitorSubscriberRateLimiting {
		return nil
	}
	unifiedPushTopicIDs := make([]string, 0)
	for _, topicID := range topicIDs {
		if strings.HasPrefix(topicID, unifiedPushTopicPrefix) && len(topicID) == unifiedPushTopicLength {
			unifiedPushTopicIDs = append(unifiedPushTopicIDs, topicID)
		}
	}
	return unifiedPushTopicIDs
}

// liveNoLock returns true if the topic has an open stream, or its payer was seen recently
func (p *topicPayers) liveNoLock(topicID string, py payer) bool {
	return p.streams[topicID] > 0 || time.Since(py.seen) < visitorExpungeAfter
}

// mayPay returns true if the user (nil if anonymous) may pay for the topic:
// - access controls are off
// - or the topic is reserved, and the user is the owner
// - or the topic is not reserved, and the user has write access
func (p *topicPayers) mayPay(u *user.User, topicID string) (bool, error) {
	if p.userManager == nil {
		return true, nil
	}
	ownerUserID, err := p.userManager.ReservationOwner(topicID)
	if err != nil {
		return false, err
	}
	if ownerUserID == "" {
		return p.userManager.Authorize(u, topicID, user.PermissionWrite) == nil, nil
	}
	return u != nil && ownerUserID == u.ID, nil
}

// currentUser looks up the user with the given ID as it is now, or returns nil for "" (anonymous).
// It returns exists=false if the user was deleted; a deleted user pays for nothing.
func (p *topicPayers) currentUser(userID string) (u *user.User, exists bool, err error) {
	if userID == "" {
		return nil, true, nil
	}
	u, err = p.userByID(userID)
	if errors.Is(err, user.ErrUserNotFound) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	} else if u.Deleted {
		return nil, false, nil
	}
	return u, true, nil
}
