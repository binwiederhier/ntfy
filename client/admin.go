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
	"strings"
	"time"
)

// Error codes of the admin API, mirrored from server/errors.go. server/errors_test.go asserts they match.
const (
	AdminErrCodeNotFound               = 40401
	AdminErrCodeUnauthorized           = 40101
	AdminErrCodeForbidden              = 40301
	AdminErrCodeAdminSocketOnly        = 40302
	AdminErrCodeUserNotFound           = 40031
	AdminErrCodeUserExists             = 40901
	AdminErrCodeTierNotFound           = 40030
	AdminErrCodeTierExists             = 40909
	AdminErrCodeTierInUse              = 40910
	AdminErrCodeTierCodeInvalid        = 40062
	AdminErrCodeTierReservationsLimit  = 40063
	AdminErrCodeAccessAdminUser        = 40064
	AdminErrCodeTokenNotFound          = 40065
	AdminErrCodeRoleInvalid            = 40066
	AdminErrCodeUserNoPrimaryEmail     = 40067
	AdminErrCodeBaseURLNotConfigured   = 40068
	AdminErrCodePasswordHashInvalid    = 40069
	AdminErrCodePermissionInvalid      = 40025
	AdminErrCodeProvisionedUserChange  = 40905
	AdminErrCodeProvisionedTokenChange = 40906
)

const (
	// AdminTierReset is the tier value that removes a user's tier (Admin.ChangeTier)
	AdminTierReset = "-"

	adminSocketBaseURL    = "http://ntfy" // Host is ignored, the socket transport dials the socket
	adminTimeout          = 10 * time.Second
	adminMaxResponseBytes = 32 * 1024 * 1024 // A large user list is big, but not unbounded
	adminPathVersion      = "/v1/version"
	adminPathUsers        = "/v1/users"
	adminPathAccess       = "/v1/users/access"
	adminPathTokens       = "/v1/users/tokens"
	adminPathReset        = "/v1/users/password/reset"
	adminPathTiers        = "/v1/admin/tiers"
)

var (
	// ErrAdminUnreachable is returned (wrapped with the cause) when the admin API cannot be reached at all
	ErrAdminUnreachable = errors.New("admin API unreachable")
)

// AdminError is a ntfy API error (HTTP status + ntfy error code + message), see server/errors.go
type AdminError struct {
	HTTPCode int    `json:"http"`
	Code     int    `json:"code"`
	Message  string `json:"error"`
	Link     string `json:"link,omitempty"`
}

// Error returns the message and the ntfy error code
func (e *AdminError) Error() string {
	return fmt.Sprintf("%s (error %d)", e.Message, e.Code)
}

// AdminUser is a user, as returned by the admin API
type AdminUser struct {
	Username      string        `json:"username"`
	Role          string        `json:"role"`
	Tier          string        `json:"tier,omitempty"`
	TierName      string        `json:"tier_name,omitempty"`
	Provisioned   bool          `json:"provisioned,omitempty"`
	DefaultAccess string        `json:"default_access,omitempty"` // Only on the "*" (everyone) user
	Grants        []*AdminGrant `json:"grants,omitempty"`
}

// AdminGrant is an access control entry of a user
type AdminGrant struct {
	Topic       string `json:"topic"` // May be a pattern
	Permission  string `json:"permission"`
	Provisioned bool   `json:"provisioned,omitempty"`
}

// AdminToken is an access token of a user. Times are Unix seconds; Expires is 0 if it never expires.
type AdminToken struct {
	Username    string `json:"username"`
	Token       string `json:"token"`
	Label       string `json:"label,omitempty"`
	Expires     int64  `json:"expires"`
	LastAccess  int64  `json:"last_access"`
	LastOrigin  string `json:"last_origin"`
	Provisioned bool   `json:"provisioned,omitempty"`
}

// AdminTier is a tier. Durations are in seconds, sizes in bytes.
type AdminTier struct {
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

// AdminPasswordReset is the result of a password reset: the link, and where it was emailed to (if at all)
type AdminPasswordReset struct {
	Link  string `json:"link"`
	Email string `json:"email,omitempty"`
}

// Admin talks to the ntfy admin API, either over HTTP(S) with an admin token (NewAdmin) or over
// the server's local admin socket (NewAdminSocket). Every method returns *AdminError for API
// errors and a wrapped ErrAdminUnreachable for transport errors.
type Admin struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewAdmin creates an admin API client for a server's HTTP(S) API, authenticating with an admin's access token
func NewAdmin(baseURL, token string) *Admin {
	return &Admin{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: adminTimeout},
	}
}

// NewAdminSocket creates an admin API client for the server's local admin socket (no credentials needed)
func NewAdminSocket(socketPath string) *Admin {
	return &Admin{
		baseURL: adminSocketBaseURL,
		httpClient: &http.Client{
			Timeout: adminTimeout,
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
func (a *Admin) Ping() error {
	return a.do(http.MethodGet, adminPathVersion, nil, nil)
}

// Users returns all users, including the everyone user ("*")
func (a *Admin) Users() ([]*AdminUser, error) {
	var users []*AdminUser
	if err := a.do(http.MethodGet, adminPathUsers, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// User returns a single user, or an AdminError with AdminErrCodeUserNotFound
func (a *Admin) User(username string) (*AdminUser, error) {
	users, err := a.Users()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, &AdminError{
		HTTPCode: http.StatusBadRequest,
		Code:     AdminErrCodeUserNotFound,
		Message:  "invalid request: user does not exist",
	}
}

// AddUser adds a user with a bcrypt password hash. Tier and role may be empty; the role can
// only be set over the admin socket.
func (a *Admin) AddUser(username, hash, tier, role string) error {
	return a.do(http.MethodPost, adminPathUsers, map[string]string{
		"username": username,
		"hash":     hash,
		"tier":     tier,
		"role":     role,
	}, nil)
}

// ChangePassword sets a user's password to the given bcrypt hash
func (a *Admin) ChangePassword(username, hash string) error {
	return a.do(http.MethodPut, adminPathUsers, map[string]string{
		"username": username,
		"hash":     hash,
	}, nil)
}

// ChangeTier changes a user's tier; AdminTierReset removes it
func (a *Admin) ChangeTier(username, tier string) error {
	return a.do(http.MethodPut, adminPathUsers, map[string]string{
		"username": username,
		"tier":     tier,
	}, nil)
}

// ChangeRole changes a user's role (only over the admin socket)
func (a *Admin) ChangeRole(username, role string) error {
	return a.do(http.MethodPut, adminPathUsers, map[string]string{
		"username": username,
		"role":     role,
	}, nil)
}

// RemoveUser removes a user
func (a *Admin) RemoveUser(username string) error {
	return a.do(http.MethodDelete, adminPathUsers, map[string]string{
		"username": username,
	}, nil)
}

// ResetPassword creates a password reset link for a user, and optionally emails it to the user's primary email
func (a *Admin) ResetPassword(username string, sendEmail bool) (*AdminPasswordReset, error) {
	var reset AdminPasswordReset
	req := map[string]any{
		"username":   username,
		"send_email": sendEmail,
	}
	if err := a.do(http.MethodPost, adminPathReset, req, &reset); err != nil {
		return nil, err
	}
	return &reset, nil
}

// AllowAccess grants a user (or "*") a permission on a topic or topic pattern
func (a *Admin) AllowAccess(username, topic, permission string) error {
	return a.do(http.MethodPut, adminPathAccess, map[string]string{
		"username":   username,
		"topic":      topic,
		"permission": permission,
	}, nil)
}

// ResetAccess removes access control entries: of a user for a topic, of a user ("" topic), or all ("" username)
func (a *Admin) ResetAccess(username, topic string) error {
	return a.do(http.MethodDelete, adminPathAccess, map[string]string{
		"username": username,
		"topic":    topic,
	}, nil)
}

// Tokens returns the tokens of a user, or of all users if username is empty
func (a *Admin) Tokens(username string) ([]*AdminToken, error) {
	path := adminPathTokens
	if username != "" {
		path += "?username=" + url.QueryEscape(username)
	}
	var tokens []*AdminToken
	if err := a.do(http.MethodGet, path, nil, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// AddToken creates a token for a user. A zero expires (or the Unix epoch) means it never expires.
func (a *Admin) AddToken(username, label string, expires time.Time) (*AdminToken, error) {
	var expiresUnix int64
	if !expires.IsZero() {
		expiresUnix = expires.Unix()
	}
	req := map[string]any{
		"username": username,
		"label":    label,
		"expires":  expiresUnix,
	}
	var token AdminToken
	if err := a.do(http.MethodPost, adminPathTokens, req, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// RemoveToken removes a user's token
func (a *Admin) RemoveToken(username, token string) error {
	return a.do(http.MethodDelete, adminPathTokens, map[string]string{
		"username": username,
		"token":    token,
	}, nil)
}

// Tiers returns all tiers, sorted by code
func (a *Admin) Tiers() ([]*AdminTier, error) {
	var tiers []*AdminTier
	if err := a.do(http.MethodGet, adminPathTiers, nil, &tiers); err != nil {
		return nil, err
	}
	return tiers, nil
}

// Tier returns a single tier, or an AdminError with AdminErrCodeTierNotFound
func (a *Admin) Tier(code string) (*AdminTier, error) {
	tiers, err := a.Tiers()
	if err != nil {
		return nil, err
	}
	for _, t := range tiers {
		if t.Code == code {
			return t, nil
		}
	}
	return nil, &AdminError{
		HTTPCode: http.StatusBadRequest,
		Code:     AdminErrCodeTierNotFound,
		Message:  "invalid request: tier does not exist",
	}
}

// AddTier adds a tier
func (a *Admin) AddTier(t *AdminTier) error {
	return a.do(http.MethodPost, adminPathTiers, t, nil)
}

// UpdateTier updates the tier with the code t.Code
func (a *Admin) UpdateTier(t *AdminTier) error {
	return a.do(http.MethodPut, adminPathTiers, t, nil)
}

// RemoveTier removes a tier that no user is on
func (a *Admin) RemoveTier(code string) error {
	return a.do(http.MethodDelete, adminPathTiers, map[string]string{
		"code": code,
	}, nil)
}

// Close is a no-op; it exists so Admin matches the CLI's admin interface
func (a *Admin) Close() error {
	return nil // Nothing to close, connections are per request
}

// do sends a JSON request and decodes the JSON response into out (if not nil)
func (a *Admin) do(method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAdminUnreachable, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, adminMaxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: %w", ErrAdminUnreachable, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var apiErr AdminError
		if err := json.Unmarshal(respBody, &apiErr); err != nil || apiErr.Code == 0 {
			return &AdminError{
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
