package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestServer_Twilio_Call_Add_Verify_Call_Delete_Success(t *testing.T) {
	var called, verified atomic.Bool
	var code atomic.Pointer[string]
	twilioVerifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		if r.URL.Path == "/v2/Services/VA1234567890/Verifications" {
			if code.Load() != nil {
				t.Fatal("Should be only called once")
			}
			require.Equal(t, "Channel=sms&To=%2B12223334444", string(body))
			code.Store(util.String("123456"))
		} else if r.URL.Path == "/v2/Services/VA1234567890/VerificationCheck" {
			if verified.Load() {
				t.Fatal("Should be only called once")
			}
			require.Equal(t, "Code=123456&To=%2B12223334444", string(body))
			verified.Store(true)
		} else {
			t.Fatal("Unexpected path:", r.URL.Path)
		}
	}))
	defer twilioVerifyServer.Close()
	twilioCallsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called.Load() {
			t.Fatal("Should be only called once")
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "From=%2B1234567890&To=%2B12223334444&Twiml=%0A%3CResponse%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay+loop%3D%223%22%3E%0A%09%09You+have+a+message+from+notify+on+topic+mytopic.+Message%3A%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09hi+there%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09End+of+message.%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09This+message+was+sent+by+user+phil.+It+will+be+repeated+three+times.%0A%09%09To+unsubscribe+from+calls+like+this%2C+remove+your+phone+number+in+the+notify+web+app.%0A%09%09%3Cbreak+time%3D%223s%22%2F%3E%0A%09%3C%2FSay%3E%0A%09%3CSay%3EGoodbye.%3C%2FSay%3E%0A%3C%2FResponse%3E", string(body))
		called.Store(true)
	}))
	defer twilioCallsServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.TwilioVerifyBaseURL = twilioVerifyServer.URL
	c.TwilioCallsBaseURL = twilioCallsServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	c.TwilioVerifyService = "VA1234567890"
	s := newTestServer(t, c)

	// Add tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
		CallLimit:    1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)

	// Send verification code for phone number
	response := request(t, s, "PUT", "/v1/account/phone/verify", `{"number":"+12223334444","channel":"sms"}`, map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	waitFor(t, func() bool {
		return *code.Load() == "123456"
	})

	// Add phone number with code
	response = request(t, s, "PUT", "/v1/account/phone", `{"number":"+12223334444","code":"123456"}`, map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)
	waitFor(t, func() bool {
		return verified.Load()
	})
	phoneNumbers, err := s.userManager.PhoneNumbers(u.ID)
	require.Nil(t, err)
	require.Equal(t, 1, len(phoneNumbers))
	require.Equal(t, "+12223334444", phoneNumbers[0])

	// Do the thing
	response = request(t, s, "POST", "/mytopic", "hi there", map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
		"x-call":        "yes",
	})
	require.Equal(t, "hi there", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})

	// Remove the phone number
	response = request(t, s, "DELETE", "/v1/account/phone", `{"number":"+12223334444"}`, map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
	})
	require.Equal(t, 200, response.Code)

	// Verify the phone number is gone from the DB
	phoneNumbers, err = s.userManager.PhoneNumbers(u.ID)
	require.Nil(t, err)
	require.Equal(t, 0, len(phoneNumbers))
}

func TestServer_Twilio_Call_Success(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called.Load() {
			t.Fatal("Should be only called once")
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "From=%2B1234567890&To=%2B11122233344&Twiml=%0A%3CResponse%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay+loop%3D%223%22%3E%0A%09%09You+have+a+message+from+notify+on+topic+mytopic.+Message%3A%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09hi+there%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09End+of+message.%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09This+message+was+sent+by+user+phil.+It+will+be+repeated+three+times.%0A%09%09To+unsubscribe+from+calls+like+this%2C+remove+your+phone+number+in+the+notify+web+app.%0A%09%09%3Cbreak+time%3D%223s%22%2F%3E%0A%09%3C%2FSay%3E%0A%09%3CSay%3EGoodbye.%3C%2FSay%3E%0A%3C%2FResponse%3E", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.TwilioCallsBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)

	// Add tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
		CallLimit:    1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, s.userManager.AddPhoneNumber(u.ID, "+11122233344"))

	// Do the thing
	response := request(t, s, "POST", "/mytopic", "hi there", map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
		"x-call":        "+11122233344",
	})
	require.Equal(t, "hi there", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})
}

func TestServer_Twilio_Call_Success_With_Yes(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called.Load() {
			t.Fatal("Should be only called once")
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "From=%2B1234567890&To=%2B11122233344&Twiml=%0A%3CResponse%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay+loop%3D%223%22%3E%0A%09%09You+have+a+message+from+notify+on+topic+mytopic.+Message%3A%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09hi+there%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09End+of+message.%0A%09%09%3Cbreak+time%3D%221s%22%2F%3E%0A%09%09This+message+was+sent+by+user+phil.+It+will+be+repeated+three+times.%0A%09%09To+unsubscribe+from+calls+like+this%2C+remove+your+phone+number+in+the+notify+web+app.%0A%09%09%3Cbreak+time%3D%223s%22%2F%3E%0A%09%3C%2FSay%3E%0A%09%3CSay%3EGoodbye.%3C%2FSay%3E%0A%3C%2FResponse%3E", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.TwilioCallsBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)

	// Add tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
		CallLimit:    1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))
	u, err := s.userManager.User("phil")
	require.Nil(t, err)
	require.Nil(t, s.userManager.AddPhoneNumber(u.ID, "+11122233344"))

	// Do the thing
	response := request(t, s, "POST", "/mytopic", "hi there", map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
		"x-call":        "yes", // <<<------
	})
	require.Equal(t, "hi there", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})
}

func TestServer_Twilio_Call_UnverifiedNumber(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.TwilioCallsBaseURL = "http://dummy.invalid"
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)

	// Add tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
		CallLimit:    1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Do the thing
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
		"x-call":        "+11122233344",
	})
	require.Equal(t, 40034, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_Call_InvalidNumber(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.TwilioCallsBaseURL = "https://127.0.0.1"
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-call": "+invalid",
	})
	require.Equal(t, 40033, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_Call_Anonymous(t *testing.T) {
	c := newTestConfigWithAuthFile(t)
	c.TwilioCallsBaseURL = "https://127.0.0.1"
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioPhoneNumber = "+1234567890"
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-call": "+123123",
	})
	require.Equal(t, 40035, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_Call_Unconfigured(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-call": "+1234",
	})
	require.Equal(t, 40032, toHTTPError(t, response.Body.String()).Code)
}
