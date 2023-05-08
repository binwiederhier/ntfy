package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	twilioMessageEndpoint     = "Messages.json"
	twilioMessageFooterFormat = "This message was sent by %s via %s"
	twilioCallEndpoint        = "Calls.json"
	twilioCallFormat          = `
<Response>
	<Pause length="1"/>
	<Say>You have a message from notify on topic %s. Message:</Say>
	<Pause length="1"/>
	<Say>%s</Say>
	<Pause length="1"/>
	<Say>End message.</Say>
	<Pause length="1"/>
	<Say>%s</Say>
	<Pause length="1"/>
</Response>`
)

func (s *Server) sendSMS(v *visitor, r *http.Request, m *message, to string) {
	body := fmt.Sprintf("%s\n\n--\n%s", m.Message, s.messageFooter(v.User(), m))
	data := url.Values{}
	data.Set("From", s.config.TwilioFromNumber)
	data.Set("To", to)
	data.Set("Body", body)
	s.performTwilioRequest(v, r, m, metricSMSSentSuccess, metricSMSSentFailure, twilioMessageEndpoint, to, body, data)
}

func (s *Server) callPhone(v *visitor, r *http.Request, m *message, to string) {
	body := fmt.Sprintf(twilioCallFormat, xmlEscapeText(m.Topic), xmlEscapeText(m.Message), xmlEscapeText(s.messageFooter(v.User(), m)))
	data := url.Values{}
	data.Set("From", s.config.TwilioFromNumber)
	data.Set("To", to)
	data.Set("Twiml", body)
	s.performTwilioRequest(v, r, m, metricCallsMadeSuccess, metricCallsMadeFailure, twilioCallEndpoint, to, body, data)
}

func (s *Server) performTwilioRequest(v *visitor, r *http.Request, m *message, msuccess, mfailure prometheus.Counter, endpoint, to, body string, data url.Values) {
	logContext := log.Context{
		"twilio_from": s.config.TwilioFromNumber,
		"twilio_to":   to,
	}
	ev := logvrm(v, r, m).Tag(tagTwilio).Fields(logContext)
	if ev.IsTrace() {
		ev.Field("twilio_body", body).Trace("Sending Twilio request")
	} else if ev.IsDebug() {
		ev.Debug("Sending Twilio request")
	}
	response, err := s.performTwilioRequestInternal(endpoint, data)
	if err != nil {
		ev.
			Field("twilio_body", body).
			Field("twilio_response", response).
			Err(err).
			Warn("Error sending Twilio request")
		minc(mfailure)
		return
	}
	if ev.IsTrace() {
		ev.Field("twilio_response", response).Trace("Received successful Twilio response")
	} else if ev.IsDebug() {
		ev.Debug("Received successful Twilio response")
	}
	minc(msuccess)
}

func (s *Server) performTwilioRequestInternal(endpoint string, data url.Values) (string, error) {
	requestURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/%s", s.config.TwilioBaseURL, s.config.TwilioAccount, endpoint)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
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

func (s *Server) messageFooter(u *user.User, m *message) string { // u may be nil!
	topicURL := s.config.BaseURL + "/" + m.Topic
	sender := m.Sender.String()
	if u != nil {
		sender = fmt.Sprintf("%s (%s)", u.Name, m.Sender)
	}
	return fmt.Sprintf(twilioMessageFooterFormat, sender, util.ShortTopicURL(topicURL))
}

func xmlEscapeText(text string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(text))
	return buf.String()
}
