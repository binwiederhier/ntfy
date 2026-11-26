package sprig

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash/adler32"
)

// sha512sum computes the SHA-512 hash of the input string and returns it as a hex-encoded string.
// This function can be used in templates to generate secure hashes of sensitive data.
//
// Example usage in templates: {{ "hello world" | sha512sum }}
func sha512sum(input string) string {
	hash := sha512.Sum512([]byte(input))
	return hex.EncodeToString(hash[:])
}

// sha256sum computes the SHA-256 hash of the input string and returns it as a hex-encoded string.
// This is a commonly used cryptographic hash function that produces a 256-bit (32-byte) hash value.
//
// Example usage in templates: {{ "hello world" | sha256sum }}
func sha256sum(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// sha1sum computes the SHA-1 hash of the input string and returns it as a hex-encoded string.
// Note: SHA-1 is no longer considered secure against well-funded attackers for cryptographic purposes.
// Consider using sha256sum or sha512sum for security-critical applications.
//
// Example usage in templates: {{ "hello world" | sha1sum }}
func sha1sum(input string) string {
	hash := sha1.Sum([]byte(input))
	return hex.EncodeToString(hash[:])
}

// adler32sum computes the Adler-32 checksum of the input string and returns it as a decimal string.
// This is a non-cryptographic hash function primarily used for error detection.
//
// Example usage in templates: {{ "hello world" | adler32sum }}
func adler32sum(input string) string {
	hash := adler32.Checksum([]byte(input))
	return fmt.Sprintf("%d", hash)
}
