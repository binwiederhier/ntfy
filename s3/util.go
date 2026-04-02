package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	// SHA-256 hash of the empty string, used as the payload hash for bodiless requests
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Sent as the payload hash for streaming uploads where the body is not buffered in memory
	unsignedPayload = "UNSIGNED-PAYLOAD"

	// maxResponseBytes caps the size of S3 response bodies we read into memory
	maxResponseBytes = 2 * 1024 * 1024

	// partSize is the size of each part for multipart uploads (5 MB). This is also the threshold
	// above which PutObject switches from a simple PUT to multipart upload. S3 requires a minimum
	// part size of 5 MB for all parts except the last.
	partSize = 5 * 1024 * 1024

	// maxSinglePutSize is the maximum size for a single PUT upload (5 GB).
	// Objects larger than this must use multipart upload.
	maxSinglePutSize = 5 * 1024 * 1024 * 1024

	// maxPages is the max number of pages to iterate through when listing objects
	maxPages = 500

	// maxDeleteBatchSize is the maximum number of keys per S3 DeleteObjects call
	maxDeleteBatchSize = 1000
)

// ParseURL parses an S3 URL of the form:
//
//	s3://ACCESS_KEY:SECRET_KEY@BUCKET[/PREFIX]?region=REGION[&endpoint=ENDPOINT][&disable_http2=true]
//
// When endpoint is specified, path-style addressing is enabled automatically.
// When disable_http2=true is set, the client forces HTTP/1.1 to work around
// HTTP/2 stream errors with some S3-compatible providers (e.g. DigitalOcean Spaces).
func ParseURL(s3URL string) (*Config, error) {
	u, err := url.Parse(s3URL)
	if err != nil {
		return nil, fmt.Errorf("s3: invalid URL: %w", err)
	}
	if u.Scheme != "s3" {
		return nil, fmt.Errorf("s3: URL scheme must be 's3', got '%s'", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("s3: bucket name must be specified as host")
	}
	bucket := u.Host
	prefix := strings.TrimPrefix(u.Path, "/")
	accessKey := u.User.Username()
	secretKey, _ := u.User.Password()
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("s3: access key and secret key must be specified in URL")
	}
	region := u.Query().Get("region")
	if region == "" {
		return nil, fmt.Errorf("s3: region query parameter is required")
	}
	endpointParam := u.Query().Get("endpoint")
	var endpoint string
	var pathStyle bool
	if endpointParam != "" {
		// Custom endpoint: strip scheme prefix to extract host[:port]
		ep := strings.TrimRight(endpointParam, "/")
		ep = strings.TrimPrefix(ep, "https://")
		ep = strings.TrimPrefix(ep, "http://")
		endpoint = ep
		pathStyle = true
	} else {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", region)
		pathStyle = false
	}
	disableHTTP2, _ := strconv.ParseBool(u.Query().Get("disable_http2"))
	return &Config{
		Endpoint:     endpoint,
		PathStyle:    pathStyle,
		Bucket:       bucket,
		Prefix:       prefix,
		Region:       region,
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		DisableHTTP2: disableHTTP2,
	}, nil
}

// parseError reads an S3 error response and returns an *errorResponse.
func parseError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("error reading S3 error response: %w", err)
	}
	return parseErrorFromBytes(resp.StatusCode, body)
}

func parseErrorFromBytes(statusCode int, body []byte) error {
	errResp := &errorResponse{
		StatusCode: statusCode,
		Body:       string(body),
	}
	// Try to parse XML error; if it fails, we still have StatusCode and Body
	_ = xml.Unmarshal(body, errResp)
	return errResp
}

// canonicalURI returns the URI-encoded path for the canonical request. Each path segment is
// percent-encoded per RFC 3986; forward slashes are preserved.
func canonicalURI(u *url.URL) string {
	p := u.Path
	if p == "" {
		return "/"
	}
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = uriEncode(seg)
	}
	return strings.Join(segments, "/")
}

// canonicalQueryString builds the query string for the canonical request. Keys and values
// are URI-encoded per RFC 3986 (using %20, not +) and sorted lexically by key.
func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		ek := uriEncode(k)
		vs := make([]string, len(values[k]))
		copy(vs, values[k])
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, ek+"="+uriEncode(v))
		}
	}
	return strings.Join(pairs, "&")
}

// uriEncode percent-encodes a string per RFC 3986, encoding everything except unreserved
// characters (A-Z a-z 0-9 - _ . ~).
func uriEncode(s string) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~' {
			buf.WriteByte(b)
		} else {
			fmt.Fprintf(&buf, "%%%02X", b)
		}
	}
	return buf.String()
}

func isHTTPSuccess(resp *http.Response) bool {
	return resp.StatusCode/100 == 2
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
