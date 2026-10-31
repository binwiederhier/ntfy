//go:build !noserver

package cmd

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Body   map[string]any
}

func TestServerClient_RequestsAndResponses(t *testing.T) {
	var requests []*recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Auth: r.Header.Get("Authorization")}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			require.Nil(t, json.Unmarshal(b, &rec.Body))
		}
		requests = append(requests, rec)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/users":
			w.Write([]byte(`[{"username":"ben","role":"user","tier":"pro","tier_name":"Pro","grants":[{"topic":"t*","permission":"read-only","provisioned":true}]},{"username":"*","role":"anonymous","default_access":"deny-all"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/users/tokens":
			w.Write([]byte(`[{"username":"ben","token":"tk_abc","expires":0,"last_access":1700000000,"last_origin":"1.2.3.4"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/users/tokens":
			w.Write([]byte(`{"username":"ben","token":"tk_new","expires":0,"last_access":1700000000,"last_origin":"0.0.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/users/password/reset":
			w.Write([]byte(`{"link":"https://ntfy.example.com/account/password/reset/abc","email":"ben@example.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tiers":
			w.Write([]byte(`[{"id":"ti_1","code":"pro","name":"Pro","messages_limit":10,"users":2}]`))
		default:
			w.Write([]byte(`{"success":true}`))
		}
	}))
	defer server.Close()
	sc := newServerClient(server.URL+"/", "tk_admin")

	// Users
	users, err := sc.Users()
	require.Nil(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "Pro", users[0].TierName)
	require.True(t, users[0].Grants[0].Provisioned)
	require.Equal(t, "deny-all", users[1].DefaultAccess)
	u, err := sc.User("ben")
	require.Nil(t, err)
	require.Equal(t, "pro", u.Tier)
	require.Equal(t, "username=ben", requests[len(requests)-1].Query) // Servers before v2.36.0 ignore it and return all users
	_, err = sc.User("nobody")
	var apiErr *apiError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, apiErrCodeUserNotFound, apiErr.Code)

	// Mutations send the right method, path and body, with the token
	requests = nil
	require.Nil(t, sc.AddUser("ben", "$2a$10$hash", "pro", "admin"))
	require.Nil(t, sc.ChangePassword("ben", "$2a$10$other"))
	require.Nil(t, sc.ChangeTier("ben", tierReset))
	require.Nil(t, sc.ChangeRole("ben", "user"))
	require.Nil(t, sc.RemoveUser("ben"))
	require.Nil(t, sc.AllowAccess("*", "up*", "write-only"))
	require.Nil(t, sc.ResetAccess("", ""))
	require.Nil(t, sc.RemoveToken("ben", "tk_abc"))
	require.Nil(t, sc.RemoveTier("pro"))
	require.Equal(t, []*recordedRequest{
		{Method: "POST", Path: "/v1/users", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben", "hash": "$2a$10$hash", "tier": "pro", "role": "admin"}},
		{Method: "PUT", Path: "/v1/users", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben", "hash": "$2a$10$other"}},
		{Method: "PUT", Path: "/v1/users", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben", "tier": "-"}},
		{Method: "PUT", Path: "/v1/users", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben", "role": "user"}},
		{Method: "DELETE", Path: "/v1/users", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben"}},
		{Method: "PUT", Path: "/v1/users/access", Auth: "Bearer tk_admin", Body: map[string]any{"username": "*", "topic": "up*", "permission": "write-only"}},
		{Method: "DELETE", Path: "/v1/users/access", Auth: "Bearer tk_admin", Body: map[string]any{"username": "", "topic": ""}},
		{Method: "DELETE", Path: "/v1/users/tokens", Auth: "Bearer tk_admin", Body: map[string]any{"username": "ben", "token": "tk_abc"}},
		{Method: "DELETE", Path: "/v1/tiers", Auth: "Bearer tk_admin", Body: map[string]any{"code": "pro"}},
	}, requests)

	// Tokens
	requests = nil
	tokens, err := sc.Tokens("ben")
	require.Nil(t, err)
	require.Equal(t, "username=ben", requests[0].Query)
	require.Equal(t, "1.2.3.4", tokens[0].LastOrigin)
	token, err := sc.AddToken("ben", "label", time.Unix(1900000000, 0))
	require.Nil(t, err)
	require.Equal(t, "tk_new", token.Token)
	require.Equal(t, float64(1900000000), requests[1].Body["expires"])
	_, err = sc.AddToken("ben", "", time.Time{})
	require.Nil(t, err)
	require.Equal(t, float64(0), requests[2].Body["expires"])

	// Password reset
	reset, err := sc.ResetPassword("ben", true)
	require.Nil(t, err)
	require.Equal(t, "ben@example.com", reset.Email)
	require.Equal(t, true, requests[3].Body["send_email"])

	// Tiers
	tier, err := sc.Tier("pro")
	require.Nil(t, err)
	require.Equal(t, "ti_1", tier.ID)
	require.Equal(t, int64(2), tier.Users)
	_, err = sc.Tier("nope")
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, apiErrCodeTierNotFound, apiErr.Code)
}

func TestServerClient_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/users" {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"code":40901,"http":409,"error":"conflict: user already exists"}` + "\n"))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>bad gateway</html>`))
	}))
	sc := newServerClient(server.URL, "")

	// ntfy error
	err := sc.AddUser("ben", "hash", "", "")
	var apiErr *apiError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 409, apiErr.HTTPCode)
	require.Equal(t, apiErrCodeUserExists, apiErr.Code)
	require.Equal(t, "conflict: user already exists (error 40901)", apiErr.Error())

	// Not a ntfy error (e.g. a proxy in between)
	err = sc.Ping()
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 502, apiErr.HTTPCode)
	require.Equal(t, 0, apiErr.Code)

	// Unreachable
	server.Close()
	err = sc.Ping()
	require.ErrorIs(t, err, errServerUnreachable)
}

func TestServerClient_Socket(t *testing.T) {
	dir, err := os.MkdirTemp("", "ntfy-client")
	require.Nil(t, err)
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "admin.sock")

	// Missing socket is unreachable
	require.ErrorIs(t, newServerSocketClient(socket).Ping(), errServerUnreachable)

	// Requests over the socket carry no Authorization header
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	var auth string
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Write([]byte(`{"version":"1.2.3"}`))
	})}
	go server.Serve(listener)
	defer server.Close()
	require.Nil(t, newServerSocketClient(socket).Ping())
	require.Equal(t, "", auth)
}

func TestServerClient_SocketPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can open any socket")
	}
	dir, err := os.MkdirTemp("", "ntfy-client")
	require.Nil(t, err)
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "admin.sock")
	listener, err := net.Listen("unix", socket)
	require.Nil(t, err)
	defer listener.Close()
	require.Nil(t, os.Chmod(socket, 0))

	// The cause stays in the error chain, so callers can tell "no permission" from "no server"
	err = newServerSocketClient(socket).Ping()
	require.ErrorIs(t, err, errServerUnreachable)
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestServerClient_TiersNotAdmin(t *testing.T) {
	// For non-admins, the server answers GET /v1/tiers with the public price list (deprecated);
	// its entries have no ID, which the client must not mistake for tiers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"limits":{"basis":"ip","messages":250}},{"code":"pro","name":"Pro","prices":{"month":500,"year":5000},"limits":{"basis":"tier"}}]`))
	}))
	defer server.Close()
	_, err := newServerClient(server.URL, "tk_user").Tiers()
	var apiErr *apiError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, apiErrCodeUnauthorized, apiErr.Code)
}

func TestServerClient_NoRedirects(t *testing.T) {
	// A redirect could resend the token over plain HTTP, and turn a DELETE into a GET that succeeds
	var deletes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes++
			http.Redirect(w, r, "/v1/users", http.StatusMovedPermanently)
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer server.Close()
	err := newServerClient(server.URL, "tk_admin").RemoveUser("ben")
	require.Error(t, err)
	require.Equal(t, 1, deletes)
	var apiErr *apiError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusMovedPermanently, apiErr.HTTPCode)
}

func TestServerClient_CloseClosesIdleConnections(t *testing.T) {
	var conns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":"1.0.0"}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			conns.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	sc := newServerClient(server.URL, "tk_admin")
	require.Nil(t, sc.Ping())
	require.Nil(t, sc.Close())
	require.Eventually(t, func() bool { return conns.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
}

func TestServerClient_ErrorCodesMatchServer(t *testing.T) {
	// The server's errHTTP values are unexported, and server cannot import cmd, so read them from the source
	codes := map[string]int{
		"errHTTPNotFound":                        apiErrCodeNotFound,
		"errHTTPUnauthorized":                    apiErrCodeUnauthorized,
		"errHTTPForbidden":                       apiErrCodeForbidden,
		"errHTTPForbiddenAdminSocketOnly":        apiErrCodeAdminSocketOnly,
		"errHTTPBadRequestUserNotFound":          apiErrCodeUserNotFound,
		"errHTTPConflictUserExists":              apiErrCodeUserExists,
		"errHTTPBadRequestTierInvalid":           apiErrCodeTierNotFound,
		"errHTTPConflictTierExists":              apiErrCodeTierExists,
		"errHTTPConflictTierInUse":               apiErrCodeTierInUse,
		"errHTTPBadRequestTierCodeInvalid":       apiErrCodeTierCodeInvalid,
		"errHTTPBadRequestTierReservationsLimit": apiErrCodeTierReservationsLimit,
		"errHTTPBadRequestAccessAdminUser":       apiErrCodeAccessAdminUser,
		"errHTTPBadRequestTokenNotFound":         apiErrCodeTokenNotFound,
		"errHTTPBadRequestRoleInvalid":           apiErrCodeRoleInvalid,
		"errHTTPBadRequestUserNoPrimaryEmail":    apiErrCodeUserNoPrimaryEmail,
		"errHTTPBadRequestBaseURLNotConfigured":  apiErrCodeBaseURLNotConfigured,
		"errHTTPBadRequestPasswordHashInvalid":   apiErrCodePasswordHashInvalid,
		"errHTTPBadRequestPermissionInvalid":     apiErrCodePermissionInvalid,
		"errHTTPConflictProvisionedUserChange":   apiErrCodeProvisionedUserChange,
		"errHTTPConflictProvisionedTokenChange":  apiErrCodeProvisionedTokenChange,
	}
	serverCodes := readServerErrorCodes(t, "../server/errors.go")
	for name, code := range codes {
		require.Contains(t, serverCodes, name)
		require.Equal(t, serverCodes[name], code, name)
	}
}

// readServerErrorCodes returns the ntfy error code of every "errHTTP... = &errHTTP{code, ...}" in the file
func readServerErrorCodes(t *testing.T, filename string) map[string]int {
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	require.Nil(t, err)
	codes := make(map[string]int)
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		unary, ok := spec.Values[0].(*ast.UnaryExpr)
		if !ok {
			return true
		}
		lit, ok := unary.X.(*ast.CompositeLit)
		if !ok || len(lit.Elts) == 0 {
			return true
		}
		if code, ok := lit.Elts[0].(*ast.BasicLit); ok {
			value, err := strconv.Atoi(code.Value)
			require.Nil(t, err)
			codes[spec.Names[0].Name] = value
		}
		return true
	})
	return codes
}
