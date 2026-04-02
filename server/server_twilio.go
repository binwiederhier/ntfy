package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/user"
	"heckel.io/ntfy/v2/util"
)

// defaultTwilioCallFormatTemplate is the default TwiML template used for Twilio calls.
// It can be overridden in the server configuration's twilio-call-format field.
//
// The format uses Go template syntax with the following fields:
// {{.Topic}}, {{.Title}}, {{.Message}}, {{.Priority}}, {{.Tags}}, {{.Sender}}
// String fields are automatically XML-escaped.
var defaultTwilioCallFormatTemplate = template.Must(template.New("twiml").Parse(`
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

// twilioCallData holds the data passed to the Twilio call format template
type twilioCallData struct {
	Topic    string
	Title    string
	Message  string
	Priority int
	Tags     []string
	Sender   string
}

// convertPhoneNumber checks if the given phone number is verified for the given user, and if so, returns the verified
// phone number. It also converts a boolean string ("yes", "1", "true") to the first verified phone number.
// If the user is anonymous, it will return an error.
func (s *Server) convertPhoneNumber(u *user.User, phoneNumber string) (string, *errHTTP) {
	if u == nil {
		return "", errHTTPBadRequestAnonymousCallsNotAllowed
	}
	phoneNumbers, err := s.userManager.PhoneNumbers(u.ID)
	if err != nil {
		return "", errHTTPInternalError
	} else if len(phoneNumbers) == 0 {
		return "", errHTTPBadRequestPhoneNumberNotVerified
	}
	if toBool(phoneNumber) {
		return phoneNumbers[0], nil
	} else if util.Contains(phoneNumbers, phoneNumber) {
		return phoneNumber, nil
	}
	for _, p := range phoneNumbers {
		if p == phoneNumber {
			return phoneNumber, nil
		}
	}
	return "", errHTTPBadRequestPhoneNumberNotVerified
}

// callPhone calls the Twilio API to make a phone call to the given phone number, using the given message.
// Failures will be logged, but not returned to the caller.
func (s *Server) callPhone(v *visitor, r *http.Request, m *model.Message, to string) {
	u, sender := v.User(), m.Sender.String()
	if u != nil {
		sender = u.Name
	}
	tmpl := defaultTwilioCallFormatTemplate
	if s.config.TwilioCallFormat != nil {
		tmpl = s.config.TwilioCallFormat
	}
	tags := make([]string, len(m.Tags))
	for i, tag := range m.Tags {
		tags[i] = xmlEscapeText(tag)
	}
	templateData := &twilioCallData{
		Topic:    xmlEscapeText(m.Topic),
		Title:    xmlEscapeText(m.Title),
		Message:  xmlEscapeText(m.Message),
		Priority: m.Priority,
		Tags:     tags,
		Sender:   xmlEscapeText(sender),
	}
	var bodyBuf bytes.Buffer
	if err := tmpl.Execute(&bodyBuf, templateData); err != nil {
		logvrm(v, r, m).Tag(tagTwilio).Err(err).Warn("Error executing Twilio call format template")
		minc(metricCallsMadeFailure)
		return
	}
	body := bodyBuf.String()
	data := url.Values{}
	data.Set("From", s.config.TwilioPhoneNumber)
	data.Set("To", to)
	data.Set("Twiml", body)
	ev := logvrm(v, r, m).Tag(tagTwilio).Field("twilio_to", to).FieldIf("twilio_body", body, log.TraceLevel).Debug("Sending Twilio request")
	response, err := s.callPhoneInternal(data)
	if err != nil {
		ev.Field("twilio_response", response).Err(err).Warn("Error sending Twilio request")
		minc(metricCallsMadeFailure)
		return
	}
	ev.FieldIf("twilio_response", response, log.TraceLevel).Debug("Received successful Twilio response")
	minc(metricCallsMadeSuccess)
}

func (s *Server) callPhoneInternal(data url.Values) (string, error) {
	requestURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Calls.json", s.config.TwilioCallsBaseURL, s.config.TwilioAccount)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ntfy/"+s.config.BuildVersion)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(response), nil
}

func (s *Server) verifyPhoneNumber(v *visitor, r *http.Request, phoneNumber, channel string) error {
	ev := logvr(v, r).Tag(tagTwilio).Field("twilio_to", phoneNumber).Field("twilio_channel", channel).Debug("Sending phone verification")
	data := url.Values{}
	data.Set("To", phoneNumber)
	data.Set("Channel", channel)
	requestURL := fmt.Sprintf("%s/v2/Services/%s/Verifications", s.config.TwilioVerifyBaseURL, s.config.TwilioVerifyService)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ntfy/"+s.config.BuildVersion)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		ev.Err(err).Warn("Error sending Twilio phone verification request")
		return err
	}
	ev.FieldIf("twilio_response", string(response), log.TraceLevel).Debug("Received Twilio phone verification response")
	return nil
}

func (s *Server) verifyPhoneNumberCheck(v *visitor, r *http.Request, phoneNumber, code string) error {
	ev := logvr(v, r).Tag(tagTwilio).Field("twilio_to", phoneNumber).Debug("Checking phone verification")
	data := url.Values{}
	data.Set("To", phoneNumber)
	data.Set("Code", code)
	requestURL := fmt.Sprintf("%s/v2/Services/%s/VerificationCheck", s.config.TwilioVerifyBaseURL, s.config.TwilioVerifyService)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ntfy/"+s.config.BuildVersion)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	} else if resp.StatusCode != http.StatusOK {
		if ev.IsTrace() {
			response, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			ev.Field("twilio_response", string(response))
		}
		ev.Warn("Twilio phone verification failed with status code %d", resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			return errHTTPGonePhoneVerificationExpired
		}
		return errHTTPInternalError
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if ev.IsTrace() {
		ev.Field("twilio_response", string(response)).Trace("Received successful Twilio phone verification response")
	} else if ev.IsDebug() {
		ev.Debug("Received successful Twilio phone verification response")
	}
	return nil
}

func xmlEscapeText(text string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(text))
	return buf.String()
}
