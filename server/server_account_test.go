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
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	token, _ := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.NotEmpty(t, token.Token)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token.Expires)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "phil", account.Username)
	require.Equal(t, "user", account.Role)
}

func TestAccount_Signup_UserExists(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 409, rr.Code)
	require.Equal(t, 40901, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Signup_LimitReached(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
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

func TestAccount_Signup_AsUser(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account", `{"username":"emma", "password":"emma"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account", `{"username":"marian", "password":"marian"}`, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Signup_Disabled(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = false
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40022, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Get_Anonymous(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.VisitorRequestLimitReplenish = 86 * time.Second
	conf.VisitorEmailLimitReplenish = time.Hour
	conf.VisitorAttachmentTotalSizeLimit = 5123
	conf.AttachmentFileSizeLimit = 512
	s := newTestServer(t, conf)
	s.smtpSender = &testMailer{}

	rr := request(t, s, "GET", "/v1/account", "", nil)
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
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
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(2), account.Stats.Messages)
	require.Equal(t, int64(1002), account.Stats.MessagesRemaining)
	require.Equal(t, int64(1), account.Stats.Emails)
	require.Equal(t, int64(23), account.Stats.EmailsRemaining)
}

func TestAccount_ChangeSettings(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	user, _ := s.userManager.User("phil")
	token, _ := s.userManager.CreateToken(user)

	rr := request(t, s, "PATCH", "/v1/account/settings", `{"notification": {"sound": "juntos"},"ignored": true}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "PATCH", "/v1/account/settings", `{"notification": {"delete_after": 86400}, "language": "de"}`, map[string]string{
		"Authorization": util.BearerAuth(token.Value),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", `{"username":"marian", "password":"marian"}`, map[string]string{
		"Authorization": util.BearerAuth(token.Value),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "de", account.Language)
	require.Equal(t, 86400, account.Notification.DeleteAfter)
	require.Equal(t, "juntos", account.Notification.Sound)
	require.Equal(t, 0, account.Notification.MinPriority) // Not set
}

func TestAccount_Subscription_AddUpdateDelete(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/subscription", `{"base_url": "http://abc.com", "topic": "def"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, 1, len(account.Subscriptions))
	require.NotEmpty(t, account.Subscriptions[0].ID)
	require.Equal(t, "http://abc.com", account.Subscriptions[0].BaseURL)
	require.Equal(t, "def", account.Subscriptions[0].Topic)
	require.Equal(t, "", account.Subscriptions[0].DisplayName)

	subscriptionID := account.Subscriptions[0].ID
	rr = request(t, s, "PATCH", "/v1/account/subscription/"+subscriptionID, `{"display_name": "ding dong"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, 1, len(account.Subscriptions))
	require.Equal(t, subscriptionID, account.Subscriptions[0].ID)
	require.Equal(t, "http://abc.com", account.Subscriptions[0].BaseURL)
	require.Equal(t, "def", account.Subscriptions[0].Topic)
	require.Equal(t, "ding dong", account.Subscriptions[0].DisplayName)

	rr = request(t, s, "DELETE", "/v1/account/subscription/"+subscriptionID, "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, 0, len(account.Subscriptions))
}

func TestAccount_ChangePassword(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/password", `{"password": "new password"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 401, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "new password"),
	})
	require.Equal(t, 200, rr.Code)
}

func TestAccount_ChangePassword_NoAccount(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))

	rr := request(t, s, "POST", "/v1/account/password", `{"password": "new password"}`, nil)
	require.Equal(t, 401, rr.Code)
}

func TestAccount_ExtendToken(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	token, err := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)

	time.Sleep(time.Second)

	rr = request(t, s, "PATCH", "/v1/account/token", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)
	extendedToken, err := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.Equal(t, token.Token, extendedToken.Token)
	require.True(t, token.Expires < extendedToken.Expires)
}

func TestAccount_ExtendToken_NoTokenProvided(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "PATCH", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"), // Not Bearer!
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40023, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_DeleteToken(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	token, err := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)

	// Delete token failure (using basic auth)
	rr = request(t, s, "DELETE", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"), // Not Bearer!
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40023, toHTTPError(t, rr.Body.String()).Code)

	// Delete token with wrong token
	rr = request(t, s, "DELETE", "/v1/account/token", "", map[string]string{
		"Authorization": util.BearerAuth("invalidtoken"),
	})
	require.Equal(t, 401, rr.Code)

	// Delete token with correct token
	rr = request(t, s, "DELETE", "/v1/account/token", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)

	// Cannot get account anymore
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Delete_Success(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
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
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", "", nil)
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Reservation_AddWithoutTierFails(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/access", `{"topic":"mytopic", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Reservation_AddAdminSuccess(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)
	require.Nil(t, s.userManager.AddUser("phil", "adminpass", user.RoleAdmin))

	rr := request(t, s, "POST", "/v1/account/access", `{"topic":"mytopic","everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "adminpass"),
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40026, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Reservation_AddRemoveUserWithTierSuccess(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	// Create user
	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	// Create a tier
	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		Code:                     "pro",
		Upgradeable:              false,
		MessagesLimit:            123,
		MessagesExpiryDuration:   86400,
		EmailsLimit:              32,
		ReservationsLimit:        2,
		AttachmentFileSizeLimit:  1231231,
		AttachmentTotalSizeLimit: 123123,
		AttachmentExpiryDuration: 10800,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Reserve two topics
	rr = request(t, s, "POST", "/v1/account/access", `{"topic": "mytopic", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/access", `{"topic": "another", "everyone":"read-only"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Trying to reserve a third should fail
	rr = request(t, s, "POST", "/v1/account/access", `{"topic": "yet-another", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 429, rr.Code)

	// Modify existing should still work
	rr = request(t, s, "POST", "/v1/account/access", `{"topic": "another", "everyone":"write-only"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Check account result
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "pro", account.Tier.Code)
	require.Equal(t, int64(123), account.Limits.Messages)
	require.Equal(t, int64(86400), account.Limits.MessagesExpiryDuration)
	require.Equal(t, int64(32), account.Limits.Emails)
	require.Equal(t, int64(2), account.Limits.Reservations)
	require.Equal(t, int64(1231231), account.Limits.AttachmentFileSize)
	require.Equal(t, int64(123123), account.Limits.AttachmentTotalSize)
	require.Equal(t, int64(10800), account.Limits.AttachmentExpiryDuration)
	require.Equal(t, 2, len(account.Reservations))
	require.Equal(t, "another", account.Reservations[0].Topic)
	require.Equal(t, "write-only", account.Reservations[0].Everyone)
	require.Equal(t, "mytopic", account.Reservations[1].Topic)
	require.Equal(t, "deny-all", account.Reservations[1].Everyone)

	// Delete and re-check
	rr = request(t, s, "DELETE", "/v1/account/access/another", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, 1, len(account.Reservations))
	require.Equal(t, "mytopic", account.Reservations[0].Topic)
}

func TestAccount_Reservation_PublishByAnonymousFails(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.AuthDefault = user.PermissionReadWrite
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	// Create user with tier
	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	require.Nil(t, s.userManager.CreateTier(&user.Tier{
		Code:              "pro",
		MessagesLimit:     20,
		ReservationsLimit: 2,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Reserve a topic
	rr = request(t, s, "POST", "/v1/account/access", `{"topic": "mytopic", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Publish a message
	rr = request(t, s, "POST", "/mytopic", `Howdy`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Publish a message (as anonymous)
	rr = request(t, s, "POST", "/mytopic", `Howdy`, nil)
	require.Equal(t, 403, rr.Code)
}
