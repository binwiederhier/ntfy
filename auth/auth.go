// Package auth deals with authentication and authorization against topics
package auth

import (
	"errors"
	"regexp"
)

// Auther is a generic interface to implement password and token based authentication and authorization
type Auther interface {
	// Authenticate checks username and password and returns a user if correct. The method
	// returns in constant-ish time, regardless of whether the user exists or the password is
	// correct or incorrect.
	Authenticate(username, password string) (*User, error)

	AuthenticateToken(token string) (*User, error)
	CreateToken(user *User) (string, error)
	RemoveToken(user *User) error

	// Authorize returns nil if the given user has access to the given topic using the desired
	// permission. The user param may be nil to signal an anonymous user.
	Authorize(user *User, topic string, perm Permission) error
}

// Manager is an interface representing user and access management
type Manager interface {
	// AddUser adds a user with the given username, password and role. The password should be hashed
	// before it is stored in a persistence layer.
	AddUser(username, password string, role Role) error

	// RemoveUser deletes the user with the given username. The function returns nil on success, even
	// if the user did not exist in the first place.
	RemoveUser(username string) error

	// Users returns a list of users. It always also returns the Everyone user ("*").
	Users() ([]*User, error)

	// User returns the user with the given username if it exists, or ErrNotFound otherwise.
	// You may also pass Everyone to retrieve the anonymous user and its Grant list.
	User(username string) (*User, error)

	// ChangePassword changes a user's password
	ChangePassword(username, password string) error

	// ChangeRole changes a user's role. When a role is changed from RoleUser to RoleAdmin,
	// all existing access control entries (Grant) are removed, since they are no longer needed.
	ChangeRole(username string, role Role) error

	// AllowAccess adds or updates an entry in th access control list for a specific user. It controls
	// read/write access to a topic. The parameter topicPattern may include wildcards (*).
	AllowAccess(username string, topicPattern string, read bool, write bool) error

	// ResetAccess removes an access control list entry for a specific username/topic, or (if topic is
	// empty) for an entire user. The parameter topicPattern may include wildcards (*).
	ResetAccess(username string, topicPattern string) error

	// DefaultAccess returns the default read/write access if no access control entry matches
	DefaultAccess() (read bool, write bool)
}

// User is a struct that represents a user
type User struct {
	Name     string
	Hash     string // password hash (bcrypt)
	Token    string // Only set if token was used to log in
	Role     Role
	Grants   []Grant
	Language string
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
	RoleAdmin     = Role("admin")
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
