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
type topicPayers struct {
	config      *Config
	visitors    *visitorEnv
	userManager *user.Manager                       // May be nil
	userByID    func(id string) (*user.User, error) // userManager.UserByID, if there is a user manager; replaced in tests
	payers      map[string]payer                    // Topic ID -> payer
	mu          sync.Mutex                          // Protects payers
}

// payer is the identity of the subscriber that pays for a topic
type payer struct {
	key    string     // The subscriber's visitor key when it subscribed; matches keepalives only, billing uses the current key
	ip     netip.Addr // The subscriber's IP address
	userID string     // The subscriber's user ID, or "" if anonymous
	seen   time.Time  // When a subscriber with this key was last seen on the topic; publishes do not count
}

func newTopicPayers(conf *Config, visitors *visitorEnv, userManager *user.Manager) *topicPayers {
	p := &topicPayers{
		config:      conf,
		visitors:    visitors,
		userManager: userManager,
		payers:      make(map[string]payer),
	}
	if userManager != nil {
		p.userByID = userManager.UserByID
	}
	return p
}

// Subscribe makes v the payer of the UnifiedPush topics it may pay for (see mayPay); the latest
// subscriber wins. It returns those topics, for the subscription's keepalives (see Keep).
func (p *topicPayers) Subscribe(v *visitor, topicIDs []string) ([]string, error) {
	if !p.config.VisitorSubscriberRateLimiting {
		return nil, nil
	}
	payerTopicIDs := make([]string, 0)
	for _, topicID := range topicIDs {
		if !strings.HasPrefix(topicID, unifiedPushTopicPrefix) || len(topicID) != unifiedPushTopicLength {
			continue
		}
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
			key:    v.key,
			ip:     v.ip,
			userID: v.MaybeUserID(),
			seen:   time.Now(),
		}
	}
	return payerTopicIDs, nil
}

// Keep is called on the keepalives of a subscription, with the topics Subscribe returned for it. A
// subscriber with the payer's key and user keeps it alive, whichever of its subscriptions set it. A
// stale topic is taken back only if v's current user may still pay for it; its write access or role
// may have changed since it subscribed.
func (p *topicPayers) Keep(v *visitor, topicIDs []string) error {
	errs := make([]error, 0)
	for _, topicID := range topicIDs {
		if !p.keepOrStale(v, topicID) {
			continue
		}
		u, exists, err := p.currentUser(v.MaybeUserID())
		if err != nil {
			errs = append(errs, err)
			continue
		} else if !exists {
			continue
		}
		ok, err := p.mayPay(u, topicID)
		if err != nil {
			errs = append(errs, err)
			continue
		} else if ok {
			p.takeIfStale(v, topicID)
		}
	}
	return errors.Join(errs...)
}

// Resolve returns a fresh visitor for the topic's payer, built with the subscriber's current user and
// tier, or nil if the topic has no live payer. A deleted user pays for nothing; its IP does not pay
// in its place. Other lookup errors are returned, so the publisher retries.
func (p *topicPayers) Resolve(topicID string) (*visitor, error) {
	p.mu.Lock()
	py, ok := p.payers[topicID]
	p.mu.Unlock()
	if !ok || py.stale() {
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

// Live returns true if the topic has a payer that is not stale; the manager keeps such topics in memory
func (p *topicPayers) Live(topicID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	py, ok := p.payers[topicID]
	return ok && !py.stale()
}

// Prune forgets the payers that are stale, and returns how many
func (p *topicPayers) Prune() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	pruned := 0
	for topicID, py := range p.payers {
		if py.stale() {
			delete(p.payers, topicID)
			pruned++
		}
	}
	return pruned
}

// keepOrStale marks the topic's payer as seen if it has v's key and user; users without a tier behind
// one IP share a key. Otherwise, it returns true if the topic has no live payer, so v may take it (see
// takeIfStale).
func (p *topicPayers) keepOrStale(v *visitor, topicID string) (stale bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	py, ok := p.payers[topicID]
	if ok && py.key == v.key && py.userID == v.MaybeUserID() {
		py.seen = time.Now()
		p.payers[topicID] = py
		return false
	}
	return !ok || py.stale()
}

// takeIfStale makes v the topic's payer, if the topic still has no live payer
func (p *topicPayers) takeIfStale(v *visitor, topicID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if py, ok := p.payers[topicID]; ok && !py.stale() {
		return
	}
	p.payers[topicID] = payer{
		key:    v.key,
		ip:     v.ip,
		userID: v.MaybeUserID(),
		seen:   time.Now(),
	}
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

// stale is true if no subscriber with the payer's key has been seen for visitorExpungeAfter
func (py payer) stale() bool {
	return time.Since(py.seen) > visitorExpungeAfter
}
