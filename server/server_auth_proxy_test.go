package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
)

func newProxyAuthTestServer(t *testing.T, autoCreate bool, groupsHeader, adminGroup string) *Server {
	conf := newTestConfigWithAuthFile(t, "")
	conf.BehindProxy = true
	conf.AuthUserHeader = "Remote-User"
	conf.AuthUserAutoCreate = autoCreate
	conf.AuthGroupsHeader = groupsHeader
	conf.AuthAdminGroup = adminGroup
	conf.AuthDefault = user.PermissionDenyAll
	return newTestServer(t, conf)
}

func TestServer_ProxyAuth_AutoCreateUserOnFirstRequest(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "", "")
	require.Nil(t, s.userManager.AllowAccess(user.Everyone, "mytopic", user.PermissionReadWrite))

	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User": "phil",
	})
	require.Equal(t, 200, rr.Code)

	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, user.RoleUser, u.Role)
}

func TestServer_ProxyAuth_NoAutoCreateRejectsUnknownUser(t *testing.T) {
	s := newProxyAuthTestServer(t, false, "", "")

	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User": "phil",
	})
	require.Equal(t, 401, rr.Code)

	_, err := s.userManager.User("phil")
	require.Equal(t, user.ErrUserNotFound, err)
}

func TestServer_ProxyAuth_MissingHeaderFallsThroughToAnonymous(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "", "")
	require.Nil(t, s.userManager.AllowAccess(user.Everyone, "up*", user.PermissionWrite))

	rr := request(t, s, "PUT", "/upAbCdEf123456", "unifiedpush", nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/upAbCdEf123456/json?poll=1", "", nil)
	require.Equal(t, 403, rr.Code)
}

func TestServer_ProxyAuth_AdminGroupMapsToAdminRole(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "Remote-Groups", "syncloud")

	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User":   "boris",
		"Remote-Groups": "users, syncloud",
	})
	require.Equal(t, 200, rr.Code)

	u, err := s.userManager.User("boris")
	require.Nil(t, err)
	require.Equal(t, user.RoleAdmin, u.Role)
}

func TestServer_ProxyAuth_AdminGroupRequiresExactMatch(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "Remote-Groups", "syncloud")

	request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User":   "eve",
		"Remote-Groups": "syncloud-guests",
	})

	u, err := s.userManager.User("eve")
	require.Nil(t, err)
	require.Equal(t, user.RoleUser, u.Role)
}

func TestServer_ProxyAuth_RoleSyncedOnDemotionAndPromotion(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "Remote-Groups", "syncloud")

	request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User":   "boris",
		"Remote-Groups": "syncloud",
	})
	u, err := s.userManager.User("boris")
	require.Nil(t, err)
	require.Equal(t, user.RoleAdmin, u.Role)

	request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User":   "boris",
		"Remote-Groups": "users",
	})
	u, err = s.userManager.User("boris")
	require.Nil(t, err)
	require.Equal(t, user.RoleUser, u.Role)

	request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User":   "boris",
		"Remote-Groups": "syncloud",
	})
	u, err = s.userManager.User("boris")
	require.Nil(t, err)
	require.Equal(t, user.RoleAdmin, u.Role)
}

func TestServer_ProxyAuth_RoleNotSyncedWithoutGroupsHeader(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "", "")
	require.Nil(t, s.userManager.AddUser("admin", "adminpass", user.RoleAdmin, false))

	request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User": "admin",
	})

	u, err := s.userManager.User("admin")
	require.Nil(t, err)
	require.Equal(t, user.RoleAdmin, u.Role)
}

func TestServer_ProxyAuth_InvalidUsernameRejected(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "", "")

	for _, username := range []string{"*", "phil bob", "phil/../root", "<script>"} {
		rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
			"Remote-User": username,
		})
		require.Equal(t, 401, rr.Code, "expected 401 for username %q", username)
	}
}

func TestServer_ProxyAuth_IgnoredWhenNotBehindProxy(t *testing.T) {
	conf := newTestConfigWithAuthFile(t, "")
	conf.BehindProxy = false
	conf.AuthUserHeader = "Remote-User"
	conf.AuthUserAutoCreate = true
	conf.AuthDefault = user.PermissionDenyAll
	s := newTestServer(t, conf)

	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User": "phil",
	})
	require.Equal(t, 403, rr.Code)

	_, err := s.userManager.User("phil")
	require.Equal(t, user.ErrUserNotFound, err)
}

func TestServer_ProxyAuth_DeletedUserRejected(t *testing.T) {
	s := newProxyAuthTestServer(t, true, "", "")
	require.Nil(t, s.userManager.AddUser("phil", "philpass", user.RoleUser, false))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, s.userManager.MarkUserRemoved(u))

	rr := request(t, s, "GET", "/mytopic/json?poll=1", "", map[string]string{
		"Remote-User": "phil",
	})
	require.Equal(t, 401, rr.Code)
}
