package client

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestAdmin_RequestsAndResponses(t *testing.T) {
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
	admin := NewAdmin(server.URL+"/", "tk_admin")

	// Users
	users, err := admin.Users()
	require.Nil(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "Pro", users[0].TierName)
	require.True(t, users[0].Grants[0].Provisioned)
	require.Equal(t, "deny-all", users[1].DefaultAccess)
	u, err := admin.User("ben")
	require.Nil(t, err)
	require.Equal(t, "pro", u.Tier)
	_, err = admin.User("nobody")
	var apiErr *AdminError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, AdminErrCodeUserNotFound, apiErr.Code)

	// Mutations send the right method, path and body, with the token
	requests = nil
	require.Nil(t, admin.AddUser("ben", "$2a$10$hash", "pro", "admin"))
	require.Nil(t, admin.ChangePassword("ben", "$2a$10$other"))
	require.Nil(t, admin.ChangeTier("ben", AdminTierReset))
	require.Nil(t, admin.ChangeRole("ben", "user"))
	require.Nil(t, admin.RemoveUser("ben"))
	require.Nil(t, admin.AllowAccess("*", "up*", "write-only"))
	require.Nil(t, admin.ResetAccess("", ""))
	require.Nil(t, admin.RemoveToken("ben", "tk_abc"))
	require.Nil(t, admin.RemoveTier("pro"))
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
	tokens, err := admin.Tokens("ben")
	require.Nil(t, err)
	require.Equal(t, "username=ben", requests[0].Query)
	require.Equal(t, "1.2.3.4", tokens[0].LastOrigin)
	token, err := admin.AddToken("ben", "label", time.Unix(1900000000, 0))
	require.Nil(t, err)
	require.Equal(t, "tk_new", token.Token)
	require.Equal(t, float64(1900000000), requests[1].Body["expires"])
	_, err = admin.AddToken("ben", "", time.Time{})
	require.Nil(t, err)
	require.Equal(t, float64(0), requests[2].Body["expires"])

	// Password reset
	reset, err := admin.ResetPassword("ben", true)
	require.Nil(t, err)
	require.Equal(t, "ben@example.com", reset.Email)
	require.Equal(t, true, requests[3].Body["send_email"])

	// Tiers
	tier, err := admin.Tier("pro")
	require.Nil(t, err)
	require.Equal(t, "ti_1", tier.ID)
	require.Equal(t, int64(2), tier.Users)
	_, err = admin.Tier("nope")
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, AdminErrCodeTierNotFound, apiErr.Code)
}

func TestAdmin_Errors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/users" {
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"code":40901,"http":409,"error":"conflict: user already exists"}` + "\n"))
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`<html>bad gateway</html>`))
	}))
	admin := NewAdmin(server.URL, "")

	// ntfy error
	err := admin.AddUser("ben", "hash", "", "")
	var apiErr *AdminError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 409, apiErr.HTTPCode)
	require.Equal(t, AdminErrCodeUserExists, apiErr.Code)
	require.Equal(t, "conflict: user already exists (error 40901)", apiErr.Error())

	// Not a ntfy error (e.g. a proxy in between)
	err = admin.Ping()
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 502, apiErr.HTTPCode)
	require.Equal(t, 0, apiErr.Code)

	// Unreachable
	server.Close()
	err = admin.Ping()
	require.ErrorIs(t, err, ErrAdminUnreachable)
}

func TestAdmin_Socket(t *testing.T) {
	dir, err := os.MkdirTemp("", "ntfy-client")
	require.Nil(t, err)
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "admin.sock")

	// Missing socket is unreachable
	require.ErrorIs(t, NewAdminSocket(socket).Ping(), ErrAdminUnreachable)

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
	require.Nil(t, NewAdminSocket(socket).Ping())
	require.Equal(t, "", auth)
}

func TestAdmin_SocketPermissionDenied(t *testing.T) {
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
	err = NewAdminSocket(socket).Ping()
	require.ErrorIs(t, err, ErrAdminUnreachable)
	require.ErrorIs(t, err, fs.ErrPermission)
}

func TestAdmin_TiersNotAdmin(t *testing.T) {
	// For non-admins, the server answers GET /v1/tiers with the public price list (deprecated);
	// its entries have no ID, which the client must not mistake for tiers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"limits":{"basis":"ip","messages":250}},{"code":"pro","name":"Pro","prices":{"month":500,"year":5000},"limits":{"basis":"tier"}}]`))
	}))
	defer server.Close()
	_, err := NewAdmin(server.URL, "tk_user").Tiers()
	var apiErr *AdminError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, AdminErrCodeUnauthorized, apiErr.Code)
}
