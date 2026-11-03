//go:build !noserver

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"heckel.io/ntfy/v2/cluster"
)

// Error codes of the admin API, mirrored from server/errors.go
const (
	apiErrCodeClusterNotEnabled = 40070
)

const (
	socketBaseURL        = "http://ntfy" // Host is ignored, the socket transport dials the socket
	requestTimeout       = 15 * time.Second
	maxResponseBytes     = 32 * 1024 * 1024
	apiPathClusterStatus = "/v1/cluster/status"
)

var (
	// errSocketUnreachable is returned (wrapped with the cause) when the admin API cannot be reached at all
	errSocketUnreachable = errors.New("admin API unreachable")
)

// apiError is a ntfy API error (HTTP status + ntfy error code + message), see server/errors.go
type apiError struct {
	HTTPCode int    `json:"http"`
	Code     int    `json:"code"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
}

// Error returns the message and the ntfy error code
func (e *apiError) Error() string {
	return fmt.Sprintf("%s (error %d)", e.Message, e.Code)
}

// socketClient talks to the ntfy admin API over the server's local admin socket. Every method returns
// *apiError for API errors and a wrapped errSocketUnreachable for transport errors.
type socketClient struct {
	httpClient *http.Client
}

// newSocketClient creates an admin API client for the server's local admin socket (no credentials needed)
func newSocketClient(socketPath string) *socketClient {
	return &socketClient{
		httpClient: &http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
}

// ClusterStatus returns the server's view of the cluster, or an apiError with apiErrCodeClusterNotEnabled
func (c *socketClient) ClusterStatus() (*cluster.Status, error) {
	var status cluster.Status
	if err := c.do(http.MethodGet, apiPathClusterStatus, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// ClusterNodeStatuses returns every live node's view of the cluster, fetched by the server from its peers
func (c *socketClient) ClusterNodeStatuses() ([]*cluster.NodeStatus, error) {
	var nodes []*cluster.NodeStatus
	if err := c.do(http.MethodGet, apiPathClusterStatus+"?all=1", nil, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (c *socketClient) do(method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, socketBaseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", errSocketUnreachable, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", errSocketUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr apiError
		if err := json.Unmarshal(respBody, &apiErr); err != nil || apiErr.Code == 0 {
			return &apiError{
				HTTPCode: resp.StatusCode,
				Message:  fmt.Sprintf("unexpected response %s", resp.Status),
			}
		}
		return &apiErr
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
