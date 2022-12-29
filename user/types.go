// Package user deals with authentication and authorization against topics
package user

import (
	"errors"
	"regexp"
	"time"
)

// User is a struct that represents a user
type User struct {
	Name   string
	Hash   string // password hash (bcrypt)
	Token  string // Only set if token was used to log in
	Role   Role
	Grants []Grant
	Prefs  *Prefs
	Plan   *Plan
	Stats  *Stats
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

// PlanCode is code identifying a user's plan
type PlanCode string

// Default plan codes
const (
	PlanUnlimited = PlanCode("unlimited")
	PlanDefault   = PlanCode("default")
	PlanNone      = PlanCode("none")
)

// Plan represents a user's account type, including its account limits
type Plan struct {
	Code                     string `json:"name"`
	Upgradable               bool   `json:"upgradable"`
	MessagesLimit            int64  `json:"messages_limit"`
	EmailsLimit              int64  `json:"emails_limit"`
	AttachmentFileSizeLimit  int64  `json:"attachment_file_size_limit"`
	AttachmentTotalSizeLimit int64  `json:"attachment_total_size_limit"`
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

// Grant is a struct that represents an access control entry to a topic
type Grant struct {
	TopicPattern string // May include wildcard (*)
	AllowRead    bool
	AllowWrite   bool
}

// Permission represents a read or write permission to a topic
type Permission int

// Permissions to a topic
const (
	PermissionRead  = Permission(1)
	PermissionWrite = Permission(2)
)

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
	allowedTopicPatternRegex = regexp.MustCompile(`^[-_*A-Za-z0-9]{1,64}$`) // Adds '*' for wildcards!
)

// AllowedRole returns true if the given role can be used for new users
func AllowedRole(role Role) bool {
	return role == RoleUser || role == RoleAdmin
}

// AllowedUsername returns true if the given username is valid
func AllowedUsername(username string) bool {
	return allowedUsernameRegex.MatchString(username)
}

// AllowedTopicPattern returns true if the given topic pattern is valid; this includes the wildcard character (*)
func AllowedTopicPattern(username string) bool {
	return allowedTopicPatternRegex.MatchString(username)
}

// Error constants used by the package
var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
)
