// Package client is the CLI's client for the ntfy admin API, over the server's local admin socket.
// Not to be confused with heckel.io/ntfy/v2/client, which publishes and subscribes.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Error codes of the admin API, mirrored from server/errors.go. TestClient_ErrorCodesMatchServer asserts they match.
const (
	ErrCodeNotFound               = 40401
	ErrCodeUserNotFound           = 40031
	ErrCodeUserExists             = 40901
	ErrCodeTierNotFound           = 40030
	ErrCodeTierExists             = 40909
	ErrCodeTierInUse              = 40910
	ErrCodeTierCodeInvalid        = 40062
	ErrCodeTierReservationsLimit  = 40063
	ErrCodeAccessAdminUser        = 40064
	ErrCodeTokenNotFound          = 40065
	ErrCodeRoleInvalid            = 40066
	ErrCodeUserNoPrimaryEmail     = 40067
	ErrCodeBaseURLNotConfigured   = 40068
	ErrCodePasswordHashInvalid    = 40069
	ErrCodePermissionInvalid      = 40025
	ErrCodeProvisionedUserChange  = 40905
	ErrCodeProvisionedTokenChange = 40906
)

const (
	// TierReset is the tier value that removes a user's tier (ChangeTier)
	TierReset = "-"
)

const (
	socketBaseURL    = "http://ntfy" // Host is ignored, the socket transport dials the socket
	requestTimeout   = 10 * time.Second
	maxResponseBytes = 32 * 1024 * 1024 // A large user list is big, but not unbounded
	apiPathVersion   = "/v1/version"
	apiPathUsers     = "/v1/users"
	apiPathAccess    = "/v1/users/access"
	apiPathTokens    = "/v1/users/tokens"
	apiPathReset     = "/v1/users/password/reset"
	apiPathTiers     = "/v1/tiers"
)

var (
	// ErrUnreachable is returned (wrapped with the cause) when the admin API cannot be reached at all
	ErrUnreachable = errors.New("admin API unreachable")
)

// Error is a ntfy API error (HTTP status + ntfy error code + message), see server/errors.go
type Error struct {
	HTTPCode int    `json:"http"`
	Code     int    `json:"code"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
}

// Error returns the message and the ntfy error code
func (e *Error) Error() string {
	return fmt.Sprintf("%s (error %d)", e.Message, e.Code)
}

// User is a user, as returned by the admin API
type User struct {
	Username      string   `json:"username"`
	Role          string   `json:"role"`
	Tier          string   `json:"tier,omitempty"`
	TierName      string   `json:"tier_name,omitempty"`
	Provisioned   bool     `json:"provisioned,omitempty"`
	DefaultAccess string   `json:"default_access,omitempty"` // Only on the "*" (everyone) user
	Grants        []*Grant `json:"grants,omitempty"`
}

// Grant is an access control entry of a user
type Grant struct {
	Topic       string `json:"topic"` // May be a pattern
	Permission  string `json:"permission"`
	Provisioned bool   `json:"provisioned,omitempty"`
}

// Token is an access token of a user. Times are Unix seconds; Expires is 0 if it never expires.
type Token struct {
	Username    string `json:"username"`
	Token       string `json:"token"`
	Label       string `json:"label,omitempty"`
	Expires     int64  `json:"expires"`
	LastAccess  int64  `json:"last_access,omitempty"` // Unix seconds; omitted if never used
	LastOrigin  string `json:"last_origin,omitempty"` // IP; omitted if never used
	Provisioned bool   `json:"provisioned,omitempty"`
}

// Tier is a tier. Durations are in seconds, sizes in bytes.
type Tier struct {
	ID                       string `json:"id,omitempty"` // Response only
	Code                     string `json:"code"`
	Name                     string `json:"name"`
	MessagesLimit            int64  `json:"messages_limit"`
	MessagesExpiryDuration   int64  `json:"messages_expiry_duration"`
	EmailsLimit              int64  `json:"emails_limit"`
	CallsLimit               int64  `json:"calls_limit"`
	ReservationsLimit        int64  `json:"reservations_limit"`
	AttachmentFileSizeLimit  int64  `json:"attachment_file_size_limit"`
	AttachmentTotalSizeLimit int64  `json:"attachment_total_size_limit"`
	AttachmentExpiryDuration int64  `json:"attachment_expiry_duration"`
	AttachmentBandwidthLimit int64  `json:"attachment_bandwidth_limit"`
	StripeMonthlyPriceID     string `json:"stripe_monthly_price_id,omitempty"`
	StripeYearlyPriceID      string `json:"stripe_yearly_price_id,omitempty"`
	Users                    int64  `json:"users"` // Response only: number of users on this tier
}

// PasswordReset is the result of a password reset: the link, and where it was emailed to (if at all)
type PasswordReset struct {
	Link  string `json:"link"`
	Email string `json:"email,omitempty"`
}

// Client talks to the ntfy admin API over the server's local admin socket. Every method returns
// *Error for API errors and a wrapped ErrUnreachable for transport errors.
type Client struct {
	httpClient *http.Client
}

// New creates an admin API client for the server's local admin socket (no credentials needed)
func New(socketPath string) *Client {
	return &Client{
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

// Ping checks that the admin API answers (GET /v1/version)
func (c *Client) Ping() error {
	return c.do(http.MethodGet, apiPathVersion, nil, nil)
}

// Users returns all users, including the everyone user ("*")
func (c *Client) Users() ([]*User, error) {
	var users []*User
	if err := c.do(http.MethodGet, apiPathUsers, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// User returns a single user, or an Error with ErrCodeUserNotFound
func (c *Client) User(username string) (*User, error) {
	var users []*User
	if err := c.do(http.MethodGet, apiPathUsers+"?username="+url.QueryEscape(username), nil, &users); err != nil {
		return nil, err
	}
	for _, u := range users { // Servers before v2.36.0 ignore the username and return all users
		if u.Username == username {
			return u, nil
		}
	}
	return nil, &Error{
		HTTPCode: http.StatusBadRequest,
		Code:     ErrCodeUserNotFound,
		Message:  "invalid request: user does not exist",
	}
}

// AddUser adds a user with a bcrypt password hash. Tier and role may be empty; the role can
// only be set over the admin socket.
func (c *Client) AddUser(username, hash, tier, role string) error {
	return c.do(http.MethodPost, apiPathUsers, map[string]string{
		"username": username,
		"hash":     hash,
		"tier":     tier,
		"role":     role,
	}, nil)
}

// ChangePassword sets a user's password to the given bcrypt hash
func (c *Client) ChangePassword(username, hash string) error {
	return c.do(http.MethodPut, apiPathUsers, map[string]string{
		"username": username,
		"hash":     hash,
	}, nil)
}

// ChangeTier changes a user's tier; TierReset removes it
func (c *Client) ChangeTier(username, tier string) error {
	return c.do(http.MethodPut, apiPathUsers, map[string]string{
		"username": username,
		"tier":     tier,
	}, nil)
}

// ChangeRole changes a user's role (only over the admin socket)
func (c *Client) ChangeRole(username, role string) error {
	return c.do(http.MethodPut, apiPathUsers, map[string]string{
		"username": username,
		"role":     role,
	}, nil)
}

// RemoveUser removes a user
func (c *Client) RemoveUser(username string) error {
	return c.do(http.MethodDelete, apiPathUsers, map[string]string{
		"username": username,
	}, nil)
}

// ResetPassword creates a password reset link for a user, and optionally emails it to the user's primary email
func (c *Client) ResetPassword(username string, sendEmail bool) (*PasswordReset, error) {
	var reset PasswordReset
	req := map[string]any{
		"username":   username,
		"send_email": sendEmail,
	}
	if err := c.do(http.MethodPost, apiPathReset, req, &reset); err != nil {
		return nil, err
	}
	return &reset, nil
}

// AllowAccess grants a user (or "*") a permission on a topic or topic pattern
func (c *Client) AllowAccess(username, topic, permission string) error {
	return c.do(http.MethodPut, apiPathAccess, map[string]string{
		"username":   username,
		"topic":      topic,
		"permission": permission,
	}, nil)
}

// ResetAccess removes access control entries: of a user for a topic, of a user ("" topic), or all ("" username)
func (c *Client) ResetAccess(username, topic string) error {
	return c.do(http.MethodDelete, apiPathAccess, map[string]string{
		"username": username,
		"topic":    topic,
	}, nil)
}

// Tokens returns the tokens of a user, or of all users if username is empty
func (c *Client) Tokens(username string) ([]*Token, error) {
	path := apiPathTokens
	if username != "" {
		path += "?username=" + url.QueryEscape(username)
	}
	var tokens []*Token
	if err := c.do(http.MethodGet, path, nil, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// AddToken creates a token for a user. A zero expires (or the Unix epoch) means it never expires.
func (c *Client) AddToken(username, label string, expires time.Time) (*Token, error) {
	var expiresUnix int64
	if !expires.IsZero() {
		expiresUnix = expires.Unix()
	}
	req := map[string]any{
		"username": username,
		"label":    label,
		"expires":  expiresUnix,
	}
	var token Token
	if err := c.do(http.MethodPost, apiPathTokens, req, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// RemoveToken removes a user's token
func (c *Client) RemoveToken(username, token string) error {
	return c.do(http.MethodDelete, apiPathTokens, map[string]string{
		"username": username,
		"token":    token,
	}, nil)
}

// Tiers returns all tiers, sorted by code
func (c *Client) Tiers() ([]*Tier, error) {
	var tiers []*Tier
	if err := c.do(http.MethodGet, apiPathTiers, nil, &tiers); err != nil {
		return nil, err
	}
	return tiers, nil
}

// Tier returns a single tier, or an Error with ErrCodeTierNotFound
func (c *Client) Tier(code string) (*Tier, error) {
	tiers, err := c.Tiers()
	if err != nil {
		return nil, err
	}
	for _, t := range tiers {
		if t.Code == code {
			return t, nil
		}
	}
	return nil, &Error{
		HTTPCode: http.StatusBadRequest,
		Code:     ErrCodeTierNotFound,
		Message:  "invalid request: tier does not exist",
	}
}

// AddTier adds a tier
func (c *Client) AddTier(t *Tier) error {
	return c.do(http.MethodPost, apiPathTiers, t, nil)
}

// UpdateTier updates the tier with the code t.Code
func (c *Client) UpdateTier(t *Tier) error {
	return c.do(http.MethodPut, apiPathTiers, t, nil)
}

// RemoveTier removes a tier that no user is on
func (c *Client) RemoveTier(code string) error {
	return c.do(http.MethodDelete, apiPathTiers, map[string]string{
		"code": code,
	}, nil)
}

// Close closes the client's idle (kept-alive) connections
func (c *Client) Close() error {
	c.httpClient.CloseIdleConnections()
	return nil
}

// do sends a JSON request and decodes the JSON response into out (if not nil)
func (c *Client) do(method, path string, body any, out any) error {
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
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr Error
		if err := json.Unmarshal(respBody, &apiErr); err != nil || apiErr.Code == 0 {
			return &Error{
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
