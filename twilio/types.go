package twilio

import (
	"bytes"
	"encoding/xml"
	"errors"
	"text/template"
)

var (
	// ErrVerificationExpired is returned by CheckVerify if the verification code has
	// expired, or if it never existed in the first place
	ErrVerificationExpired = errors.New("phone number verification expired or does not exist")

	// ErrVerificationCodeInvalid is returned by CheckVerify if the verification code is wrong
	ErrVerificationCodeInvalid = errors.New("phone number verification code invalid")
)

// Config holds the Twilio configuration for the client
type Config struct {
	Account       string             // Twilio account SID, e.g. AC123...
	AuthToken     string             // Twilio auth token
	PhoneNumber   string             // Twilio number to use for outgoing calls
	CallsBaseURL  string             // Base URL of the Twilio Calls API
	VerifyBaseURL string             // Base URL of the Twilio Verify API
	VerifyService string             // Twilio Verify service ID, e.g. VA123...
	CallFormat    *template.Template // TwiML template for calls; if nil, defaultCallFormatTemplate is used
	BuildVersion  string             // ntfy version, used for the User-Agent header
}

// defaultCallFormatTemplate is the default TwiML template used for Twilio calls.
// It can be overridden in the server configuration's twilio-call-format field.
//
// The format uses Go template syntax with the following fields:
// {{.Topic}}, {{.Title}}, {{.Message}}, {{.Priority}}, {{.Tags}}, {{.Sender}}
// String fields are automatically XML-escaped.
var defaultCallFormatTemplate = template.Must(template.New("twiml").Parse(`
<Response>
	<Pause length="1"/>
	<Say loop="3">
		You have a message from notify on topic {{.Topic}}. Message:
		<break time="1s"/>
		{{.Message}}
		<break time="1s"/>
		End of message.
		<break time="1s"/>
		This message was sent by user {{.Sender}}. It will be repeated three times.
		To unsubscribe from calls like this, remove your phone number in the notify web app.
		<break time="3s"/>
	</Say>
	<Say>Goodbye.</Say>
</Response>`))

// verificationCheckResponse is the relevant part of the Twilio Verify VerificationCheck response
type verificationCheckResponse struct {
	Status string `json:"status"` // "approved" if the code is correct, "pending" otherwise
}

// CallData holds the data passed to the Twilio call format template. String fields are
// XML-escaped before the template is executed, so callers pass them unescaped.
type CallData struct {
	Topic    string
	Title    string
	Message  string
	Priority int
	Tags     []string
	Sender   string
}

// escaped returns a copy of the call data with all string fields XML-escaped
func (d *CallData) escaped() *CallData {
	tags := make([]string, len(d.Tags))
	for i, tag := range d.Tags {
		tags[i] = xmlEscapeText(tag)
	}
	return &CallData{
		Topic:    xmlEscapeText(d.Topic),
		Title:    xmlEscapeText(d.Title),
		Message:  xmlEscapeText(d.Message),
		Priority: d.Priority,
		Tags:     tags,
		Sender:   xmlEscapeText(d.Sender),
	}
}

func xmlEscapeText(text string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(text))
	return buf.String()
}
