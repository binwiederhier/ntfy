package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
)

func TestServer_EnsureUser_AnonymousNotAuthorizedAsConcurrentUser(t *testing.T) {
	// The attack from the auth-race report: an anonymous request that overlaps with an
	// authenticated request from the same IP must not see that request's user, or it could
	// mint a token for the victim's account (POST /v1/account/token is behind ensureUser).
	c := newTestConfigWithAuthFile(t, "")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")

	anon := s.visitor(ip, nil)
	s.visitor(ip, u) // The concurrent authenticated request, made deterministic

	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		nextCalled = true
		return nil
	}
	r, _ := http.NewRequest("POST", "/v1/account/token", nil)
	err = s.ensureUser(next)(httptest.NewRecorder(), r, anon)
	require.Equal(t, errHTTPUnauthorized, err)
	require.False(t, nextCalled)
}

func TestServer_AuthorizeTopic_UserSurvivesConcurrentAnonymousRequest(t *testing.T) {
	// The reverse direction of the auth race: a legitimate authenticated request must not be
	// authorized as nobody (and rejected under deny-all) because an anonymous request from
	// the same IP overlapped with it.
	c := newTestConfigWithAuthFile(t, "")
	c.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, c)
	require.Nil(t, s.userManager.AddUser("phil", "phil12345", user.RoleUser, false))
	require.Nil(t, s.userManager.AllowAccess("phil", "mytopic", user.PermissionReadWrite))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	ip := netip.MustParseAddr("9.9.9.9")

	authed := s.visitor(ip, u)
	s.visitor(ip, nil) // The concurrent anonymous request, made deterministic

	nextCalled := false
	next := func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		nextCalled = true
		return nil
	}
	r, _ := http.NewRequest("PUT", "/mytopic", nil)
	require.Nil(t, s.authorizeTopicWrite(next)(httptest.NewRecorder(), r, authed))
	require.True(t, nextCalled)
}
