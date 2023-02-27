package server

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccount_Signup_Success(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)
	defer s.closeDatabases()

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	token, _ := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.NotEmpty(t, token.Token)
	require.True(t, time.Now().Add(71*time.Hour).Unix() < token.Expires)
	require.True(t, strings.HasPrefix(token.Token, "tk_"))
	require.Equal(t, "9.9.9.9", token.LastOrigin)
	require.True(t, token.LastAccess > time.Now().Unix()-2)
	require.True(t, token.LastAccess < time.Now().Unix()+2)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "phil", account.Username)
	require.Equal(t, "user", account.Role)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("", token.Token), // We allow a fake basic auth to make curl-ing easier (curl -u :<token>)
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, "phil", account.Username)
}

func TestAccount_Signup_UserExists(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)
	defer s.closeDatabases()

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
	defer s.closeDatabases()

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
	defer s.closeDatabases()

	log.Info("1")
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleAdmin))
	log.Info("2")
	require.Nil(t, s.userManager.AddUser("ben", "ben", user.RoleUser))
	log.Info("3")
	rr := request(t, s, "POST", "/v1/account", `{"username":"emma", "password":"emma"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	log.Info("4")
	rr = request(t, s, "POST", "/v1/account", `{"username":"marian", "password":"marian"}`, map[string]string{
		"Authorization": util.BasicAuth("ben", "ben"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Signup_Disabled(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = false
	s := newTestServer(t, conf)
	defer s.closeDatabases()

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40022, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Signup_Rate_Limit(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	for i := 0; i < 3; i++ {
		rr := request(t, s, "POST", "/v1/account", fmt.Sprintf(`{"username":"phil%d", "password":"mypass"}`, i), nil)
		require.Equal(t, 200, rr.Code, "failed on iteration %d", i)
	}
	rr := request(t, s, "POST", "/v1/account", `{"username":"notallowed", "password":"mypass"}`, nil)
	require.Equal(t, 429, rr.Code)
	require.Equal(t, 42906, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Get_Anonymous(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.VisitorRequestLimitReplenish = 86 * time.Second
	conf.VisitorEmailLimitReplenish = time.Hour
	conf.VisitorAttachmentTotalSizeLimit = 5123
	conf.AttachmentFileSizeLimit = 512
	s := newTestServer(t, conf)
	s.smtpSender = &testMailer{}
	defer s.closeDatabases()

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
	defer s.closeDatabases()

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	u, _ := s.userManager.User("phil")
	token, _ := s.userManager.CreateToken(u.ID, "", time.Unix(0, 0), netip.IPv4Unspecified())

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
	require.Equal(t, util.Int(86400), account.Notification.DeleteAfter)
	require.Equal(t, util.String("juntos"), account.Notification.Sound)
	require.Nil(t, account.Notification.MinPriority) // Not set
}

func TestAccount_Subscription_AddUpdateDelete(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

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
	require.Equal(t, "http://abc.com", account.Subscriptions[0].BaseURL)
	require.Equal(t, "def", account.Subscriptions[0].Topic)
	require.Nil(t, account.Subscriptions[0].DisplayName)

	rr = request(t, s, "PATCH", "/v1/account/subscription", `{"base_url": "http://abc.com", "topic": "def", "display_name": "ding dong"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, 1, len(account.Subscriptions))
	require.Equal(t, "http://abc.com", account.Subscriptions[0].BaseURL)
	require.Equal(t, "def", account.Subscriptions[0].Topic)
	require.Equal(t, util.String("ding dong"), account.Subscriptions[0].DisplayName)

	rr = request(t, s, "DELETE", "/v1/account/subscription", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"X-BaseURL":     "http://abc.com",
		"X-Topic":       "def",
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
	defer s.closeDatabases()

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/password", `{"password": "WRONG", "new_password": ""}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 400, rr.Code)

	rr = request(t, s, "POST", "/v1/account/password", `{"password": "WRONG", "new_password": "new password"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40026, toHTTPError(t, rr.Body.String()).Code)

	rr = request(t, s, "POST", "/v1/account/password", `{"password": "phil", "new_password": "new password"}`, map[string]string{
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
	defer s.closeDatabases()

	rr := request(t, s, "POST", "/v1/account/password", `{"password": "new password"}`, nil)
	require.Equal(t, 401, rr.Code)
}

func TestAccount_ExtendToken(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

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

	expires := time.Now().Add(999 * time.Hour)
	body := fmt.Sprintf(`{"token":"%s", "label":"some label", "expires": %d}`, token.Token, expires.Unix())
	rr = request(t, s, "PATCH", "/v1/account/token", body, map[string]string{
		"Authorization": util.BearerAuth(token.Token),
	})
	require.Equal(t, 200, rr.Code)
	token, err = util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.Equal(t, "some label", token.Label)
	require.Equal(t, expires.Unix(), token.Expires)
}

func TestAccount_ExtendToken_NoTokenProvided(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "PATCH", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"), // Not Bearer!
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40023, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_DeleteToken(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t))
	defer s.closeDatabases()

	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))

	rr := request(t, s, "POST", "/v1/account/token", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	token, err := util.UnmarshalJSON[apiAccountTokenResponse](io.NopCloser(rr.Body))
	require.Nil(t, err)
	require.True(t, token.Expires > time.Now().Add(71*time.Hour).Unix())

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

	rr = request(t, s, "DELETE", "/v1/account", `{"password":"mypass"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Account was marked deleted
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 401, rr.Code)

	// Cannot re-create account, since still exists
	rr = request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 409, rr.Code)
}

func TestAccount_Delete_Not_Allowed(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", "", nil)
	require.Equal(t, 401, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", `{"password":"mypass"}`, nil)
	require.Equal(t, 401, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account", `{"password":"INCORRECT"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40026, toHTTPError(t, rr.Body.String()).Code)
}

func TestAccount_Reservation_AddWithoutTierFails(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic":"mytopic", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 401, rr.Code)
}

func TestAccount_Reservation_AddAdminSuccess(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	// A user, an admin, and a reservation walk into a bar
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:             "pro",
		ReservationLimit: 2,
	}))
	require.Nil(t, s.userManager.AddUser("noadmin1", "pass", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("noadmin1", "pro"))
	require.Nil(t, s.userManager.AddReservation("noadmin1", "mytopic", user.PermissionDenyAll))

	require.Nil(t, s.userManager.AddUser("noadmin2", "pass", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("noadmin2", "pro"))

	require.Nil(t, s.userManager.AddUser("phil", "adminpass", user.RoleAdmin))

	// Admin can reserve topic
	rr := request(t, s, "POST", "/v1/account/reservation", `{"topic":"sometopic","everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "adminpass"),
	})
	require.Equal(t, 200, rr.Code)

	// User cannot reserve already reserved topic
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic":"mytopic","everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("noadmin2", "pass"),
	})
	require.Equal(t, 409, rr.Code)

	// Admin cannot reserve already reserved topic
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic":"mytopic","everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "adminpass"),
	})
	require.Equal(t, 409, rr.Code)

	reservations, err := s.userManager.Reservations("phil")
	require.Nil(t, err)
	require.Equal(t, 1, len(reservations))
	require.Equal(t, "sometopic", reservations[0].Topic)

	reservations, err = s.userManager.Reservations("noadmin1")
	require.Nil(t, err)
	require.Equal(t, 1, len(reservations))
	require.Equal(t, "mytopic", reservations[0].Topic)

	reservations, err = s.userManager.Reservations("noadmin2")
	require.Nil(t, err)
	require.Equal(t, 0, len(reservations))
}

func TestAccount_Reservation_AddRemoveUserWithTierSuccess(t *testing.T) {
	conf := newTestConfigWithAuthFile(t)
	conf.EnableSignup = true
	s := newTestServer(t, conf)

	// Create user
	rr := request(t, s, "POST", "/v1/account", `{"username":"phil", "password":"mypass"}`, nil)
	require.Equal(t, 200, rr.Code)

	// Create a tier
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                     "pro",
		MessageLimit:             123,
		MessageExpiryDuration:    86400 * time.Second,
		EmailLimit:               32,
		ReservationLimit:         2,
		AttachmentFileSizeLimit:  1231231,
		AttachmentTotalSizeLimit: 123123,
		AttachmentExpiryDuration: 10800 * time.Second,
		AttachmentBandwidthLimit: 21474836480,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Reserve two topics
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "mytopic", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "another", "everyone":"read-only"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Trying to reserve a third should fail
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "yet-another", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 429, rr.Code)

	// Modify existing should still work
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "another", "everyone":"write-only"}`, map[string]string{
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
	require.Equal(t, int64(21474836480), account.Limits.AttachmentBandwidth)
	require.Equal(t, 2, len(account.Reservations))
	require.Equal(t, "another", account.Reservations[0].Topic)
	require.Equal(t, "write-only", account.Reservations[0].Everyone)
	require.Equal(t, "mytopic", account.Reservations[1].Topic)
	require.Equal(t, "deny-all", account.Reservations[1].Everyone)

	// Delete and re-check
	rr = request(t, s, "DELETE", "/v1/account/reservation/another", "", map[string]string{
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

	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:             "pro",
		MessageLimit:     20,
		ReservationLimit: 2,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Reserve a topic
	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "mytopic", "everyone":"deny-all"}`, map[string]string{
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

func TestAccount_Reservation_Delete_Messages_And_Attachments(t *testing.T) {
	t.Parallel()
	conf := newTestConfigWithAuthFile(t)
	conf.AuthDefault = user.PermissionReadWrite
	s := newTestServer(t, conf)

	// Create user with tier
	require.Nil(t, s.userManager.AddUser("phil", "mypass", user.RoleUser))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:                     "pro",
		MessageLimit:             20,
		MessageExpiryDuration:    time.Hour,
		ReservationLimit:         2,
		AttachmentTotalSizeLimit: 10000,
		AttachmentFileSizeLimit:  10000,
		AttachmentExpiryDuration: time.Hour,
		AttachmentBandwidthLimit: 10000,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Reserve two topics "mytopic1" and "mytopic2"
	rr := request(t, s, "POST", "/v1/account/reservation", `{"topic": "mytopic1", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/account/reservation", `{"topic": "mytopic2", "everyone":"deny-all"}`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Publish a message with attachment to each topic
	rr = request(t, s, "POST", "/mytopic1?f=attach.txt", `Howdy`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	m1 := toMessage(t, rr.Body.String())
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, m1.ID))

	rr = request(t, s, "POST", "/mytopic2?f=attach.txt", `Howdy`, map[string]string{
		"Authorization": util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)
	m2 := toMessage(t, rr.Body.String())
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, m2.ID))

	// Delete reservation
	rr = request(t, s, "DELETE", "/v1/account/reservation/mytopic1", ``, map[string]string{
		"X-Delete-Messages": "true",
		"Authorization":     util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "DELETE", "/v1/account/reservation/mytopic2", ``, map[string]string{
		"X-Delete-Messages": "false",
		"Authorization":     util.BasicAuth("phil", "mypass"),
	})
	require.Equal(t, 200, rr.Code)

	// Verify that messages and attachments were deleted
	// This does not explicitly call the manager!
	time.Sleep(time.Second)

	ms, err := s.messageCache.Messages("mytopic1", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 0, len(ms))
	require.NoFileExists(t, filepath.Join(s.config.AttachmentCacheDir, m1.ID))

	ms, err = s.messageCache.Messages("mytopic2", sinceAllMessages, false)
	require.Nil(t, err)
	require.Equal(t, 1, len(ms))
	require.Equal(t, m2.ID, ms[0].ID)
	require.FileExists(t, filepath.Join(s.config.AttachmentCacheDir, m2.ID))
}

func TestAccount_Persist_UserStats_After_Tier_Change(t *testing.T) {
	t.Parallel()
	conf := newTestConfigWithAuthFile(t)
	conf.AuthDefault = user.PermissionReadWrite
	conf.AuthStatsQueueWriterInterval = 100 * time.Millisecond
	s := newTestServer(t, conf)
	defer s.closeDatabases()

	// Create user with tier
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "starter",
		MessageLimit: 10,
	}))
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 20,
	}))
	require.Nil(t, s.userManager.ChangeTier("phil", "starter"))

	// Publish a message
	rr := request(t, s, "POST", "/mytopic", "hi", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Wait for stats queue writer
	time.Sleep(600 * time.Millisecond)

	// Verify that message stats were persisted
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(1), u.Stats.Messages)

	// Change tier, make a request (to reset limiters)
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ := util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(1), account.Stats.Messages) // Is not reset!

	// Publish another message
	rr = request(t, s, "POST", "/mytopic", "hi", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)

	// Verify that message stats were persisted
	time.Sleep(600 * time.Millisecond)
	u, err = s.userManager.User("phil")
	require.Nil(t, err)
	require.Equal(t, int64(2), u.Stats.Messages) // v.EnqueueUserStats had run!

	// Stats keep counting
	rr = request(t, s, "GET", "/v1/account", "", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, rr.Code)
	account, _ = util.UnmarshalJSON[apiAccountResponse](io.NopCloser(rr.Body))
	require.Equal(t, int64(2), account.Stats.Messages) // Is not reset!
}
