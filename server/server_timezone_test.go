package server

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServer_PublishTimezone(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		for _, zone := range []string{"Asia/Tokyo", "America/New_York", "UTC"} {
			for _, transport := range []string{"header", "x-header", "query", "json"} {
				t.Run(zone+"/"+transport, func(t *testing.T) {
					s := newTestServer(t, newTestConfig(t, databaseURL))
					location, err := time.LoadLocation(zone)
					require.NoError(t, err)
					tomorrow := func() int64 {
						now := time.Now().In(location)
						return time.Date(now.Year(), now.Month(), now.Day()+1, 10, 0, 0, 0, location).Unix()
					}
					path, body := "/mytopic", "a message"
					headers := map[string]string{"Delay": "tomorrow 10am"}
					switch transport {
					case "header":
						headers["Timezone"] = zone
					case "x-header":
						headers["X-Timezone"] = zone
					case "query":
						path += "?timezone=" + url.QueryEscape(zone)
					case "json":
						path = "/"
						body = fmt.Sprintf(`{"topic":"mytopic","message":"a message","delay":"tomorrow 10am","timezone":%q}`, zone)
						headers = nil
					}
					before := tomorrow()
					response := request(t, s, "POST", path, body, headers)
					require.Equal(t, 200, response.Code, response.Body.String())
					message := toMessage(t, response.Body.String())
					require.Contains(t, []int64{before, tomorrow()}, message.Time)
				})
			}
		}
	})
}

func TestServer_PublishTimezoneRelativeAndUnix(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfig(t, databaseURL))
		for _, delay := range []string{"1h", fmt.Sprint(time.Now().Add(time.Hour).Unix())} {
			before := time.Now().Add(time.Hour).Unix()
			response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
				"Delay": delay, "Timezone": "Asia/Tokyo",
			})
			require.Equal(t, 200, response.Code, response.Body.String())
			require.InDelta(t, before, toMessage(t, response.Body.String()).Time, 2)
		}
	})
}

func TestServer_PublishTimezoneInvalid(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfig(t, databaseURL))
		for _, zone := range []string{"Unknown/Timezone", "../etc/passwd", "+09:00"} {
			response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
				"Delay": "1h", "Timezone": zone,
			})
			require.Equal(t, 400, response.Code)
			require.Equal(t, errHTTPBadRequestTimezoneInvalid, toHTTPError(t, response.Body.String()))
		}
	})
}

func TestServer_PublishTimezoneWithoutDelay(t *testing.T) {
	forEachBackend(t, func(t *testing.T, databaseURL string) {
		s := newTestServer(t, newTestConfig(t, databaseURL))
		response := request(t, s, "PUT", "/mytopic", "a message", map[string]string{
			"Timezone": "Unknown/Timezone",
		})
		require.Equal(t, 200, response.Code)
		require.InDelta(t, time.Now().Unix(), toMessage(t, response.Body.String()).Time, 2)
	})
}
