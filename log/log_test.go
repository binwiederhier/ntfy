package log_test

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"heckel.io/ntfy/log"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()
	resetState()
	log.SetLevel(log.ErrorLevel) // For other modules!
	os.Exit(exitCode)
}

func TestLog_TagContextFieldFields(t *testing.T) {
	t.Cleanup(resetState)
	v := &fakeVisitor{
		UserID: "u_abc",
		IP:     "1.2.3.4",
	}
	var out bytes.Buffer
	log.SetOutput(&out)
	log.SetFormat(log.JSONFormat)
	log.SetLevelOverride("tag", "stripe", log.DebugLevel)

	log.
		Tag("mytag").
		Field("field2", 123).
		Field("field1", "value1").
		Time(time.Unix(123, 0)).
		Info("hi there %s", "phil")
	log.
		Tag("not-stripe").
		Debug("this message will not appear")
	log.
		With(v).
		Fields(log.Context{
			"stripe_customer_id":     "acct_123",
			"stripe_subscription_id": "sub_123",
		}).
		Tag("stripe").
		Err(http.ErrHandlerTimeout).
		Time(time.Unix(456, 0)).
		Debug("Subscription status %s", "active")

	expected := `{"time":123000,"level":"INFO","message":"hi there phil","field1":"value1","field2":123,"tag":"mytag"}
{"time":456000,"level":"DEBUG","message":"Subscription status active","error":"http: Handler timeout","stripe_customer_id":"acct_123","stripe_subscription_id":"sub_123","tag":"stripe","user_id":"u_abc","visitor_ip":"1.2.3.4"}
`
	require.Equal(t, expected, out.String())
}

type fakeVisitor struct {
	UserID string
	IP     string
}

func (v *fakeVisitor) Context() log.Context {
	return map[string]any{
		"user_id":    v.UserID,
		"visitor_ip": v.IP,
	}
}

func resetState() {
	log.SetLevel(log.DefaultLevel)
	log.SetFormat(log.DefaultFormat)
	log.SetOutput(log.DefaultOutput)
	log.ResetLevelOverrides()
}
