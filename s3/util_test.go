package s3

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURIEncode(t *testing.T) {
	// Unreserved characters are not encoded
	require.Equal(t, "abcdefghijklmnopqrstuvwxyz", uriEncode("abcdefghijklmnopqrstuvwxyz"))
	require.Equal(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ", uriEncode("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	require.Equal(t, "0123456789", uriEncode("0123456789"))
	require.Equal(t, "-_.~", uriEncode("-_.~"))

	// Spaces use %20, not +
	require.Equal(t, "hello%20world", uriEncode("hello world"))

	// Slashes are encoded (canonicalURI handles slash splitting separately)
	require.Equal(t, "a%2Fb", uriEncode("a/b"))

	// Special characters
	require.Equal(t, "%2B", uriEncode("+"))
	require.Equal(t, "%3D", uriEncode("="))
	require.Equal(t, "%26", uriEncode("&"))
	require.Equal(t, "%40", uriEncode("@"))
	require.Equal(t, "%23", uriEncode("#"))

	// Mixed
	require.Equal(t, "test~file-name_1.txt", uriEncode("test~file-name_1.txt"))
	require.Equal(t, "key%20with%20spaces%2Fand%2Fslashes", uriEncode("key with spaces/and/slashes"))

	// Empty string
	require.Equal(t, "", uriEncode(""))
}

func TestCanonicalURI(t *testing.T) {
	// Simple path
	u, _ := url.Parse("https://example.com/bucket/key")
	require.Equal(t, "/bucket/key", canonicalURI(u))

	// Root path
	u, _ = url.Parse("https://example.com/")
	require.Equal(t, "/", canonicalURI(u))

	// Empty path
	u, _ = url.Parse("https://example.com")
	require.Equal(t, "/", canonicalURI(u))

	// Path with special characters
	u, _ = url.Parse("https://example.com/bucket/key%20with%20spaces")
	require.Equal(t, "/bucket/key%20with%20spaces", canonicalURI(u))

	// Nested path
	u, _ = url.Parse("https://example.com/bucket/a/b/c/file.txt")
	require.Equal(t, "/bucket/a/b/c/file.txt", canonicalURI(u))
}

func TestCanonicalQueryString(t *testing.T) {
	// Multiple keys sorted alphabetically
	vals := url.Values{
		"prefix":    {"test/"},
		"list-type": {"2"},
	}
	require.Equal(t, "list-type=2&prefix=test%2F", canonicalQueryString(vals))

	// Empty values
	require.Equal(t, "", canonicalQueryString(url.Values{}))

	// Single key
	require.Equal(t, "key=value", canonicalQueryString(url.Values{"key": {"value"}}))

	// Key with multiple values (sorted)
	vals = url.Values{"key": {"b", "a"}}
	require.Equal(t, "key=a&key=b", canonicalQueryString(vals))

	// Keys requiring encoding
	vals = url.Values{"continuation-token": {"abc+def"}}
	require.Equal(t, "continuation-token=abc%2Bdef", canonicalQueryString(vals))
}

func TestSHA256Hex(t *testing.T) {
	// SHA-256 of empty string
	require.Equal(t, emptyPayloadHash, sha256Hex([]byte("")))

	// SHA-256 of known value
	require.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", sha256Hex([]byte("hello")))
}

func TestHmacSHA256(t *testing.T) {
	// Known test vector: HMAC-SHA256("key", "message")
	result := hmacSHA256([]byte("key"), []byte("message"))
	require.Len(t, result, 32) // SHA-256 produces 32 bytes
	require.NotEqual(t, make([]byte, 32), result)

	// Same inputs should produce same output
	result2 := hmacSHA256([]byte("key"), []byte("message"))
	require.Equal(t, result, result2)

	// Different inputs should produce different output
	result3 := hmacSHA256([]byte("different-key"), []byte("message"))
	require.NotEqual(t, result, result3)
}

func TestSignV4_SetsRequiredHeaders(t *testing.T) {
	c := &Client{config: &Config{
		AccessKey: "AKID",
		SecretKey: "SECRET",
		Region:    "us-east-1",
		Endpoint:  "s3.us-east-1.amazonaws.com",
		Bucket:    "my-bucket",
	}}

	req, _ := http.NewRequest(http.MethodGet, "https://my-bucket.s3.us-east-1.amazonaws.com/test-key", nil)
	c.signV4(req, emptyPayloadHash)

	// All required SigV4 headers must be set
	require.NotEmpty(t, req.Header.Get("Host"))
	require.NotEmpty(t, req.Header.Get("X-Amz-Date"))
	require.Equal(t, emptyPayloadHash, req.Header.Get("X-Amz-Content-Sha256"))

	// Authorization header must have correct format
	auth := req.Header.Get("Authorization")
	require.Contains(t, auth, "AWS4-HMAC-SHA256")
	require.Contains(t, auth, "Credential=AKID/")
	require.Contains(t, auth, "/us-east-1/s3/aws4_request")
	require.Contains(t, auth, "SignedHeaders=")
	require.Contains(t, auth, "Signature=")
}

func TestSignV4_UnsignedPayload(t *testing.T) {
	c := &Client{config: &Config{
		AccessKey: "AKID",
		SecretKey: "SECRET",
		Region:    "us-east-1",
		Endpoint:  "s3.us-east-1.amazonaws.com",
		Bucket:    "my-bucket",
	}}

	req, _ := http.NewRequest(http.MethodPut, "https://my-bucket.s3.us-east-1.amazonaws.com/test-key", nil)
	c.signV4(req, unsignedPayload)

	require.Equal(t, unsignedPayload, req.Header.Get("X-Amz-Content-Sha256"))
}

func TestSignV4_DifferentRegions(t *testing.T) {
	c1 := &Client{config: &Config{AccessKey: "AKID", SecretKey: "SECRET", Region: "us-east-1", Endpoint: "s3.us-east-1.amazonaws.com", Bucket: "b"}}
	c2 := &Client{config: &Config{AccessKey: "AKID", SecretKey: "SECRET", Region: "eu-west-1", Endpoint: "s3.eu-west-1.amazonaws.com", Bucket: "b"}}

	req1, _ := http.NewRequest(http.MethodGet, "https://b.s3.us-east-1.amazonaws.com/key", nil)
	c1.signV4(req1, emptyPayloadHash)

	req2, _ := http.NewRequest(http.MethodGet, "https://b.s3.eu-west-1.amazonaws.com/key", nil)
	c2.signV4(req2, emptyPayloadHash)

	// Different regions should produce different signatures
	require.NotEqual(t, req1.Header.Get("Authorization"), req2.Header.Get("Authorization"))
}

func TestParseError_XMLResponse(t *testing.T) {
	xmlBody := []byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`)
	err := parseErrorFromBytes(404, xmlBody)

	var errResp *errorResponse
	require.ErrorAs(t, err, &errResp)
	require.Equal(t, 404, errResp.StatusCode)
	require.Equal(t, "NoSuchKey", errResp.Code)
	require.Equal(t, "The specified key does not exist.", errResp.Message)
}

func TestParseError_NonXMLResponse(t *testing.T) {
	err := parseErrorFromBytes(500, []byte("internal server error"))

	var errResp *errorResponse
	require.ErrorAs(t, err, &errResp)
	require.Equal(t, 500, errResp.StatusCode)
	require.Equal(t, "", errResp.Code) // XML parsing failed, no code
	require.Contains(t, errResp.Body, "internal server error")
}
