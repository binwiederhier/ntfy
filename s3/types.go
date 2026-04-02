package s3

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds the parsed fields from an S3 URL. Use ParseURL to create one from a URL string.
type Config struct {
	Endpoint     string // host[:port] only, e.g. "s3.us-east-1.amazonaws.com"
	PathStyle    bool
	Bucket       string
	Prefix       string
	Region       string
	AccessKey    string
	SecretKey    string
	DisableHTTP2 bool         // Force HTTP/1.1 to work around HTTP/2 issues with some S3-compatible providers
	HTTPClient   *http.Client // if nil, a default client is created (respecting DisableHTTP2)
}

// BucketURL returns the base URL for bucket-level operations.
func (c *Config) BucketURL() string {
	if c.PathStyle {
		return fmt.Sprintf("https://%s/%s", c.Endpoint, c.Bucket)
	}
	return fmt.Sprintf("https://%s.%s", c.Bucket, c.Endpoint)
}

// HostHeader returns the value for the Host header.
func (c *Config) HostHeader() string {
	if c.PathStyle {
		return c.Endpoint
	}
	return c.Bucket + "." + c.Endpoint
}

// ListPrefix returns the prefix to use in ListObjectsV2 requests,
// with a trailing slash so that only objects under the prefix directory are returned.
func (c *Config) ListPrefix() string {
	if c.Prefix != "" {
		return c.Prefix + "/"
	}
	return ""
}

// StripPrefix removes the configured prefix from a key returned by ListObjectsV2,
// so keys match what was passed to PutObject/GetObject/DeleteObjects.
func (c *Config) StripPrefix(key string) string {
	if c.Prefix != "" {
		return strings.TrimPrefix(key, c.Prefix+"/")
	}
	return key
}

// ObjectKey prepends the configured prefix to the given key.
func (c *Config) ObjectKey(key string) string {
	if c.Prefix != "" {
		return c.Prefix + "/" + key
	}
	return key
}

// ObjectURL returns the full URL for an object, automatically prepending the configured prefix.
func (c *Config) ObjectURL(key string) string {
	u, _ := url.JoinPath(c.BucketURL(), c.ObjectKey(key))
	return u
}

// Object represents an S3 object returned by list operations.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// errorResponse is returned when S3 responds with a non-2xx status code.
type errorResponse struct {
	StatusCode int
	Code       string `xml:"Code"`
	Message    string `xml:"Message"`
	Body       string `xml:"-"` // raw response body
}

func (e *errorResponse) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("s3: %s (HTTP %d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("s3: HTTP %d: %s", e.StatusCode, e.Body)
}

// listObjectsV2Result is the XML response from S3 ListObjectsV2
type listObjectsV2Result struct {
	Contents              []*listObject `xml:"Contents"`
	IsTruncated           bool          `xml:"IsTruncated"`
	NextContinuationToken string        `xml:"NextContinuationToken"`
}

type listObject struct {
	Key          string `xml:"Key"`
	Size         int64  `xml:"Size"`
	LastModified string `xml:"LastModified"`
}

// deleteObjectsRequest is the XML request body for S3 DeleteObjects
type deleteObjectsRequest struct {
	XMLName xml.Name        `xml:"Delete"`
	Quiet   bool            `xml:"Quiet"`
	Objects []*deleteObject `xml:"Object"`
}

type deleteObject struct {
	Key string `xml:"Key"`
}

// deleteObjectsResult is the XML response from S3 DeleteObjects
type deleteObjectsResult struct {
	Errors []*deleteError `xml:"Error"`
}

type deleteError struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// listMultipartUploadsResult is the XML response from S3 listMultipartUploads
type listMultipartUploadsResult struct {
	Uploads            []*listUpload `xml:"Upload"`
	IsTruncated        bool          `xml:"IsTruncated"`
	NextKeyMarker      string        `xml:"NextKeyMarker"`
	NextUploadIDMarker string        `xml:"NextUploadIdMarker"`
}

type listUpload struct {
	Key       string `xml:"Key"`
	UploadID  string `xml:"UploadId"`
	Initiated string `xml:"Initiated"`
}

// multipartUpload represents an in-progress multipart upload returned by listMultipartUploads.
type multipartUpload struct {
	Key       string
	UploadID  string
	Initiated time.Time
}

// initiateMultipartUploadResult is the XML response from S3 InitiateMultipartUpload
type initiateMultipartUploadResult struct {
	UploadID string `xml:"UploadId"`
}

// completeMultipartUploadRequest is the XML request body for S3 CompleteMultipartUpload
type completeMultipartUploadRequest struct {
	XMLName xml.Name         `xml:"CompleteMultipartUpload"`
	Parts   []*completedPart `xml:"Part"`
}

// completedPart represents a successfully uploaded part for CompleteMultipartUpload
type completedPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}
