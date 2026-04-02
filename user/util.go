package user

import (
	"database/sql"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/v2/util"
)

var (
	allowedUsernameRegex     = regexp.MustCompile(`^[-_.+@a-zA-Z0-9]+$`)    // Does not include Everyone (*)
	allowedTopicRegex        = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)  // No '*'
	allowedTopicPatternRegex = regexp.MustCompile(`^[-_*A-Za-z0-9]{1,64}$`) // Adds '*' for wildcards!
	allowedTierRegex         = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
	allowedTokenRegex        = regexp.MustCompile(`^tk_[-_A-Za-z0-9]{29}$`) // Must be tokenLength-len(tokenPrefix)
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

// ValidPasswordHash checks if the given password hash is a valid bcrypt hash
func ValidPasswordHash(hash string, minCost int) error {
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") && !strings.HasPrefix(hash, "$2y$") {
		return ErrPasswordHashInvalid
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil { // Check if the hash is valid (length, format, etc.)
		return err
	} else if cost < minCost {
		return ErrPasswordHashWeak
	}
	return nil
}

// ValidToken returns true if the given token matches the naming convention
func ValidToken(token string) bool {
	return allowedTokenRegex.MatchString(token)
}

// GenerateToken generates a new token with a prefix and a fixed length
// Lowercase only to support "<topic>+<token>@<domain>" email addresses
func GenerateToken() string {
	return util.RandomLowerStringPrefix(tokenPrefix, tokenLength)
}

// HashPassword hashes the given password using bcrypt with the configured cost
func HashPassword(password string) (string, error) {
	return hashPassword(password, DefaultUserPasswordBcryptCost)
}

func hashPassword(password string, cost int) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

// toSQLWildcard converts a wildcard string to a SQL wildcard string. It only allows '*' as wildcards,
// and escapes '_', assuming '\' as escape character.
func toSQLWildcard(s string) string {
	return escapeUnderscore(strings.ReplaceAll(s, "*", "%"))
}

// fromSQLWildcard converts a SQL wildcard string to a wildcard string. It converts '%' to '*',
// and removes the '\_' escape character.
func fromSQLWildcard(s string) string {
	return strings.ReplaceAll(unescapeUnderscore(s), "%", "*")
}

func escapeUnderscore(s string) string {
	return strings.ReplaceAll(s, "_", "\\_")
}

func unescapeUnderscore(s string) string {
	return strings.ReplaceAll(s, "\\_", "_")
}
