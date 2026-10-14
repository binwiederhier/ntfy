package server

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func TestAccount_TokenEnroll_SelfRedeem(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))

		enroll := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		require.NotEmpty(t, enroll.Code)
		require.InDelta(t, time.Now().Add(10*time.Minute).Unix(), enroll.Expires, 5)

		// Redeeming needs no credentials; the code is the credential
		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s","label":"Pixel 9"}`, enroll.Code), nil)
		require.Equal(t, 200, rr.Code)
		token, err := util.UnmarshalJSON[apiAccountTokenRedeemResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.True(t, user.ValidToken(token.Token))
		require.Equal(t, "phil", token.Username) // The redeemer has no other way to learn who it is signed in as
		require.Equal(t, "Pixel 9", token.Label)
		require.Equal(t, int64(0), token.Expires) // A device token lives until it is revoked

		rr = request(t, s, "GET", "/v1/account", "", map[string]string{"Authorization": util.BearerAuth(token.Token)})
		require.Equal(t, 200, rr.Code)
		account, err := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, "phil", account.Username)

		// Single use
		rr = request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, enroll.Code), nil)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestAccount_TokenEnroll_DefaultLabel(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))

		enroll := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, enroll.Code), nil)
		require.Equal(t, 200, rr.Code)
		token, err := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, "Enrolled device", token.Label)
	})
}

func TestAccount_TokenEnroll_AnonymousCannotIssue(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()

		rr := request(t, s, "POST", "/v1/account/token/enroll", "", nil)
		require.Equal(t, 401, rr.Code)
	})
}

func TestAccount_TokenEnroll_InvalidCode(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()

		for _, body := range []string{`{"code":"nope"}`, `{"code":""}`, `{}`} {
			rr := request(t, s, "POST", "/v1/account/token/redeem", body, nil)
			require.Equal(t, 400, rr.Code, body)
			require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code, body)
		}
	})
}

func TestAccount_TokenEnroll_Expired(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)

		code, err := s.userManager.AddMagicLink(user.MagicLinkKindTokenEnroll, u.ID, "", -time.Second)
		require.Nil(t, err)
		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, code), nil)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestAccount_TokenEnroll_ReissueSupersedes(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))

		first := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		second := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		require.NotEqual(t, first.Code, second.Code)

		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, first.Code), nil)
		require.Equal(t, 400, rr.Code)
		rr = request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, second.Code), nil)
		require.Equal(t, 200, rr.Code)
	})
}

func TestAccount_TokenEnroll_NotInterchangeableWithResetLinks(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))
		u, err := s.userManager.User("phil")
		require.Nil(t, err)

		resetToken, err := s.userManager.AddMagicLink(user.MagicLinkKindPasswordReset, u.ID, "", time.Hour)
		require.Nil(t, err)
		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, resetToken), nil)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)

		enroll := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		require.ErrorIs(t, s.userManager.ResetPassword(enroll.Code, "hijacked"), user.ErrMagicLinkNotFound)

		// Both links survive the attempt to use them for the wrong flow
		require.Nil(t, s.userManager.ResetPassword(resetToken, "phil2"))
		rr = request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, enroll.Code), nil)
		require.Equal(t, 200, rr.Code)
	})
}

func TestAccount_TokenEnroll_DeletedAccount(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser, false))

		enroll := enrollCode(t, s, "POST", "/v1/account/token/enroll", "", util.BasicAuth("phil", "phil"))
		rr := request(t, s, "DELETE", "/v1/account", `{"password":"phil"}`, map[string]string{
			"Authorization": util.BasicAuth("phil", "phil"),
		})
		require.Equal(t, 200, rr.Code)

		rr = request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s"}`, enroll.Code), nil)
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)
	})
}

func TestUser_TokenEnroll_AdminEnrollsUserForProtectedTopic(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		c := newTestConfigWithAuthFile(t, databaseURL)
		c.AuthDefault = user.PermissionDenyAll
		s := newTestServer(t, c)
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		admin := util.BasicAuth("phil", "phil")

		// The flow an integration (e.g. Authelia) runs: create a user that can only read one topic,
		// then mint a one-time code for the QR code instead of handing out a password
		require.Equal(t, 200, request(t, s, "POST", "/v1/users", `{"username":"device-1","password":"unused-secret"}`, map[string]string{"Authorization": admin}).Code)
		require.Equal(t, 200, request(t, s, "POST", "/v1/users/access", `{"username":"device-1","topic":"device-1-topic","permission":"read-only"}`, map[string]string{"Authorization": admin}).Code)
		enroll := enrollCode(t, s, "POST", "/v1/users/token/enroll", `{"username":"device-1"}`, admin)

		rr := request(t, s, "POST", "/v1/account/token/redeem", fmt.Sprintf(`{"code":"%s","label":"Phone"}`, enroll.Code), nil)
		require.Equal(t, 200, rr.Code)
		token, err := util.UnmarshalJSON[apiAccountTokenRedeemResponse](io.NopCloser(rr.Body))
		require.Nil(t, err)
		require.Equal(t, "device-1", token.Username)
		device := map[string]string{"Authorization": util.BearerAuth(token.Token)}

		require.Equal(t, 200, request(t, s, "GET", "/device-1-topic/json?poll=1", "", device).Code)
		require.Equal(t, 403, request(t, s, "PUT", "/device-1-topic", "spoofed", device).Code)
		require.Equal(t, 403, request(t, s, "GET", "/device-1-topic/json?poll=1", "", nil).Code)
	})
}

func TestUser_TokenEnroll_AdminRestrictions(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()
		require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("other-admin", "other-admin", user.RoleAdmin, false))
		require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser, false))

		// Only admins can enroll other users
		rr := request(t, s, "POST", "/v1/users/token/enroll", `{"username":"ben"}`, map[string]string{"Authorization": util.BasicAuth("ben", "ben")})
		require.Equal(t, 401, rr.Code)

		// Unknown user
		rr = request(t, s, "POST", "/v1/users/token/enroll", `{"username":"nobody"}`, map[string]string{"Authorization": util.BasicAuth("phil", "phil")})
		require.Equal(t, 400, rr.Code)
		require.Equal(t, 40031, toHTTPError(t, rr.Body.String()).Code)

		// The admin API does not mint credentials for admins, matching what it can create and delete
		rr = request(t, s, "POST", "/v1/users/token/enroll", `{"username":"other-admin"}`, map[string]string{"Authorization": util.BasicAuth("phil", "phil")})
		require.Equal(t, 401, rr.Code)
	})
}

func TestAccount_TokenEnroll_WebLandingPage(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfigWithAuthFile(t, databaseURL))
		defer s.closeDatabases()

		// The code travels in the fragment, which browsers never send, so the page itself is static
		rr := request(t, s, "GET", "/account/token/redeem", "", nil)
		require.Equal(t, 200, rr.Code)
		require.Equal(t, "noindex", rr.Header().Get("X-Robots-Tag"))
		require.Equal(t, "no-referrer", rr.Header().Get("Referrer-Policy"))
	})
}

func enrollCode(t *testing.T, s *Server, method, url, body, authorization string) *apiAccountTokenEnrollResponse {
	rr := request(t, s, method, url, body, map[string]string{"Authorization": authorization})
	require.Equal(t, 200, rr.Code, rr.Body.String())
	enroll, err := util.UnmarshalJSON[apiAccountTokenEnrollResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	return enroll
}
