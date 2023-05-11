package server

import (
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestServer_Twilio_SMS(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Messages.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "Body=test%0A%0A--%0AThis+message+was+sent+by+9.9.9.9+via+ntfy.sh%2Fmytopic&From=%2B1234567890&To=%2B11122233344", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfig(t)
	c.BaseURL = "https://ntfy.sh"
	c.TwilioMessagingBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
	c.VisitorSMSDailyLimit = 1
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"SMS": "+11122233344",
	})
	require.Equal(t, "test", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})
}

func TestServer_Twilio_SMS_With_User(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called.Load() {
			t.Fatal("Should be only called once")
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Messages.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "Body=test%0A%0A--%0AThis+message+was+sent+by+phil+%289.9.9.9%29+via+ntfy.sh%2Fmytopic&From=%2B1234567890&To=%2B11122233344", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.BaseURL = "https://ntfy.sh"
	c.TwilioMessagingBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
	s := newTestServer(t, c)

	// Add tier and user
	require.Nil(t, s.userManager.AddTier(&user.Tier{
		Code:         "pro",
		MessageLimit: 10,
		SMSLimit:     1,
	}))
	require.Nil(t, s.userManager.AddUser("phil", "phil", user.RoleUser))
	require.Nil(t, s.userManager.ChangeTier("phil", "pro"))

	// Do request with user
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"SMS":           "+11122233344",
	})
	require.Equal(t, "test", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})

	// Second one should fail due to rate limits
	response = request(t, s, "POST", "/mytopic", "test", map[string]string{
		"Authorization": util.BasicAuth("phil", "phil"),
		"SMS":           "+11122233344",
	})
	require.Equal(t, 42910, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_Call(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "From=%2B1234567890&To=%2B11122233344&Twiml=%0A%3CResponse%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EYou+have+a+message+from+notify+on+topic+mytopic.+Message%3A%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3Ethis+message+has%26%23xA%3Ba+new+line+and+%26lt%3Bbrackets%26gt%3B%21%26%23xA%3Band+%26%2334%3Bquotes+and+other+%26%2339%3Bquotes%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EEnd+message.%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EThis+message+was+sent+by+9.9.9.9+via+127.0.0.1%3A12345%2Fmytopic%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%3C%2FResponse%3E", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfig(t)
	c.TwilioMessagingBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
	c.VisitorCallDailyLimit = 1
	s := newTestServer(t, c)

	body := `this message has
a new line and <brackets>!
and "quotes and other 'quotes`
	response := request(t, s, "POST", "/mytopic", body, map[string]string{
		"x-call": "+11122233344",
	})
	require.Equal(t, "this message has\na new line and <brackets>!\nand \"quotes and other 'quotes", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})
}

func TestServer_Twilio_Call_With_User(t *testing.T) {
	var called atomic.Bool
	twilioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if called.Load() {
			t.Fatal("Should be only called once")
		}
		body, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "From=%2B1234567890&To=%2B11122233344&Twiml=%0A%3CResponse%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EYou+have+a+message+from+notify+on+topic+mytopic.+Message%3A%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3Ehi+there%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EEnd+message.%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%09%3CSay%3EThis+message+was+sent+by+phil+%289.9.9.9%29+via+127.0.0.1%3A12345%2Fmytopic%3C%2FSay%3E%0A%09%3CPause+length%3D%221%22%2F%3E%0A%3C%2FResponse%3E", string(body))
		called.Store(true)
	}))
	defer twilioServer.Close()

	c := newTestConfigWithAuthFile(t)
	c.TwilioMessagingBaseURL = twilioServer.URL
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
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
	response := request(t, s, "POST", "/mytopic", "hi there", map[string]string{
		"authorization": util.BasicAuth("phil", "phil"),
		"x-call":        "+11122233344",
	})
	require.Equal(t, "hi there", toMessage(t, response.Body.String()).Message)
	waitFor(t, func() bool {
		return called.Load()
	})
}

func TestServer_Twilio_Call_InvalidNumber(t *testing.T) {
	c := newTestConfig(t)
	c.TwilioMessagingBaseURL = "https://127.0.0.1"
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-call": "+invalid",
	})
	require.Equal(t, 40031, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_SMS_InvalidNumber(t *testing.T) {
	c := newTestConfig(t)
	c.TwilioMessagingBaseURL = "https://127.0.0.1"
	c.TwilioAccount = "AC1234567890"
	c.TwilioAuthToken = "AAEAA1234567890"
	c.TwilioFromNumber = "+1234567890"
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-sms": "+invalid",
	})
	require.Equal(t, 40031, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_SMS_Unconfigured(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-sms": "+1234",
	})
	require.Equal(t, 40030, toHTTPError(t, response.Body.String()).Code)
}

func TestServer_Twilio_Call_Unconfigured(t *testing.T) {
	s := newTestServer(t, newTestConfig(t))
	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"x-call": "+1234",
	})
	require.Equal(t, 40030, toHTTPError(t, response.Body.String()).Code)
}
