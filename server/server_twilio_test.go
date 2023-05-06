package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestServer_Twilio_SMS(t *testing.T) {
	c := newTestConfig(t)
	c.TwilioBaseURL = "http://"
	c.TwilioAccount = "AC123"
	c.TwilioAuthToken = "secret-token"
	c.TwilioFromNumber = "+123456789"
	s := newTestServer(t, c)

	response := request(t, s, "POST", "/mytopic", "test", map[string]string{
		"SMS": "+11122233344",
	})
	require.Equal(t, 1, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=low", "test", nil)
	require.Equal(t, 2, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=default", "test", nil)
	require.Equal(t, 3, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=high", "test", nil)
	require.Equal(t, 4, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/send?priority=max", "test", nil)
	require.Equal(t, 5, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/trigger?priority=urgent", "test", nil)
	require.Equal(t, 5, toMessage(t, response.Body.String()).Priority)

	response = request(t, s, "GET", "/mytopic/trigger?priority=INVALID", "test", nil)
	require.Equal(t, 40007, toHTTPError(t, response.Body.String()).Code)
}
