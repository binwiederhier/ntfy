package log

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	exitCode := m.Run()
	resetState()
	SetLevel(ErrorLevel) // For other modules!
	os.Exit(exitCode)
}

func TestLog_TagContextFieldFields(t *testing.T) {
	t.Cleanup(resetState)
	v := &fakeVisitor{
		UserID: "u_abc",
		IP:     "1.2.3.4",
	}
	err := &fakeError{
		Code:    123,
		Message: "some error",
	}
	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)
	SetLevelOverride("tag", "stripe", DebugLevel)
	SetLevelOverride("number", "5", DebugLevel)

	Tag("mytag").
		Field("field2", 123).
		Field("field1", "value1").
		Time(time.Unix(123, 999000000).UTC()).
		Info("hi there %s", "phil")

	Tag("not-stripe").
		Debug("this message will not appear")

	With(v).
		Fields(Context{
			"stripe_customer_id":     "acct_123",
			"stripe_subscription_id": "sub_123",
		}).
		Tag("stripe").
		Err(err).
		Time(time.Unix(456, 123000000).UTC()).
		Debug("Subscription status %s", "active")

	Field("number", 5).
		Time(time.Unix(777, 001000000).UTC()).
		Debug("The number 5 is an int, but the level override is a string")

	expected := `{"time":"1970-01-01T00:02:03.999Z","level":"INFO","message":"hi there phil","field1":"value1","field2":123,"tag":"mytag"}
{"time":"1970-01-01T00:07:36.123Z","level":"DEBUG","message":"Subscription status active","error":"some error","error_code":123,"stripe_customer_id":"acct_123","stripe_subscription_id":"sub_123","tag":"stripe","user_id":"u_abc","visitor_ip":"1.2.3.4"}
{"time":"1970-01-01T00:12:57Z","level":"DEBUG","message":"The number 5 is an int, but the level override is a string","number":5}
`
	require.Equal(t, expected, out.String())
}

func TestLog_NoAllocIfNotPrinted(t *testing.T) {
	t.Cleanup(resetState)
	v := &fakeVisitor{
		UserID: "u_abc",
		IP:     "1.2.3.4",
	}

	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)

	// Do not log, do not call contexters (because global level is INFO)
	v.contextCalled = false
	ev := With(v)
	ev.Debug("some message")
	require.False(t, v.contextCalled)
	require.Equal(t, "", ev.Timestamp)
	require.Equal(t, Level(0), ev.Level)
	require.Equal(t, "", ev.Message)
	require.Nil(t, ev.fields)

	// Logged because info level, contexters called
	v.contextCalled = false
	ev = With(v).Time(time.Unix(1111, 0).UTC())
	ev.Info("some message")
	require.True(t, v.contextCalled)
	require.NotNil(t, ev.fields)
	require.Equal(t, "1.2.3.4", ev.fields["visitor_ip"])

	// Not logged, but contexters called, because overrides exist
	SetLevel(DebugLevel)
	SetLevelOverride("tag", "overridetag", TraceLevel)
	v.contextCalled = false
	ev = Tag("sometag").Field("field", "value").With(v).Time(time.Unix(123, 0).UTC())
	ev.Trace("some debug message")
	require.True(t, v.contextCalled) // If there are overrides, we must call the context to determine the filter fields
	require.Equal(t, "", ev.Timestamp)
	require.Equal(t, Level(0), ev.Level)
	require.Equal(t, "", ev.Message)
	require.Equal(t, 4, len(ev.fields))
	require.Equal(t, "value", ev.fields["field"])
	require.Equal(t, "sometag", ev.fields["tag"])

	// Logged because of override tag, and contexters called
	v.contextCalled = false
	ev = Tag("overridetag").Field("field", "value").With(v).Time(time.Unix(123, 0).UTC())
	ev.Trace("some trace message")
	require.True(t, v.contextCalled)
	require.Equal(t, "1970-01-01T00:02:03Z", ev.Timestamp)
	require.Equal(t, TraceLevel, ev.Level)
	require.Equal(t, "some trace message", ev.Message)

	// Logged because of field override, and contexters called
	ResetLevelOverrides()
	SetLevelOverride("visitor_ip", "1.2.3.4", TraceLevel)
	v.contextCalled = false
	ev = With(v).Time(time.Unix(124, 0).UTC())
	ev.Trace("some trace message with override")
	require.True(t, v.contextCalled)
	require.Equal(t, "1970-01-01T00:02:04Z", ev.Timestamp)
	require.Equal(t, TraceLevel, ev.Level)
	require.Equal(t, "some trace message with override", ev.Message)

	expected := `{"time":"1970-01-01T00:18:31Z","level":"INFO","message":"some message","user_id":"u_abc","visitor_ip":"1.2.3.4"}
{"time":"1970-01-01T00:02:03Z","level":"TRACE","message":"some trace message","field":"value","tag":"overridetag","user_id":"u_abc","visitor_ip":"1.2.3.4"}
{"time":"1970-01-01T00:02:04Z","level":"TRACE","message":"some trace message with override","user_id":"u_abc","visitor_ip":"1.2.3.4"}
`
	require.Equal(t, expected, out.String())
}

func TestLog_Timing(t *testing.T) {
	t.Cleanup(resetState)

	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)

	Timing(func() { time.Sleep(300 * time.Millisecond) }).
		Time(time.Unix(12, 0).UTC()).
		Info("A thing that takes a while")

	var ev struct {
		TimeTakenMs int64 `json:"time_taken_ms"`
	}
	require.Nil(t, json.Unmarshal(out.Bytes(), &ev))
	require.True(t, ev.TimeTakenMs >= 300)
	require.Contains(t, out.String(), `{"time":"1970-01-01T00:00:12Z","level":"INFO","message":"A thing that takes a while","time_taken_ms":`)
}

func TestLog_LevelOverrideAny(t *testing.T) {
	t.Cleanup(resetState)

	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)
	SetLevelOverride("this_one", "", DebugLevel)
	SetLevelOverride("time_taken_ms", "", TraceLevel)

	Time(time.Unix(11, 0).UTC()).Field("this_one", "11").Debug("this is logged")
	Time(time.Unix(12, 0).UTC()).Field("not_this", "11").Debug("this is not logged")
	Time(time.Unix(13, 0).UTC()).Field("this_too", "11").Info("this is also logged")
	Time(time.Unix(14, 0).UTC()).Field("time_taken_ms", 0).Info("this is also logged")

	expected := `{"time":"1970-01-01T00:00:11Z","level":"DEBUG","message":"this is logged","this_one":"11"}
{"time":"1970-01-01T00:00:13Z","level":"INFO","message":"this is also logged","this_too":"11"}
{"time":"1970-01-01T00:00:14Z","level":"INFO","message":"this is also logged","time_taken_ms":0}
`
	require.Equal(t, expected, out.String())
	require.False(t, IsFile())
	require.Equal(t, "", File())
}

func TestLog_LevelOverride_ManyOnSameField(t *testing.T) {
	t.Cleanup(resetState)

	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)
	SetLevelOverride("tag", "manager", DebugLevel)
	SetLevelOverride("tag", "publish", DebugLevel)

	Time(time.Unix(11, 0).UTC()).Field("tag", "manager").Debug("this is logged")
	Time(time.Unix(12, 0).UTC()).Field("tag", "no-match").Debug("this is not logged")
	Time(time.Unix(13, 0).UTC()).Field("tag", "publish").Info("this is also logged")

	expected := `{"time":"1970-01-01T00:00:11Z","level":"DEBUG","message":"this is logged","tag":"manager"}
{"time":"1970-01-01T00:00:13Z","level":"INFO","message":"this is also logged","tag":"publish"}
`
	require.Equal(t, expected, out.String())
	require.False(t, IsFile())
	require.Equal(t, "", File())
}

func TestLog_UsingStdLogger_JSON(t *testing.T) {
	t.Cleanup(resetState)

	var out bytes.Buffer
	SetOutput(&out)
	SetFormat(JSONFormat)

	log.Println("Some other library is using the standard Go logger")
	require.Contains(t, out.String(), `,"level":"INFO","message":"Some other library is using the standard Go logger","tag":"stdlog"}`+"\n")
}

func TestLog_UsingStdLogger_Text(t *testing.T) {
	t.Cleanup(resetState)

	var out bytes.Buffer
	SetOutput(&out)

	log.Println("Some other library is using the standard Go logger")
	require.Contains(t, out.String(), `Some other library is using the standard Go logger`+"\n")
	require.NotContains(t, out.String(), `{`)
}

func TestLog_File(t *testing.T) {
	t.Cleanup(resetState)

	logfile := filepath.Join(t.TempDir(), "ntfy.log")
	f, err := os.OpenFile(logfile, os.O_CREATE|os.O_WRONLY, 0600)
	require.Nil(t, err)
	SetOutput(f)
	SetFormat(JSONFormat)
	require.True(t, IsFile())
	require.Equal(t, logfile, File())

	Time(time.Unix(11, 0).UTC()).Field("this_one", "11").Info("this is logged")
	require.Nil(t, f.Close())

	f, err = os.Open(logfile)
	require.Nil(t, err)
	contents, err := io.ReadAll(f)
	require.Nil(t, err)
	require.Equal(t, `{"time":"1970-01-01T00:00:11Z","level":"INFO","message":"this is logged","this_one":"11"}`+"\n", string(contents))
}

type fakeError struct {
	Code    int
	Message string
}

func (e fakeError) Error() string {
	return e.Message
}

func (e fakeError) Context() Context {
	return Context{
		"error":      e.Message,
		"error_code": e.Code,
	}
}

type fakeVisitor struct {
	UserID        string
	IP            string
	contextCalled bool
}

func (v *fakeVisitor) Context() Context {
	v.contextCalled = true
	return Context{
		"user_id":    v.UserID,
		"visitor_ip": v.IP,
	}
}

func resetState() {
	SetLevel(DefaultLevel)
	SetFormat(DefaultFormat)
	SetOutput(DefaultOutput)
	ResetLevelOverrides()
}
