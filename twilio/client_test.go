package twilio

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
)

func TestClient_Call_Success(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/2010-04-01/Accounts/AC1234567890/Calls.json", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Equal(t, "ntfy/1.2.3", r.Header.Get("User-Agent"))
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Nil(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there", Sender: "phil"}))

	form, err := url.ParseQuery(body)
	require.Nil(t, err)
	require.Equal(t, "+1234567890", form.Get("From"))
	require.Equal(t, "+11122233344", form.Get("To"))
	require.Contains(t, form.Get("Twiml"), "You have a message from notify on topic mytopic. Message:")
	require.Contains(t, form.Get("Twiml"), "hi there")
	require.Contains(t, form.Get("Twiml"), "This message was sent by user phil.")
}

// TestClient_Call_EscapesXML ensures that user-controlled fields cannot break out of the
// TwiML document, i.e. that a message containing XML is escaped rather than interpreted
func TestClient_Call_EscapesXML(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	data := &CallData{
		Topic:   "mytopic",
		Message: `</Say><Say>evil</Say>`,
		Tags:    []string{"<tag>"},
		Sender:  `phil & "friends"`,
	}
	require.Nil(t, c.Call("+11122233344", data))

	form, err := url.ParseQuery(body)
	require.Nil(t, err)
	twiml := form.Get("Twiml")
	require.NotContains(t, twiml, "<Say>evil</Say>")
	require.Contains(t, twiml, "&lt;/Say&gt;&lt;Say&gt;evil&lt;/Say&gt;")
	require.Contains(t, twiml, "phil &amp; &#34;friends&#34;")
	// The caller's data must not be modified by the escaping
	require.Equal(t, `</Say><Say>evil</Say>`, data.Message)
	require.Equal(t, []string{"<tag>"}, data.Tags)
}

func TestClient_Call_CustomCallFormat(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
	}))
	defer server.Close()

	conf := testConfig(server.URL)
	conf.CallFormat = template.Must(template.New("twiml").Parse(`<Response><Say>{{.Message}} von {{.Sender}}</Say></Response>`))
	c := NewClient(conf)
	require.Nil(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there", Sender: "phil"}))

	form, err := url.ParseQuery(body)
	require.Nil(t, err)
	require.Equal(t, "<Response><Say>hi there von phil</Say></Response>", form.Get("Twiml"))
}

// TestClient_Call_RendersAllFields covers the fields that the default TwiML template does not
// use, i.e. Title, Priority and Tags, including the escaping of every tag
func TestClient_Call_RendersAllFields(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
	}))
	defer server.Close()

	conf := testConfig(server.URL)
	conf.CallFormat = template.Must(template.New("twiml").Parse(`<Response><Say>{{.Title}}/{{.Priority}}{{range .Tags}}/{{.}}{{end}}</Say></Response>`))
	c := NewClient(conf)
	data := &CallData{
		Topic:    "mytopic",
		Title:    "a <title>",
		Priority: 5,
		Tags:     []string{"<one>", "two & three"},
	}
	require.Nil(t, c.Call("+11122233344", data))

	form, err := url.ParseQuery(body)
	require.Nil(t, err)
	require.Equal(t, "<Response><Say>a &lt;title&gt;/5/&lt;one&gt;/two &amp; three</Say></Response>", form.Get("Twiml"))
}

func TestClient_Call_TemplateError(t *testing.T) {
	conf := testConfig("http://dummy.invalid")
	conf.CallFormat = template.Must(template.New("twiml").Parse(`{{.DoesNotExist}}`))
	c := NewClient(conf)
	require.Error(t, c.Call("+11122233344", &CallData{Topic: "mytopic"}))
}

// TestClient_Call_Created ensures that a 201 Created is treated as a success. The Twilio Calls
// API returns 201 (not 200) for a successfully queued call, so this must not be an error.
func TestClient_Call_Created(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"queued"}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Nil(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there"}))
}

// TestClient_Call_TwilioError ensures that a non-2xx response from Twilio is returned as an
// error, so that the server counts it as a failure instead of a success. Twilio rejects calls
// with a 4xx, e.g. for an invalid "To" number, or when the account is out of funds.
func TestClient_Call_TwilioError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":21211,"message":"Invalid 'To' Phone Number: +invalid"}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	err := c.Call("+invalid", &CallData{Topic: "mytopic", Message: "hi there"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestClient_Call_TwilioServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Error(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there"}))
}

// TestClient_Call_TransportError ensures that a call to an unreachable Twilio API returns an
// error, so that the server can count it as a failure
func TestClient_Call_TransportError(t *testing.T) {
	c := NewClient(testConfig(closedServerURL(t)))
	require.Error(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there"}))
}

func TestClient_Call_InvalidBaseURL(t *testing.T) {
	c := NewClient(testConfig("://invalid"))
	require.Error(t, c.Call("+11122233344", &CallData{Topic: "mytopic", Message: "hi there"}))
}

// TestClient_Verify_Created ensures that a 201 Created is treated as a success. The Twilio
// Verify API returns 201 (not 200) when it creates a verification, so this must not be an error.
func TestClient_Verify_Created(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"pending"}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Nil(t, c.Verify("+12223334444", "sms"))
}

// TestClient_Verify_TwilioError ensures that a non-2xx response from Twilio is returned as an
// error. Without this, no SMS is sent, but the user is still told to check their phone.
func TestClient_Verify_TwilioError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"code":60200,"message":"Invalid parameter"}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	err := c.Verify("+12223334444", "sms")
	require.Error(t, err)
	require.Contains(t, err.Error(), "400")
}

func TestClient_Verify_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Error(t, c.Verify("+12223334444", "sms"))
}

func TestClient_Verify_TransportError(t *testing.T) {
	c := NewClient(testConfig(closedServerURL(t)))
	require.Error(t, c.Verify("+12223334444", "sms"))
}

func TestClient_CheckVerify_TransportError(t *testing.T) {
	c := NewClient(testConfig(closedServerURL(t)))
	err := c.CheckVerify("+12223334444", "123456")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVerificationExpired))
}

func TestClient_Verify_Success(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/Services/VA1234567890/Verifications", r.URL.Path)
		require.Equal(t, "Basic QUMxMjM0NTY3ODkwOkFBRUFBMTIzNDU2Nzg5MA==", r.Header.Get("Authorization"))
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Nil(t, c.Verify("+12223334444", "sms"))
	require.Equal(t, "Channel=sms&To=%2B12223334444", body)
}

func TestClient_CheckVerify_Success(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/Services/VA1234567890/VerificationCheck", r.URL.Path)
		b, err := io.ReadAll(r.Body)
		require.Nil(t, err)
		body = string(b)
		w.Write([]byte(`{"status":"approved","valid":true}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Nil(t, c.CheckVerify("+12223334444", "123456"))
	require.Equal(t, "Code=123456&To=%2B12223334444", body)
}

// TestClient_CheckVerify_WrongCode ensures that a wrong code is rejected. Twilio answers
// a wrong code with 200 OK and status "pending", so the status code alone is not enough.
func TestClient_CheckVerify_WrongCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"pending","valid":false}`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	err := c.CheckVerify("+12223334444", "000000")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrVerificationCodeInvalid))
}

func TestClient_CheckVerify_InvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	require.Error(t, c.CheckVerify("+12223334444", "123456"))
}

// TestClient_CheckVerify_Expired ensures that a 404 from the Twilio Verify API is
// mapped to ErrVerificationExpired, which the server turns into an HTTP 410
func TestClient_CheckVerify_Expired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	err := c.CheckVerify("+12223334444", "123456")
	require.True(t, errors.Is(err, ErrVerificationExpired))
}

func TestClient_CheckVerify_OtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := NewClient(testConfig(server.URL))
	err := c.CheckVerify("+12223334444", "123456")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrVerificationExpired))
}

// closedServerURL returns the URL of a server that is not listening anymore, to simulate an
// unreachable Twilio API
func closedServerURL(t *testing.T) string {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Should not be called")
	}))
	server.Close()
	return server.URL
}

func testConfig(baseURL string) *Config {
	return &Config{
		Account:       "AC1234567890",
		AuthToken:     "AAEAA1234567890",
		PhoneNumber:   "+1234567890",
		CallsBaseURL:  baseURL,
		VerifyBaseURL: baseURL,
		VerifyService: "VA1234567890",
		BuildVersion:  "1.2.3",
	}
}
