package server

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"testing"
	"time"
)

func TestAccount_Signup_Success(t *testing.T) {
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

func TestAccount_Signup_UserExists(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 409, rr.Code)
	require.Equal(t, 40901, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Signup_LimitReached(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	for i := 0; i < 3; i++ {
		rr := request(t, s, "POST", "/v1/account", fmt.Sprintf(`{"username":"phil%d", "password":"mypass"}`, i), nil)
		require.Equal(t, 200, rr.Code)
	}
	rr := request(t, s, "POST", "/v1/account", `{"username":"thiswontwork", "password":"mypass"}`, nil)
	require.Equal(t, 429, rr.Code)
	require.Equal(t, 42906, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Signup_Disabled(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = false
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40022, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Get_Anonymous(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.VisitorRequestLimitReplenish = 86 * time.Second
	conf.VisitorEmailLimitReplenish = time.Hour
	conf.VisitorAttachmentTotalSizeLimit = 5123
	conf.AttachmentFileSizeLimit = 512
	s := newTestServer(t, conf)
	s.smtpSender = &testMailer{}

	rr := request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, rr.Code)
	account, _ := util.ReadJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "*", account.Username)
	require.Equal(t, string(user.RoleAnonymous), account.Role)
	require.Equal(t, "ip", account.Limits.Basis)
	require.Equal(t, int64(1004), account.Limits.Messages) // I hate this
	require.Equal(t, int64(24), account.Limits.Emails)     // I hate this
	require.Equal(t, int64(5123), account.Limits.AttachmentTotalSize)
	require.Equal(t, int64(512), account.Limits.AttachmentFileSize)
	require.Equal(t, int64(0), account.Stats.Messages)
	require.Equal(t, int64(1004), account.Stats.MessagesRemaining)
	require.Equal(t, int64(0), account.Stats.Emails)
	require.Equal(t, int64(24), account.Stats.EmailsRemaining)

	rr = request(t, s, "POST", "/mytopic", "", nil)
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "POST", "/mytopic", "", map[string]string{
		"Email": "phil@ntfy.sh",
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, rr.Code)
	account, _ = util.ReadJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(2), account.Stats.Messages)
	require.Equal(t, int64(1002), account.Stats.MessagesRemaining)
	require.Equal(t, int64(1), account.Stats.Emails)
	require.Equal(t, int64(23), account.Stats.EmailsRemaining)
}

func TestAccount_Delete_Success(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Delete_Not_Allowed(t *testing.T) {
	conf := newTestConfigWithUsers(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", "", nil)
	require.Equal(t, 401, rr.Code)
}
