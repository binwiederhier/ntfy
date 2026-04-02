package s3

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // MD5 is required by the S3 protocol for Content-MD5 headers
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"heckel.io/ntfy/v2/log"
)

const (
	tagS3Client = "s3_client"
)

// Client is a minimal S3-compatible client. It supports PutObject, GetObject, DeleteObjects,
// and ListObjectsV2 operations using AWS Signature V4 signing. The bucket and optional key prefix
// are fixed at construction time. All operations target the same bucket and prefix.
//
// The following IAM policy is required for AWS S3:
//
//	{
//	    "Version": "2012-10-17",
//	    "Statement": [
//	        {
//	            "Effect": "Allow",
//	            "Action": [
//	                "s3:ListBucket",
//	                "s3:ListBucketMultipartUploads"
//	            ],
//	            "Resource": "arn:aws:s3:::BUCKET_NAME"
//	        },
//	        {
//	            "Effect": "Allow",
//	            "Action": [
//	                "s3:GetObject",
//	                "s3:PutObject",
//	                "s3:DeleteObject",
//	                "s3:AbortMultipartUpload"
//	            ],
//	            "Resource": "arn:aws:s3:::BUCKET_NAME/*"
//	        }
//	    ]
//	}
//
// Fields must not be modified after the Client is passed to any method or goroutine.
type Client struct {
	config *Config
	http   *http.Client
}

// New creates a new S3 client from the given Config.
func New(config *Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		if config.DisableHTTP2 {
			httpClient = newHTTP1Client()
		} else {
			httpClient = http.DefaultClient
		}
	}
	return &Client{
		config: config,
		http:   httpClient,
	}
}

// PutObject uploads body to the given key. The key is automatically prefixed with the client's
// configured prefix.
//
// If untrustedLength is between 1 and 5 GB, the body is streamed directly to S3 via a
// single PUT request without buffering. The read is limited to untrustedLength bytes;
// any extra data in the body is ignored. If the body is shorter than claimed, the upload fails.
//
// Otherwise (untrustedLength <= 0 or > 5 GB), the first 5 MB are buffered to decide
// between a simple PUT and multipart upload.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html
// and https://docs.aws.amazon.com/AmazonS3/latest/API/API_CreateMultipartUpload.html
func (c *Client) PutObject(ctx context.Context, key string, body io.Reader, untrustedLength int64) error {
	if untrustedLength > 0 && untrustedLength <= maxSinglePutSize {
		// Stream directly: Content-Length is known (but untrusted). LimitReader ensures we send at most
		// untrustedLength bytes, and any extra data in body is ignored.
		return c.putObject(ctx, key, io.LimitReader(body, untrustedLength), untrustedLength)
	}
	// Buffered path: read first 5 MB to decide simple vs multipart
	first := make([]byte, partSize)
	n, err := io.ReadFull(body, first)
	if errors.Is(err, io.ErrUnexpectedEOF) || err == io.EOF {
		return c.putObject(ctx, key, bytes.NewReader(first[:n]), int64(n))
	} else if err != nil {
		return fmt.Errorf("error reading object %s from client: %w", key, err)
	}
	return c.putObjectMultipart(ctx, key, io.MultiReader(bytes.NewReader(first), body))
}

// putObject uploads a body with known size using a simple PUT with UNSIGNED-PAYLOAD.
func (c *Client) putObject(ctx context.Context, key string, body io.Reader, size int64) error {
	log.Tag(tagS3Client).Debug("Uploading object %s (%d bytes)", key, size)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.config.ObjectURL(key), body)
	if err != nil {
		return fmt.Errorf("creating upload request object %s failed: %w", key, err)
	}
	req.ContentLength = size
	c.signV4(req, unsignedPayload)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("uploading object %s failed: %w", key, err)
	}
	defer resp.Body.Close()
	if !isHTTPSuccess(resp) {
		return parseError(resp)
	}
	return nil
}

// GetObject downloads an object. The key is automatically prefixed with the client's configured
// prefix. The caller must close the returned ReadCloser.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/API_GetObject.html
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	log.Tag(tagS3Client).Debug("Fetching object %s", key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.ObjectURL(key), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("error creating HTTP GET request for %s: %w", key, err)
	}
	c.signV4(req, emptyPayloadHash)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error fetching object %s: %w", key, err)
	} else if !isHTTPSuccess(resp) {
		err := parseError(resp)
		resp.Body.Close()
		return nil, 0, err
	}
	return resp.Body, resp.ContentLength, nil
}

// ListObjectsV2 returns all objects under the client's configured prefix by paginating through
// ListObjectsV2 results automatically. Keys in the returned objects have the prefix stripped,
// so they match the keys used with PutObject/GetObject/DeleteObjects. It stops after 10,000
// pages as a safety valve.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html
func (c *Client) ListObjectsV2(ctx context.Context) ([]*Object, error) {
	var all []*Object
	var token string
	for page := 0; page < maxPages; page++ {
		result, err := c.listObjectsV2(ctx, token)
		if err != nil {
			return nil, err
		}
		for _, obj := range result.Contents {
			var lastModified time.Time
			if obj.LastModified != "" {
				lastModified, _ = time.Parse(time.RFC3339, obj.LastModified)
			}
			all = append(all, &Object{
				Key:          c.config.StripPrefix(obj.Key),
				Size:         obj.Size,
				LastModified: lastModified,
			})
		}
		if !result.IsTruncated {
			return all, nil
		}
		token = result.NextContinuationToken
	}
	return nil, fmt.Errorf("listing objects exceeded %d pages", maxPages)
}

// listObjectsV2 performs a single ListObjectsV2 request using the client's configured prefix.
func (c *Client) listObjectsV2(ctx context.Context, continuationToken string) (*listObjectsV2Result, error) {
	if continuationToken == "" {
		log.Tag(tagS3Client).Debug("Listing remote objects")
	} else {
		log.Tag(tagS3Client).Debug("Listing remote objects, continuing with token '%s'", continuationToken)
	}
	query := url.Values{"list-type": {"2"}}
	if prefix := c.config.ListPrefix(); prefix != "" {
		query.Set("prefix", prefix)
	}
	if continuationToken != "" {
		query.Set("continuation-token", continuationToken)
	}
	respBody, err := c.do(ctx, "ListObjects", http.MethodGet, c.config.BucketURL()+"?"+query.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	var result listObjectsV2Result
	if err := xml.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal list object response: %w", err)
	}
	return &result, nil
}

// DeleteObjects removes multiple objects in a single batch request. Keys are automatically
// prefixed with the client's configured prefix. S3 supports up to 1000 keys per call; the
// caller is responsible for batching if needed.
//
// Even when S3 returns HTTP 200, individual keys may fail. If any per-key errors are present
// in the response, they are returned as a combined error.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/API_DeleteObjects.html
func (c *Client) DeleteObjects(ctx context.Context, keys []string) error {
	// S3 DeleteObjects supports up to 1000 keys per call
	for i := 0; i < len(keys); i += maxDeleteBatchSize {
		end := i + maxDeleteBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		if err := c.deleteObjects(ctx, keys[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) deleteObjects(ctx context.Context, keys []string) error {
	log.Tag(tagS3Client).Debug("Deleting %d object(s)", len(keys))
	req := &deleteObjectsRequest{
		Quiet: true,
	}
	for _, key := range keys {
		req.Objects = append(req.Objects, &deleteObject{Key: c.config.ObjectKey(key)})
	}
	body, err := xml.Marshal(req)
	if err != nil {
		return fmt.Errorf("error marshalling XML for deleting objects: %w", err)
	}

	// Content-MD5 is required by the S3 protocol for DeleteObjects requests.
	md5Sum := md5.Sum(body) //nolint:gosec
	headers := map[string]string{
		"Content-MD5": base64.StdEncoding.EncodeToString(md5Sum[:]),
	}
	reqURL := c.config.BucketURL() + "?delete"
	respBody, err := c.do(ctx, "DeleteObjects", http.MethodPost, reqURL, body, headers)
	if err != nil {
		return fmt.Errorf("error deleting objects: %w", err)
	}

	// S3 may return HTTP 200 with per-key errors in the response body
	var result deleteObjectsResult
	if err := xml.Unmarshal(respBody, &result); err != nil {
		return nil // If we can't parse, assume success (Quiet mode returns empty body on success)
	}
	if len(result.Errors) > 0 {
		var msgs []string
		for _, e := range result.Errors {
			msgs = append(msgs, fmt.Sprintf("%s: %s", e.Key, e.Message))
		}
		return fmt.Errorf("error deleting objects, partial failure: %s", strings.Join(msgs, "; "))
	}
	return nil
}

// do creates a signed request, executes it, reads the response body, and checks for errors.
// If body is nil, the request is sent with an empty payload. If body is non-nil, it is sent
// with a computed SHA-256 payload hash and Content-Type: application/xml.
func (c *Client) do(ctx context.Context, op, method, reqURL string, body []byte, headers map[string]string) ([]byte, error) {
	log.Tag(tagS3Client).Trace("Performing request %s %s %s (body: %d bytes)", op, method, reqURL, len(body))
	var reader io.Reader
	var hash string
	if body != nil {
		reader = bytes.NewReader(body)
		hash = sha256Hex(body)
	} else {
		hash = emptyPayloadHash
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reader)
	if err != nil {
		return nil, fmt.Errorf("s3: %s request: %w", op, err)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		req.Header.Set("Content-Type", "application/xml")
	} else {
		req.ContentLength = 0
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.signV4(req, hash)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s3: %s: %w", op, err)
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("s3: %s read: %w", op, err)
	}
	if !isHTTPSuccess(resp) {
		return nil, parseErrorFromBytes(resp.StatusCode, respBody)
	}
	return respBody, nil
}

// newHTTP1Client creates an HTTP client that forces HTTP/1.1 by disabling HTTP/2
// ALPN negotiation. This works around HTTP/2 stream errors with some S3-compatible
// providers (e.g. DigitalOcean Spaces) that can cause non-retryable failures on
// streaming uploads when the server resets the stream mid-transfer.
// See https://github.com/rclone/rclone/issues/4673, https://github.com/golang/go/issues/42777
func newHTTP1Client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			ForceAttemptHTTP2: false,
			TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
		},
	}
}
