package s3

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"heckel.io/ntfy/v2/log"
)

// AbortIncompleteUploads lists all in-progress multipart uploads and aborts those initiated
// before the given cutoff time. This cleans up orphaned upload parts from interrupted uploads.
//
// See https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListMultipartUploads.html
// and https://docs.aws.amazon.com/AmazonS3/latest/API/API_AbortMultipartUpload.html
func (c *Client) AbortIncompleteUploads(ctx context.Context, cutoff time.Time) error {
	uploads, err := c.listMultipartUploads(ctx)
	if err != nil {
		return err
	}
	for _, u := range uploads {
		if !u.Initiated.IsZero() && u.Initiated.Before(cutoff) {
			c.abortMultipartUpload(ctx, u.Key, u.UploadID)
		}
	}
	return nil
}

// listMultipartUploads returns in-progress multipart uploads for the client's prefix.
// It paginates automatically, stopping after 10,000 pages as a safety valve.
func (c *Client) listMultipartUploads(ctx context.Context) ([]*multipartUpload, error) {
	var all []*multipartUpload
	var keyMarker, uploadIDMarker string
	for page := 0; page < maxPages; page++ {
		query := url.Values{"uploads": {""}}
		if prefix := c.config.ListPrefix(); prefix != "" {
			query.Set("prefix", prefix)
		}
		if keyMarker != "" {
			query.Set("key-marker", keyMarker)
			query.Set("upload-id-marker", uploadIDMarker)
		}
		respBody, err := c.do(ctx, "ListMultipartUploads", http.MethodGet, c.config.BucketURL()+"?"+query.Encode(), nil, nil)
		if err != nil {
			return nil, err
		}
		var result listMultipartUploadsResult
		if err := xml.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("error unmarshalling multipart upload result: %w", err)
		}
		for _, u := range result.Uploads {
			var initiated time.Time
			if u.Initiated != "" {
				initiated, _ = time.Parse(time.RFC3339, u.Initiated)
			}
			all = append(all, &multipartUpload{
				Key:       u.Key,
				UploadID:  u.UploadID,
				Initiated: initiated,
			})
		}
		if !result.IsTruncated {
			return all, nil
		}
		keyMarker = result.NextKeyMarker
		uploadIDMarker = result.NextUploadIDMarker
	}
	return nil, fmt.Errorf("error listing multipart uploads, exceeded %d pages", maxPages)
}

// abortMultipartUpload cancels an in-progress multipart upload. Called on error to clean up.
func (c *Client) abortMultipartUpload(ctx context.Context, key, uploadID string) {
	log.Tag(tagS3Client).Info("Aborting multipart upload for object %s", key)
	reqURL := fmt.Sprintf("%s?uploadId=%s", c.config.ObjectURL(key), url.QueryEscape(uploadID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return
	}
	c.signV4(req, emptyPayloadHash)
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// putObjectMultipart uploads body using S3 multipart upload. It reads the body in partSize
// chunks, uploading each as a separate part. This allows uploading without knowing the total
// body size in advance.
func (c *Client) putObjectMultipart(ctx context.Context, key string, body io.Reader) error {
	log.Tag(tagS3Client).Debug("Uploading multipart object %s", key)

	// Step 1: Initiate multipart upload
	uploadID, err := c.initiateMultipartUpload(ctx, key)
	if err != nil {
		return err
	}

	// Step 2: Upload parts
	partNumber := 1
	buf := make([]byte, partSize)
	var parts []*completedPart
	for {
		n, err := io.ReadFull(body, buf)
		if n > 0 {
			etag, uploadErr := c.uploadPart(ctx, key, uploadID, partNumber, buf[:n])
			if uploadErr != nil {
				c.abortMultipartUpload(ctx, key, uploadID)
				return uploadErr
			}
			parts = append(parts, &completedPart{
				PartNumber: partNumber,
				ETag:       etag,
			})
			partNumber++
		}
		if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		} else if err != nil {
			c.abortMultipartUpload(ctx, key, uploadID)
			return fmt.Errorf("error uploading object %s, reading from client failed: %w", key, err)
		}
	}

	// Step 3: Complete multipart upload
	return c.completeMultipartUpload(ctx, key, uploadID, parts)
}

// initiateMultipartUpload starts a new multipart upload and returns the upload ID.
func (c *Client) initiateMultipartUpload(ctx context.Context, key string) (string, error) {
	respBody, err := c.do(ctx, "InitiateMultipartUpload", http.MethodPost, c.config.ObjectURL(key)+"?uploads", nil, nil)
	if err != nil {
		return "", err
	}
	var result initiateMultipartUploadResult
	if err := xml.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("error unmarshalling initiate multipart upload response: %w", err)
	}
	return result.UploadID, nil
}

// uploadPart uploads a single part of a multipart upload and returns the ETag.
func (c *Client) uploadPart(ctx context.Context, key, uploadID string, partNumber int, data []byte) (string, error) {
	log.Tag(tagS3Client).Debug("Uploading multipart part for object %s, part %d, size %d", key, partNumber, len(data))
	reqURL := fmt.Sprintf("%s?partNumber=%d&uploadId=%s", c.config.ObjectURL(key), partNumber, url.QueryEscape(uploadID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("error creating multipart upload part request for object %s: %w", key, err)
	}
	req.ContentLength = int64(len(data))
	c.signV4(req, unsignedPayload)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("error uploading multipart part for object %s: %w", key, err)
	}
	defer resp.Body.Close()
	if !isHTTPSuccess(resp) {
		return "", parseError(resp)
	}
	return resp.Header.Get("ETag"), nil
}

// completeMultipartUpload finalizes a multipart upload with the given parts.
func (c *Client) completeMultipartUpload(ctx context.Context, key, uploadID string, parts []*completedPart) error {
	log.Tag(tagS3Client).Debug("Completing multipart upload for object %s, %d parts", key, len(parts))
	bodyBytes, err := xml.Marshal(&completeMultipartUploadRequest{Parts: parts})
	if err != nil {
		return fmt.Errorf("error marshalling complete multipart upload request: %w", err)
	}
	reqURL := fmt.Sprintf("%s?uploadId=%s", c.config.ObjectURL(key), url.QueryEscape(uploadID))
	respBody, err := c.do(ctx, "CompleteMultipartUpload", http.MethodPost, reqURL, bodyBytes, nil)
	if err != nil {
		return err
	}
	// Check if the response contains an error (S3 can return 200 with an error body)
	var errResp errorResponse
	if xml.Unmarshal(respBody, &errResp) == nil && errResp.Code != "" {
		return &errResp
	}
	return nil
}
