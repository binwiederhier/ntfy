package server

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/monitor"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

func newTestConfigWithMonitor(t *testing.T) *Config {
	conf := newTestConfigWithAuthFile(t, "")
	conf.MonitorFile = filepath.Join(t.TempDir(), "monitor.db")
	conf.MonitorCheckInterval = time.Hour // disable the background ticker for handler tests
	return conf
}

func addUser(t *testing.T, s *Server, name string) {
	require.Nil(t, s.userManager.AddUser(name, name, user.RoleUser, false))
}

func basicAuth(name string) map[string]string {
	return map[string]string{"Authorization": util.BasicAuth(name, name)}
}

func TestMonitor_Add_Success(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")

	body := `{"key":"scanner","period":300,"grace":60,"alert_topic":"alerts","alert_priority":4}`
	rr := request(t, s, "POST", "/v1/monitors", body, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)

	var resp monitorResponse
	require.Nil(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Equal(t, "scanner", resp.Key)
	require.Equal(t, int64(300), resp.Period)
	require.Equal(t, int64(60), resp.Grace)
	require.Equal(t, "alerts", resp.AlertTopic)
	require.Equal(t, 4, resp.AlertPriority)
	require.Equal(t, monitor.StatePending, resp.State)
}

func TestMonitor_Add_DefaultPriority(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")

	body := `{"key":"scanner","period":300,"grace":60,"alert_topic":"alerts"}`
	rr := request(t, s, "POST", "/v1/monitors", body, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	resp, _ := util.UnmarshalJSON[monitorResponse](io.NopCloser(rr.Body))
	require.Equal(t, monitor.DefaultAlertPriority, resp.AlertPriority)
}

func TestMonitor_Add_Unauthorized(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"x","period":300,"grace":60,"alert_topic":"a"}`, nil)
	require.Equal(t, 401, rr.Code)
}

func TestMonitor_Add_DuplicateKey(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")

	body := `{"key":"scanner","period":300,"grace":60,"alert_topic":"alerts"}`
	rr := request(t, s, "POST", "/v1/monitors", body, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "POST", "/v1/monitors", body, basicAuth("phil"))
	require.Equal(t, 409, rr.Code)
	require.Equal(t, 40908, toHTTPError(t, rr.Body.String()).Code)
}

func TestMonitor_Add_FeatureDisabled(t *testing.T) {
	s := newTestServer(t, newTestConfigWithAuthFile(t, ""))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"x","period":300,"grace":60,"alert_topic":"a"}`, basicAuth("phil"))
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40066, toHTTPError(t, rr.Body.String()).Code)
}

func TestMonitor_Add_BadBody(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `not json`, basicAuth("phil"))
	require.Equal(t, 400, rr.Code)
	require.Equal(t, 40060, toHTTPError(t, rr.Body.String()).Code)
}

func TestMonitor_Add_ValidationErrors(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")

	cases := []struct {
		name string
		body string
		code int
	}{
		{"bad key", `{"key":"bad/key","period":300,"grace":60,"alert_topic":"a"}`, 40061},
		{"period low", `{"key":"k","period":1,"grace":60,"alert_topic":"a"}`, 40062},
		{"grace low", `{"key":"k","period":300,"grace":0,"alert_topic":"a"}`, 40063},
		{"bad topic", `{"key":"k","period":300,"grace":60,"alert_topic":"bad/topic"}`, 40064},
		{"prio high", `{"key":"k","period":300,"grace":60,"alert_topic":"a","alert_priority":9}`, 40065},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := request(t, s, "POST", "/v1/monitors", tc.body, basicAuth("phil"))
			require.Equal(t, 400, rr.Code)
			require.Equal(t, tc.code, toHTTPError(t, rr.Body.String()).Code)
		})
	}
}

func TestMonitor_List_ScopedToUser(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	addUser(t, s, "ben")

	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"key":"k%d","period":300,"grace":60,"alert_topic":"a"}`, i)
		rr := request(t, s, "POST", "/v1/monitors", body, basicAuth("phil"))
		require.Equal(t, 200, rr.Code)
	}
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"ben1","period":300,"grace":60,"alert_topic":"a"}`, basicAuth("ben"))
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "GET", "/v1/monitors", "", basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	list, _ := util.UnmarshalJSON[monitorListResponse](io.NopCloser(rr.Body))
	require.Len(t, list.Monitors, 3)

	rr = request(t, s, "GET", "/v1/monitors", "", basicAuth("ben"))
	require.Equal(t, 200, rr.Code)
	list, _ = util.UnmarshalJSON[monitorListResponse](io.NopCloser(rr.Body))
	require.Len(t, list.Monitors, 1)
	require.Equal(t, "ben1", list.Monitors[0].Key)
}

func TestMonitor_Get_NotFound(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "GET", "/v1/monitors/ghost", "", basicAuth("phil"))
	require.Equal(t, 404, rr.Code)
	require.Equal(t, 40402, toHTTPError(t, rr.Body.String()).Code)
}

func TestMonitor_Delete(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"x","period":300,"grace":60,"alert_topic":"a"}`, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "DELETE", "/v1/monitors/x", "", basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "DELETE", "/v1/monitors/x", "", basicAuth("phil"))
	require.Equal(t, 404, rr.Code)
}

func TestMonitor_Heartbeat_PendingToUp(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"scanner","period":300,"grace":60,"alert_topic":"alerts"}`, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)

	rr = request(t, s, "POST", "/v1/heartbeat/scanner", "", basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	resp, _ := util.UnmarshalJSON[heartbeatResponse](io.NopCloser(rr.Body))
	require.Equal(t, monitor.StateUp, resp.State)
	require.Equal(t, monitor.StatePending, resp.PrevState)
}

func TestMonitor_Heartbeat_DownToUp_PublishesAlert(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"scanner","period":300,"grace":60,"alert_topic":"alerts"}`, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)

	// Directly mark down via the manager to simulate a missed heartbeat
	mon, err := s.monitorManager.GetMonitor(currentUserID(t, s, "phil"), "scanner")
	require.Nil(t, err)
	require.Nil(t, s.monitorManager.MarkDown(mon.ID))

	rr = request(t, s, "POST", "/v1/heartbeat/scanner", "", basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	resp, _ := util.UnmarshalJSON[heartbeatResponse](io.NopCloser(rr.Body))
	require.Equal(t, monitor.StateUp, resp.State)
	require.Equal(t, monitor.StateDown, resp.PrevState)

	// publishMonitorAlert runs in a goroutine; give it a moment
	waitFor(t, func() bool {
		msgs, err := s.messageCache.Messages("alerts", model.SinceAllMessages, false)
		require.Nil(t, err)
		for _, m := range msgs {
			if m.Title == "Monitor scanner is UP" {
				return true
			}
		}
		return false
	})
}

func TestMonitor_CheckMonitors_UpToDown_PublishesAlert(t *testing.T) {
	s := newTestServer(t, newTestConfigWithMonitor(t))
	defer s.closeDatabases()
	addUser(t, s, "phil")
	rr := request(t, s, "POST", "/v1/monitors", `{"key":"scanner","period":30,"grace":10,"alert_topic":"alerts"}`, basicAuth("phil"))
	require.Equal(t, 200, rr.Code)
	rr = request(t, s, "POST", "/v1/heartbeat/scanner", "", basicAuth("phil"))
	require.Equal(t, 200, rr.Code)

	// Backdate last_seen_at so the monitor is stale on the next check
	userID := currentUserID(t, s, "phil")
	mon, err := s.monitorManager.GetMonitor(userID, "scanner")
	require.Nil(t, err)
	require.Nil(t, s.monitorManager.SetLastSeenAt(mon.ID, time.Now().Unix()-1000))

	require.Nil(t, s.checkMonitors())

	got, err := s.monitorManager.GetMonitor(userID, "scanner")
	require.Nil(t, err)
	require.Equal(t, monitor.StateDown, got.State)

	waitFor(t, func() bool {
		msgs, err := s.messageCache.Messages("alerts", model.SinceAllMessages, false)
		require.Nil(t, err)
		for _, m := range msgs {
			if m.Title == "Monitor scanner is DOWN" {
				return true
			}
		}
		return false
	})
}

func currentUserID(t *testing.T, s *Server, name string) string {
	u, err := s.userManager.User(name)
	require.Nil(t, err)
	return u.ID
}
