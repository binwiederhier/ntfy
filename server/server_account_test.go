package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/util"
	"io"
	"testing"
	"time"
)

func TestAccount_Create_Success(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	token, _ := util.ReadJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.NotEmpty(t, token.Token)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token.Expires)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.ReadJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "phil", account.Username)
	require.Equal(t, "user", account.Role)
}

func TestAccount_Create_Disabled(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = false
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40022, toHTTPError(t, rr.Body.String()).Code)
}
