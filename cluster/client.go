package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	clientTimeout  = 5 * time.Second
	healthMaxBytes = 4 * 1024    // Upper bound for a peer's health response
	statusMaxBytes = 1024 * 1024 // Upper bound for a node's status response
)

// Client speaks the peer API to another node's cluster listener: it builds the requests, sets the
// secret and origin headers, and checks and decodes the answers. What a failure means (a lost
// batch, a retried state, an unhealthy peer) is the caller's decision.
type Client struct {
	httpClient *http.Client
	secret     string
	origin     NodeID // Empty for callers that are not a node (the CLI); sent only on fan-out requests
}

// HTTPError is a peer API answer other than 200
type HTTPError struct {
	URL        string
	StatusCode int
}

// NewClient creates a client authenticating with the shared cluster secret, as the given node
func NewClient(secret string, origin NodeID) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: clientTimeout},
		secret:     secret,
		origin:     origin,
	}
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("request to %s failed with HTTP %d", e.URL, e.StatusCode)
}

// SendMessages posts a batch of NDJSON message lines to a peer's message endpoint
func (c *Client) SendMessages(ctx context.Context, baseURL string, body []byte) error {
	return c.post(ctx, messageURL(baseURL), contentTypeNDJSON, body)
}

// SendState posts a JSON state envelope to a peer's state endpoint
func (c *Client) SendState(ctx context.Context, baseURL string, body []byte) error {
	return c.post(ctx, stateURL(baseURL), contentTypeJSON, body)
}

// Health asks a peer whether it is healthy; any answer other than a 200 with a health body is an
// error, since a proxy, a captive portal or a parked domain answers 200 too
func (c *Client) Health(ctx context.Context, baseURL string) (bool, error) {
	var health apiHealth
	if err := c.get(ctx, healthURL(baseURL), healthMaxBytes, &health); err != nil {
		return false, err
	}
	return health.Healthy, nil
}

// Status reads a node's view of the cluster from its status endpoint (baseURL, e.g. http://10.0.0.5:2587)
func (c *Client) Status(ctx context.Context, baseURL string) (*Status, error) {
	var status Status
	if err := c.get(ctx, statusURL(baseURL), statusMaxBytes, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// post sends a fan-out request: authenticated with the secret, and carrying this node's ID so the
// receiver can skip its own broadcasts
func (c *Client) post(ctx context.Context, url, contentType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set(secretHeader, c.secret)
	if c.origin != "" {
		req.Header.Set(originHeader, string(c.origin))
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{URL: url, StatusCode: resp.StatusCode}
	}
	return nil
}

// get reads a JSON answer from a secret-only endpoint, decoding at most limit bytes into v
func (c *Client) get(ctx context.Context, url string, limit int64, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set(secretHeader, c.secret)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &HTTPError{URL: url, StatusCode: resp.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(v)
}

// messageURL derives the peer's message endpoint URL from its advertise URL.
func messageURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + MessagePath
}

// stateURL derives the peer's state endpoint URL from its advertise URL.
func stateURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + StatePath
}

// statusURL derives a node's status endpoint URL from its cluster base URL.
func statusURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + StatusPath
}

// healthURL derives the peer's health endpoint URL from its advertise URL.
func healthURL(advertiseURL string) string {
	return strings.TrimRight(advertiseURL, "/") + HealthPath
}
