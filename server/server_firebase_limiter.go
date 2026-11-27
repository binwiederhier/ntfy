package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
	vis "heckel.io/ntfy/v2/visitor"
)

// Stopgap against FCM's hourly fanout budget (see ticket #26 and plan 261120-firebase-delivery):
// a visitor without a paid tier, and a topic, each get a token bucket for Firebase messages. A
// message goes to Firebase only if both have a token, and is not sent to Firebase at all
// otherwise. Casual use (a few messages in a row, ~10 per topic and hour) is not affected.
const (
	firebaseLimitSweepEvery = time.Minute
)

// DefaultFirebaseLimits are the limits if experimental-firebase-limit sets no other values
var DefaultFirebaseLimits = &FirebaseLimits{
	VisitorBurst:  5,
	VisitorRefill: time.Minute, // One token per minute
	TopicBurst:    10,
	TopicRefill:   6 * time.Minute, // Ten tokens per hour
}

// FirebaseLimits are the token bucket sizes of the Firebase limit, see experimental-firebase-limit
type FirebaseLimits struct {
	VisitorBurst  int
	VisitorRefill time.Duration // One token per this duration
	TopicBurst    int
	TopicRefill   time.Duration // One token per this duration
}

// firebaseLimiter holds the Firebase token buckets per visitor and per topic, across the cluster:
// local publishes and peer publishes both go through allowKeys. Full buckets are forgotten, so it
// only holds the visitors and topics that sent recently.
type firebaseLimiter struct {
	limits    *FirebaseLimits
	config    *vis.Config                       // To mask the senders of peer messages
	visitors  map[string]*firebaseLimiterBucket // By masked visitor IP
	topics    map[string]*firebaseLimiterBucket // By topic
	lastSweep time.Time
	mu        sync.Mutex // Protects visitors, topics and lastSweep
}

// firebaseLimiterBucket is one token bucket, refilled lazily when it is next used
type firebaseLimiterBucket struct {
	tokens float64
	last   time.Time
}

func newFirebaseLimiter(limits *FirebaseLimits, config *vis.Config) *firebaseLimiter {
	return &firebaseLimiter{
		limits:   limits,
		config:   config,
		visitors: make(map[string]*firebaseLimiterBucket),
		topics:   make(map[string]*firebaseLimiterBucket),
	}
}

// newFirebaseLimiterFromConfig returns a limiter if experimental-firebase-limit is set, and nil otherwise
func newFirebaseLimiterFromConfig(conf *Config) *firebaseLimiter {
	if conf.FirebaseLimit == nil {
		return nil
	}
	return newFirebaseLimiter(conf.FirebaseLimit, visitorConfigFrom(conf))
}

// ParseFirebaseLimits parses the experimental-firebase-limit option, e.g. "visitor-burst=5,
// visitor-refill=1m, topic-burst=10, topic-refill=6m"; missing values are the defaults, "defaults"
// is all defaults, and an empty string disables the limit (nil)
func ParseFirebaseLimits(s string) (*FirebaseLimits, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	limits := *DefaultFirebaseLimits
	if s == "defaults" {
		return &limits, nil
	}
	for _, pair := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			return nil, fmt.Errorf("invalid Firebase limit %q, expected key=value", pair)
		}
		var err error
		switch key {
		case "visitor-burst":
			limits.VisitorBurst, err = parseFirebaseLimitBurst(value)
		case "visitor-refill":
			limits.VisitorRefill, err = parseFirebaseLimitRefill(value)
		case "topic-burst":
			limits.TopicBurst, err = parseFirebaseLimitBurst(value)
		case "topic-refill":
			limits.TopicRefill, err = parseFirebaseLimitRefill(value)
		default:
			return nil, fmt.Errorf("unknown Firebase limit %q", key)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid Firebase limit %q: %w", key, err)
		}
	}
	return &limits, nil
}

// allowKeys returns true if a message from the visitor to the topic may go to Firebase, and takes a
// token from both buckets if so. A message that is not allowed takes no token.
func (l *firebaseLimiter) allowKeys(visitor, topic string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maybeSweepNoLock(now)
	vb := refillNoLock(l.visitors, visitor, l.limits.VisitorBurst, l.limits.VisitorRefill, now)
	tb := refillNoLock(l.topics, topic, l.limits.TopicBurst, l.limits.TopicRefill, now)
	if vb.tokens < 1 || tb.tokens < 1 {
		return false
	}
	vb.tokens--
	tb.tokens--
	return true
}

// Allow returns true if the message may go to Firebase. Only plain messages are limited
// (deletes, clears and upstream poll requests pass), and visitors with a paid tier are not.
func (l *firebaseLimiter) Allow(v *visitor, m *model.Message) bool {
	if m.Event != model.MessageEvent {
		return true
	} else if u := v.User(); u != nil && u.Tier != nil {
		return true
	}
	return l.allowKeys(v.MaskedIP(), m.Topic, time.Now())
}

// Observe takes the tokens a peer node used for its Firebase message. Messages of logged-in users
// are skipped: telling a paid tier from a free one would need a user lookup per message, and free
// accounts are a small share of the messages.
func (l *firebaseLimiter) Observe(m *model.Message) {
	if m.Event != model.MessageEvent || m.User != "" {
		return
	}
	l.allowKeys(vis.Mask(m.Sender, l.config).String(), m.Topic, time.Now())
}

// maybeSweepNoLock forgets the buckets that have refilled completely, at most once per sweep
// interval; a forgotten bucket starts out full, so forgetting it changes nothing
func (l *firebaseLimiter) maybeSweepNoLock(now time.Time) {
	if now.Sub(l.lastSweep) < firebaseLimitSweepEvery {
		return
	}
	l.lastSweep = now
	sweepNoLock(l.visitors, l.limits.VisitorBurst, l.limits.VisitorRefill, now)
	sweepNoLock(l.topics, l.limits.TopicBurst, l.limits.TopicRefill, now)
}

func parseFirebaseLimitBurst(value string) (int, error) {
	burst, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	} else if burst < 1 {
		return 0, errors.New("must be at least 1")
	}
	return burst, nil
}

func parseFirebaseLimitRefill(value string) (time.Duration, error) {
	refill, err := util.ParseDuration(value)
	if err != nil {
		return 0, err
	} else if refill <= 0 {
		return 0, errors.New("must be positive")
	}
	return refill, nil
}

func refillNoLock(buckets map[string]*firebaseLimiterBucket, key string, burst int, refill time.Duration, now time.Time) *firebaseLimiterBucket {
	b, ok := buckets[key]
	if !ok {
		b = &firebaseLimiterBucket{
			tokens: float64(burst),
			last:   now,
		}
		buckets[key] = b
		return b
	}
	b.tokens = min(float64(burst), b.tokens+float64(now.Sub(b.last))/float64(refill))
	b.last = now
	return b
}

func sweepNoLock(buckets map[string]*firebaseLimiterBucket, burst int, refill time.Duration, now time.Time) {
	for key, b := range buckets {
		if b.tokens+float64(now.Sub(b.last))/float64(refill) >= float64(burst) {
			delete(buckets, key)
		}
	}
}
