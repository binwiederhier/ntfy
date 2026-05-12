// Package monitor implements heartbeat / dead-man's-switch monitors. A monitor is an authenticated
// per-user record that expects to receive periodic pings on a heartbeat endpoint. If a ping does
// not arrive within period+grace, the server publishes a DOWN alert message to the configured
// alert topic. When pings resume, an UP recovery alert is published.
package monitor

import (
	"errors"
	"regexp"
)

// Monitor states
const (
	StatePending = "pending" // Created but no heartbeat received yet; never alerts
	StateUp      = "up"      // Last heartbeat was within period+grace; eligible for DOWN transition
	StateDown    = "down"    // Missed period+grace since last heartbeat; eligible for UP recovery
)

// Bounds for monitor configuration. These are sanity caps, not policy.
const (
	MinPeriodSeconds = 30
	MaxPeriodSeconds = 86400 // 24h
	MinGraceSeconds  = 10
	MaxGraceSeconds  = 86400

	MinAlertPriority = 1
	MaxAlertPriority = 5

	DefaultAlertPriority = 4 // High, but not Max
)

var (
	// keyRegex must remain in sync with the URL regex in server.go that maps /v1/heartbeat/<key>.
	keyRegex = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)

	// alertTopicRegex matches ntfy's topicRegex. Kept independently to avoid a server-package import.
	alertTopicRegex = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
)

// Errors returned by the Manager.
var (
	ErrMonitorNotFound       = errors.New("monitor not found")
	ErrMonitorExists         = errors.New("monitor already exists")
	ErrInvalidKey            = errors.New("invalid monitor key")
	ErrInvalidPeriod         = errors.New("invalid period")
	ErrInvalidGrace          = errors.New("invalid grace")
	ErrInvalidAlertTopic     = errors.New("invalid alert topic")
	ErrInvalidAlertPriority  = errors.New("invalid alert priority")
	ErrInvalidUserID         = errors.New("invalid user id")
)

// Monitor is a single heartbeat-tracked entity owned by a user.
type Monitor struct {
	ID            int64  // Auto-increment row id
	UserID        string // Owner; references user.id (no cross-DB FK)
	Key           string // User-chosen identifier, unique per UserID
	PeriodSeconds int64  // How often a ping is expected
	GraceSeconds  int64  // Slack allowed before declaring DOWN
	AlertTopic    string // ntfy topic to publish DOWN/UP alerts on
	AlertPriority int    // Priority for alert messages (1-5)
	State         string // pending | up | down
	LastSeenAt    int64  // Unix seconds; 0 means never seen
	CreatedAt     int64  // Unix seconds
}

// ValidateKey returns ErrInvalidKey if the key is malformed.
func ValidateKey(key string) error {
	if !keyRegex.MatchString(key) {
		return ErrInvalidKey
	}
	return nil
}

// ValidatePeriod returns ErrInvalidPeriod if the period is out of bounds.
func ValidatePeriod(seconds int64) error {
	if seconds < MinPeriodSeconds || seconds > MaxPeriodSeconds {
		return ErrInvalidPeriod
	}
	return nil
}

// ValidateGrace returns ErrInvalidGrace if the grace is out of bounds.
func ValidateGrace(seconds int64) error {
	if seconds < MinGraceSeconds || seconds > MaxGraceSeconds {
		return ErrInvalidGrace
	}
	return nil
}

// ValidateAlertTopic returns ErrInvalidAlertTopic if the topic is malformed.
func ValidateAlertTopic(topic string) error {
	if !alertTopicRegex.MatchString(topic) {
		return ErrInvalidAlertTopic
	}
	return nil
}

// ValidateAlertPriority returns ErrInvalidAlertPriority if the priority is out of bounds.
func ValidateAlertPriority(priority int) error {
	if priority < MinAlertPriority || priority > MaxAlertPriority {
		return ErrInvalidAlertPriority
	}
	return nil
}
