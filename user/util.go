package user

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"math"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"heckel.io/ntfy/v2/util"
)

// linkTokenLength is the length of a raw magic-link token. At 48 base62 characters
// it carries ~285 bits of entropy, well above the ~256-bit target, so the tokens
// need no brute-force cap -- just expiry and single-use.
const linkTokenLength = 48

var (
	allowedUsernameRegex     = regexp.MustCompile(`^[-_.+@a-zA-Z0-9]+$`)    // Does not include Everyone (*)
	allowedTopicRegex        = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)  // No '*'
	allowedTopicPatternRegex = regexp.MustCompile(`^[-_*A-Za-z0-9]{1,64}$`) // Adds '*' for wildcards!
	allowedTierRegex         = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
	allowedTokenRegex        = regexp.MustCompile(`^tk_[-_A-Za-z0-9]{29}$`) // Must be tokenLength-len(tokenPrefix)
	sqlLikeEscaper           = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
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

// generateLinkToken returns a fresh high-entropy raw token for a magic link
// (email verification or password reset). The raw token is carried in the emailed
// link; only its hashToken digest is persisted.
func generateLinkToken() string {
	return util.RandomString(linkTokenLength)
}

// hashToken returns the hex-encoded SHA-256 digest of a raw magic-link token.
// Tokens are stored hashed so a database read cannot yield working links; a high-entropy
// token makes a fast (unsalted) hash sufficient, unlike a password.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// HashPassword hashes the given password using bcrypt with the given cost
func HashPassword(password string, cost int) (string, error) {
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

// toSQLSearchPattern converts a search string to a case-insensitive "contains" LIKE pattern (to be
// compared against a LOWER()ed column), escaping LIKE wildcards so they are matched literally.
func toSQLSearchPattern(s string) string {
	return "%" + sqlLikeEscaper.Replace(strings.ToLower(s)) + "%"
}

// toSQLLimit converts a page limit to a SQL LIMIT value; zero or negative means no limit
func toSQLLimit(limit int) int64 {
	if limit <= 0 {
		return math.MaxInt32
	}
	return int64(limit)
}
