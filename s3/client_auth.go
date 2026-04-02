package s3

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// signV4 signs req in place using AWS Signature V4. payloadHash is the hex-encoded SHA-256
// of the request body, or the literal string "UNSIGNED-PAYLOAD" for streaming uploads.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-authenticating-requests.html
func (c *Client) signV4(req *http.Request, hash string) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	// Required headers
	req.Header.Set("Host", c.config.HostHeader())
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", hash)

	// Canonical headers (all headers we set, sorted by lowercase key)
	signedKeys := make([]string, 0, len(req.Header))
	canonHeaders := make(map[string]string, len(req.Header))
	for k := range req.Header {
		lk := strings.ToLower(k)
		signedKeys = append(signedKeys, lk)
		canonHeaders[lk] = strings.TrimSpace(req.Header.Get(k))
	}
	sort.Strings(signedKeys)
	signedHeadersStr := strings.Join(signedKeys, ";")
	var chBuf strings.Builder
	for _, k := range signedKeys {
		chBuf.WriteString(k)
		chBuf.WriteByte(':')
		chBuf.WriteString(canonHeaders[k])
		chBuf.WriteByte('\n')
	}

	// Canonical request
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL.Query()),
		chBuf.String(),
		signedHeadersStr,
		hash,
	}, "\n")

	// String to sign
	credentialScope := datestamp + "/" + c.config.Region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + credentialScope + "\n" + sha256Hex([]byte(canonicalRequest))

	// Signing key
	signingKey := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256(
		[]byte("AWS4"+c.config.SecretKey), []byte(datestamp)),
		[]byte(c.config.Region)),
		[]byte("s3")),
		[]byte("aws4_request"))

	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))
	header := fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.config.AccessKey, credentialScope, signedHeadersStr, signature,
	)
	req.Header.Set("Authorization", header)
}
