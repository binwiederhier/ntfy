package user

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestAllowedRole(t *testing.T) {
	require.True(t, AllowedRole(RoleUser))
	require.True(t, AllowedRole(RoleAdmin))
	require.False(t, AllowedRole(RoleAnonymous))
	require.False(t, AllowedRole(Role("invalid")))
	require.False(t, AllowedRole(Role("")))
	require.False(t, AllowedRole(Role("superadmin")))
}

func TestAllowedTopic(t *testing.T) {
	// Valid topics
	require.True(t, AllowedTopic("test"))
	require.True(t, AllowedTopic("mytopic"))
	require.True(t, AllowedTopic("topic123"))
	require.True(t, AllowedTopic("my-topic"))
	require.True(t, AllowedTopic("my_topic"))
	require.True(t, AllowedTopic("Topic123"))
	require.True(t, AllowedTopic("a"))
	require.True(t, AllowedTopic(strings.Repeat("a", 64))) // Max length

	// Invalid topics - wildcards not allowed
	require.False(t, AllowedTopic("topic*"))
	require.False(t, AllowedTopic("*"))
	require.False(t, AllowedTopic("my*topic"))

	// Invalid topics - special characters
	require.False(t, AllowedTopic("my topic"))  // Space
	require.False(t, AllowedTopic("my.topic"))  // Dot
	require.False(t, AllowedTopic("my/topic"))  // Slash
	require.False(t, AllowedTopic("my@topic"))  // At sign
	require.False(t, AllowedTopic("my+topic"))  // Plus
	require.False(t, AllowedTopic("topic!"))    // Exclamation
	require.False(t, AllowedTopic("topic#"))    // Hash
	require.False(t, AllowedTopic("topic$"))    // Dollar
	require.False(t, AllowedTopic("topic%"))    // Percent
	require.False(t, AllowedTopic("topic&"))    // Ampersand
	require.False(t, AllowedTopic("my\\topic")) // Backslash

	// Invalid topics - length
	require.False(t, AllowedTopic(""))                      // Empty
	require.False(t, AllowedTopic(strings.Repeat("a", 65))) // Too long
}

func TestAllowedTopicPattern(t *testing.T) {
	// Valid patterns - same as AllowedTopic
	require.True(t, AllowedTopicPattern("test"))
	require.True(t, AllowedTopicPattern("mytopic"))
	require.True(t, AllowedTopicPattern("topic123"))
	require.True(t, AllowedTopicPattern("my-topic"))
	require.True(t, AllowedTopicPattern("my_topic"))
	require.True(t, AllowedTopicPattern("a"))
	require.True(t, AllowedTopicPattern(strings.Repeat("a", 64))) // Max length

	// Valid patterns - with wildcards
	require.True(t, AllowedTopicPattern("*"))
	require.True(t, AllowedTopicPattern("topic*"))
	require.True(t, AllowedTopicPattern("*topic"))
	require.True(t, AllowedTopicPattern("my*topic"))
	require.True(t, AllowedTopicPattern("***"))
	require.True(t, AllowedTopicPattern("test_*"))
	require.True(t, AllowedTopicPattern("my-*-topic"))
	require.True(t, AllowedTopicPattern(strings.Repeat("*", 64))) // Max length with wildcards

	// Invalid patterns - special characters (other than wildcard)
	require.False(t, AllowedTopicPattern("my topic"))  // Space
	require.False(t, AllowedTopicPattern("my.topic"))  // Dot
	require.False(t, AllowedTopicPattern("my/topic"))  // Slash
	require.False(t, AllowedTopicPattern("my@topic"))  // At sign
	require.False(t, AllowedTopicPattern("my+topic"))  // Plus
	require.False(t, AllowedTopicPattern("topic!"))    // Exclamation
	require.False(t, AllowedTopicPattern("topic#"))    // Hash
	require.False(t, AllowedTopicPattern("topic$"))    // Dollar
	require.False(t, AllowedTopicPattern("topic%"))    // Percent
	require.False(t, AllowedTopicPattern("topic&"))    // Ampersand
	require.False(t, AllowedTopicPattern("my\\topic")) // Backslash

	// Invalid patterns - length
	require.False(t, AllowedTopicPattern(""))                      // Empty
	require.False(t, AllowedTopicPattern(strings.Repeat("a", 65))) // Too long
}

func TestValidPasswordHash(t *testing.T) {
	// Valid bcrypt hashes with different versions
	require.Nil(t, ValidPasswordHash("$2a$10$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy", 10))
	require.Nil(t, ValidPasswordHash("$2b$10$YLiO8U21sX1uhZamTLJXHuxgVC0Z/GKISibrKCLohPgtG7yIxSk4C", 10))
	require.Nil(t, ValidPasswordHash("$2y$12$1234567890123456789012u1234567890123456789012345678901", 10))

	// Valid hash with minimum cost
	require.Nil(t, ValidPasswordHash("$2a$04$1234567890123456789012u1234567890123456789012345678901", 4))

	// Invalid - wrong prefix
	require.Equal(t, ErrPasswordHashInvalid, ValidPasswordHash("$2c$10$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy", 10))
	require.Equal(t, ErrPasswordHashInvalid, ValidPasswordHash("$3a$10$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy", 10))
	require.Equal(t, ErrPasswordHashInvalid, ValidPasswordHash("bcrypt$10$hash", 10))
	require.Equal(t, ErrPasswordHashInvalid, ValidPasswordHash("nothash", 10))
	require.Equal(t, ErrPasswordHashInvalid, ValidPasswordHash("", 10))

	// Invalid - malformed hash
	require.NotNil(t, ValidPasswordHash("$2a$10$tooshort", 10))
	require.NotNil(t, ValidPasswordHash("$2a$10", 10))
	require.NotNil(t, ValidPasswordHash("$2a$", 10))

	// Invalid - cost too low
	require.Equal(t, ErrPasswordHashWeak, ValidPasswordHash("$2a$04$1234567890123456789012u1234567890123456789012345678901", 10))
	require.Equal(t, ErrPasswordHashWeak, ValidPasswordHash("$2a$09$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy", 10))

	// Edge case - cost exactly at minimum
	require.Nil(t, ValidPasswordHash("$2a$10$EEp6gBheOsqEFsXlo523E.gBVoeg1ytphXiEvTPlNzkenBlHZBPQy", 10))
}

func TestValidToken(t *testing.T) {
	// Valid tokens
	require.True(t, ValidToken("tk_1234567890123456789012345678x"))
	require.True(t, ValidToken("tk_abcdefghijklmnopqrstuvwxyzabc"))
	require.True(t, ValidToken("tk_ABCDEFGHIJKLMNOPQRSTUVWXYZABC"))
	require.True(t, ValidToken("tk_012345678901234567890123456ab"))
	require.True(t, ValidToken("tk_-----------------------------"))
	require.True(t, ValidToken("tk______________________________"))

	// Invalid tokens - wrong prefix
	require.False(t, ValidToken("tx_1234567890123456789012345678x"))
	require.False(t, ValidToken("tk1234567890123456789012345678xy"))
	require.False(t, ValidToken("token_1234567890123456789012345"))

	// Invalid tokens - wrong length
	require.False(t, ValidToken("tk_"))                               // Too short
	require.False(t, ValidToken("tk_123"))                            // Too short
	require.False(t, ValidToken("tk_123456789012345678901234567890")) // Too long (30 chars after prefix)
	require.False(t, ValidToken("tk_123456789012345678901234567"))    // Too short (28 chars)

	// Invalid tokens - invalid characters
	require.False(t, ValidToken("tk_123456789012345678901234567!@"))
	require.False(t, ValidToken("tk_12345678901234567890123456 8x"))
	require.False(t, ValidToken("tk_123456789012345678901234567.x"))
	require.False(t, ValidToken("tk_123456789012345678901234567*x"))

	// Invalid tokens - no prefix
	require.False(t, ValidToken("1234567890123456789012345678901x"))
	require.False(t, ValidToken(""))
}

func TestGenerateToken(t *testing.T) {
	// Generate multiple tokens
	tokens := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token := GenerateToken()

		// Check format
		require.True(t, strings.HasPrefix(token, "tk_"), "Token should start with tk_")
		require.Equal(t, 32, len(token), "Token should be 32 characters long")

		// Check it's valid
		require.True(t, ValidToken(token), "Generated token should be valid")

		// Check it's lowercase
		require.Equal(t, strings.ToLower(token), token, "Token should be lowercase")

		// Check uniqueness
		require.False(t, tokens[token], "Token should be unique")
		tokens[token] = true
	}

	// Verify we got 100 unique tokens
	require.Equal(t, 100, len(tokens))
}

func TestHashPassword(t *testing.T) {
	password := "test-password-123"

	// Hash the password
	hash, err := HashPassword(password)
	require.Nil(t, err)
	require.NotEmpty(t, hash)

	// Check it's a valid bcrypt hash
	require.Nil(t, ValidPasswordHash(hash, DefaultUserPasswordBcryptCost))

	// Check it starts with correct prefix
	require.True(t, strings.HasPrefix(hash, "$2a$"))

	// Hash the same password again - should produce different hash
	hash2, err := HashPassword(password)
	require.Nil(t, err)
	require.NotEqual(t, hash, hash2, "Same password should produce different hashes (salt)")

	// Empty password should still work
	emptyHash, err := HashPassword("")
	require.Nil(t, err)
	require.NotEmpty(t, emptyHash)
	require.Nil(t, ValidPasswordHash(emptyHash, DefaultUserPasswordBcryptCost))
}

func TestHashPassword_WithCost(t *testing.T) {
	password := "test-password"

	// Test with different costs
	hash4, err := hashPassword(password, 4)
	require.Nil(t, err)
	require.True(t, strings.HasPrefix(hash4, "$2a$04$"))

	hash10, err := hashPassword(password, 10)
	require.Nil(t, err)
	require.True(t, strings.HasPrefix(hash10, "$2a$10$"))

	hash12, err := hashPassword(password, 12)
	require.Nil(t, err)
	require.True(t, strings.HasPrefix(hash12, "$2a$12$"))

	// All should be valid
	require.Nil(t, ValidPasswordHash(hash4, 4))
	require.Nil(t, ValidPasswordHash(hash10, 10))
	require.Nil(t, ValidPasswordHash(hash12, 12))
}

func TestUser_TierID(t *testing.T) {
	// User with tier
	u := &User{
		Tier: &Tier{
			ID:   "ti_123",
			Code: "pro",
		},
	}
	require.Equal(t, "ti_123", u.TierID())

	// User without tier
	u2 := &User{
		Tier: nil,
	}
	require.Equal(t, "", u2.TierID())

	// Nil user
	var u3 *User
	require.Equal(t, "", u3.TierID())
}

func TestUser_IsAdmin(t *testing.T) {
	admin := &User{Role: RoleAdmin}
	require.True(t, admin.IsAdmin())
	require.False(t, admin.IsUser())

	user := &User{Role: RoleUser}
	require.False(t, user.IsAdmin())

	anonymous := &User{Role: RoleAnonymous}
	require.False(t, anonymous.IsAdmin())

	// Nil user
	var nilUser *User
	require.False(t, nilUser.IsAdmin())
}

func TestUser_IsUser(t *testing.T) {
	user := &User{Role: RoleUser}
	require.True(t, user.IsUser())
	require.False(t, user.IsAdmin())

	admin := &User{Role: RoleAdmin}
	require.False(t, admin.IsUser())

	anonymous := &User{Role: RoleAnonymous}
	require.False(t, anonymous.IsUser())

	// Nil user
	var nilUser *User
	require.False(t, nilUser.IsUser())
}

func TestPermission_String(t *testing.T) {
	require.Equal(t, "read-write", PermissionReadWrite.String())
	require.Equal(t, "read-only", PermissionRead.String())
	require.Equal(t, "write-only", PermissionWrite.String())
	require.Equal(t, "deny-all", PermissionDenyAll.String())
}
