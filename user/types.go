package user

import (
	"errors"
	"github.com/stripe/stripe-go/v74"
	"heckel.io/ntfy/log"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// User is a struct that represents a user
type User struct {
	ID        string
	Name      string
	Hash      string // password hash (bcrypt)
	Token     string // Only set if token was used to log in
	Role      Role
	Prefs     *Prefs
	Tier      *Tier
	Stats     *Stats
	Billing   *Billing
	SyncTopic string
	Deleted   bool
}

// TierID returns the ID of the User.Tier, or an empty string if the user has no tier,
// or if the user itself is nil.
func (u *User) TierID() string {
	if u == nil || u.Tier == nil {
		return ""
	}
	return u.Tier.ID
}

// IsAdmin returns true if the user is an admin
func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}

// IsUser returns true if the user is a regular user, not an admin
func (u *User) IsUser() bool {
	return u != nil && u.Role == RoleUser
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
	Value      string
	Label      string
	LastAccess time.Time
	LastOrigin netip.Addr
	Expires    time.Time
}

// TokenUpdate holds information about the last access time and origin IP address of a token
type TokenUpdate struct {
	LastAccess time.Time
	LastOrigin netip.Addr
}

// Prefs represents a user's configuration settings
type Prefs struct {
	Language      *string            `json:"language,omitempty"`
	Notification  *NotificationPrefs `json:"notification,omitempty"`
	Subscriptions []*Subscription    `json:"subscriptions,omitempty"`
}

// Tier represents a user's account type, including its account limits
type Tier struct {
	ID                       string        // Tier identifier (ti_...)
	Code                     string        // Code of the tier
	Name                     string        // Name of the tier
	MessageLimit             int64         // Daily message limit
	MessageExpiryDuration    time.Duration // Cache duration for messages
	EmailLimit               int64         // Daily email limit
	CallLimit                int64         // Daily phone call limit
	ReservationLimit         int64         // Number of topic reservations allowed by user
	AttachmentFileSizeLimit  int64         // Max file size per file (bytes)
	AttachmentTotalSizeLimit int64         // Total file size for all files of this user (bytes)
	AttachmentExpiryDuration time.Duration // Duration after which attachments will be deleted
	AttachmentBandwidthLimit int64         // Daily bandwidth limit for the user
	StripeMonthlyPriceID     string        // Monthly price ID for paid tiers (price_...)
	StripeYearlyPriceID      string        // Yearly price ID for paid tiers (price_...)
}

// Context returns fields for the log
func (t *Tier) Context() log.Context {
	return log.Context{
		"tier_id":                 t.ID,
		"tier_code":               t.Code,
		"stripe_monthly_price_id": t.StripeMonthlyPriceID,
		"stripe_yearly_price_id":  t.StripeYearlyPriceID,
	}
}

// Subscription represents a user's topic subscription
type Subscription struct {
	BaseURL     string  `json:"base_url"`
	Topic       string  `json:"topic"`
	DisplayName *string `json:"display_name"`
}

// Context returns fields for the log
func (s *Subscription) Context() log.Context {
	return log.Context{
		"base_url": s.BaseURL,
		"topic":    s.Topic,
	}
}

// NotificationPrefs represents the user's notification settings
type NotificationPrefs struct {
	Sound       *string `json:"sound,omitempty"`
	MinPriority *int    `json:"min_priority,omitempty"`
	DeleteAfter *int    `json:"delete_after,omitempty"`
}

// Stats is a struct holding daily user statistics
type Stats struct {
	Messages int64
	Emails   int64
	Calls    int64
}

// Billing is a struct holding a user's billing information
type Billing struct {
	StripeCustomerID            string
	StripeSubscriptionID        string
	StripeSubscriptionStatus    stripe.SubscriptionStatus
	StripeSubscriptionInterval  stripe.PriceRecurringInterval
	StripeSubscriptionPaidUntil time.Time
	StripeSubscriptionCancelAt  time.Time
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
	switch strings.ToLower(s) {
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
	Everyone   = "*"
	everyoneID = "u_everyone"
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
	ErrUnauthenticated     = errors.New("unauthenticated")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrUserNotFound        = errors.New("user not found")
	ErrUserExists          = errors.New("user already exists")
	ErrTierNotFound        = errors.New("tier not found")
	ErrTokenNotFound       = errors.New("token not found")
	ErrPhoneNumberNotFound = errors.New("phone number not found")
	ErrTooManyReservations = errors.New("new tier has lower reservation limit")
	ErrPhoneNumberExists   = errors.New("phone number already exists")
)
