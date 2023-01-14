// Package user deals with authentication and authorization against topics
package user

import (
	"errors"
	"regexp"
	"time"
)

// User is a struct that represents a user
type User struct {
	Name      string
	Hash      string // password hash (bcrypt)
	Token     string // Only set if token was used to log in
	Role      Role
	Prefs     *Prefs
	Tier      *Tier
	Stats     *Stats
	Billing   *Billing
	SyncTopic string
	Created   time.Time
	LastSeen  time.Time
}

// Auther is an interface for authentication and authorization
type Auther interface {
	// Authenticate checks username and password and returns a user if correct. The method
	// returns in constant-ish time, regardless of whether the user exists or the password is
	// correct or incorrect.
	Authenticate(username, password string) (*User, error)

	// Authorize returns nil if the given user has access to the given topic using the desired
	// permission. The user param may be nil to signal an anonymous user.
	Authorize(user *User, topic string, perm Permission) error
}

// Token represents a user token, including expiry date
type Token struct {
	Value   string
	Expires time.Time
}

// Prefs represents a user's configuration settings
type Prefs struct {
	Language      string             `json:"language,omitempty"`
	Notification  *NotificationPrefs `json:"notification,omitempty"`
	Subscriptions []*Subscription    `json:"subscriptions,omitempty"`
}

// Tier represents a user's account type, including its account limits
type Tier struct {
	Code                     string
	Name                     string
	Paid                     bool
	MessagesLimit            int64
	MessagesExpiryDuration   time.Duration
	EmailsLimit              int64
	ReservationsLimit        int64
	AttachmentFileSizeLimit  int64
	AttachmentTotalSizeLimit int64
	AttachmentExpiryDuration time.Duration
	StripePriceID            string
}

// Subscription represents a user's topic subscription
type Subscription struct {
	ID          string `json:"id"`
	BaseURL     string `json:"base_url"`
	Topic       string `json:"topic"`
	DisplayName string `json:"display_name"`
}

// NotificationPrefs represents the user's notification settings
type NotificationPrefs struct {
	Sound       string `json:"sound,omitempty"`
	MinPriority int    `json:"min_priority,omitempty"`
	DeleteAfter int    `json:"delete_after,omitempty"`
}

// Stats is a struct holding daily user statistics
type Stats struct {
	Messages int64
	Emails   int64
}

// Billing is a struct holding a user's billing information
type Billing struct {
	StripeCustomerID     string
	StripeSubscriptionID string
}

// Grant is a struct that represents an access control entry to a topic by a user
type Grant struct {
	TopicPattern string // May include wildcard (*)
	Allow        Permission
}

// Reservation is a struct that represents the ownership over a topic by a user
type Reservation struct {
	Topic    string
	Owner    Permission
	Everyone Permission
}

// Permission represents a read or write permission to a topic
type Permission uint8

// Permissions to a topic
const (
	PermissionDenyAll Permission = iota
	PermissionRead
	PermissionWrite
	PermissionReadWrite // 3!
)

// NewPermission is a helper to create a Permission based on read/write bool values
func NewPermission(read, write bool) Permission {
	p := uint8(0)
	if read {
		p |= uint8(PermissionRead)
	}
	if write {
		p |= uint8(PermissionWrite)
	}
	return Permission(p)
}

// ParsePermission parses the string representation and returns a Permission
func ParsePermission(s string) (Permission, error) {
	switch s {
	case "read-write", "rw":
		return NewPermission(true, true), nil
	case "read-only", "read", "ro":
		return NewPermission(true, false), nil
	case "write-only", "write", "wo":
		return NewPermission(false, true), nil
	case "deny-all", "deny", "none":
		return NewPermission(false, false), nil
	default:
		return NewPermission(false, false), errors.New("invalid permission")
	}
}

// IsRead returns true if readable
func (p Permission) IsRead() bool {
	return p&PermissionRead != 0
}

// IsWrite returns true if writable
func (p Permission) IsWrite() bool {
	return p&PermissionWrite != 0
}

// IsReadWrite returns true if readable and writable
func (p Permission) IsReadWrite() bool {
	return p.IsRead() && p.IsWrite()
}

// String returns a string representation of the permission
func (p Permission) String() string {
	if p.IsReadWrite() {
		return "read-write"
	} else if p.IsRead() {
		return "read-only"
	} else if p.IsWrite() {
		return "write-only"
	}
	return "deny-all"
}

// Role represents a user's role, either admin or regular user
type Role string

// User roles
const (
	RoleAdmin     = Role("admin") // Some queries have these values hardcoded!
	RoleUser      = Role("user")
	RoleAnonymous = Role("anonymous")
)

// Everyone is a special username representing anonymous users
const (
	Everyone = "*"
)

var (
	allowedUsernameRegex     = regexp.MustCompile(`^[-_.@a-zA-Z0-9]+$`)     // Does not include Everyone (*)
	allowedTopicRegex        = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)  // No '*'
	allowedTopicPatternRegex = regexp.MustCompile(`^[-_*A-Za-z0-9]{1,64}$`) // Adds '*' for wildcards!
	allowedTierRegex         = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
)

// AllowedRole returns true if the given role can be used for new users
func AllowedRole(role Role) bool {
	return role == RoleUser || role == RoleAdmin
}

// AllowedUsername returns true if the given username is valid
func AllowedUsername(username string) bool {
	return allowedUsernameRegex.MatchString(username)
}

// AllowedTopic returns true if the given topic name is valid
func AllowedTopic(topic string) bool {
	return allowedTopicRegex.MatchString(topic)
}

// AllowedTopicPattern returns true if the given topic pattern is valid; this includes the wildcard character (*)
func AllowedTopicPattern(topic string) bool {
	return allowedTopicPatternRegex.MatchString(topic)
}

// AllowedTier returns true if the given tier name is valid
func AllowedTier(tier string) bool {
	return allowedTierRegex.MatchString(tier)
}

// Error constants used by the package
var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrUserNotFound    = errors.New("user not found")
	ErrTierNotFound    = errors.New("tier not found")
)
