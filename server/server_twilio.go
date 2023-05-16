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
	twilioCallEndpoint = "Calls.json"
	twilioCallFormat   = `
<Response>
	<Pause length="1"/>
	<Say loop="5">
		You have a notification from notify on topic %s. Message:
		<break time="1s"/>
		%s
		<break time="1s"/>
		End message.
		<break time="1s"/>
		This message was sent by user %s. It will be repeated up to five times.
		<break time="3s"/>
	</Say>
	<Say>Goodbye.</Say>
</Response>`
)

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

func (s *Server) callPhone(v *visitor, r *http.Request, m *message, to string) {
	u, sender := v.User(), m.Sender.String()
	if u != nil {
		sender = u.Name
	}
	body := fmt.Sprintf(twilioCallFormat, xmlEscapeText(m.Topic), xmlEscapeText(m.Message), xmlEscapeText(sender))
	data := url.Values{}
	data.Set("From", s.config.TwilioFromNumber)
	data.Set("To", to)
	data.Set("Twiml", body)
	s.twilioMessagingRequest(v, r, m, metricCallsMadeSuccess, metricCallsMadeFailure, twilioCallEndpoint, to, body, data)
}

func (s *Server) verifyPhone(v *visitor, r *http.Request, phoneNumber string) error {
	logvr(v, r).Tag(tagTwilio).Field("twilio_to", phoneNumber).Debug("Sending phone verification")
	data := url.Values{}
	data.Set("To", phoneNumber)
	data.Set("Channel", "sms")
	requestURL := fmt.Sprintf("%s/v2/Services/%s/Verifications", s.config.TwilioVerifyBaseURL, s.config.TwilioVerifyService)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	response, err := io.ReadAll(resp.Body)
	ev := logvr(v, r).Tag(tagTwilio)
	if err != nil {
		ev.Err(err).Warn("Error sending Twilio phone verification request")
		return err
	}
	if ev.IsTrace() {
		ev.Field("twilio_response", string(response)).Trace("Received successful Twilio phone verification response")
	} else if ev.IsDebug() {
		ev.Debug("Received successful Twilio phone verification response")
	}
	return nil
}

func (s *Server) checkVerifyPhone(v *visitor, r *http.Request, phoneNumber, code string) error {
	logvr(v, r).Tag(tagTwilio).Field("twilio_to", phoneNumber).Debug("Checking phone verification")
	data := url.Values{}
	data.Set("To", phoneNumber)
	data.Set("Code", code)
	requestURL := fmt.Sprintf("%s/v2/Services/%s/VerificationCheck", s.config.TwilioVerifyBaseURL, s.config.TwilioVerifyService)
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	log.Fields(httpContext(req)).Field("http_body", data.Encode()).Info("Twilio call")
	ev := logvr(v, r).
		Tag(tagTwilio).
		Field("twilio_to", phoneNumber)
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

func (s *Server) twilioMessagingRequest(v *visitor, r *http.Request, m *message, msuccess, mfailure prometheus.Counter, endpoint, to, body string, data url.Values) {
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
	response, err := s.performTwilioMessagingRequestInternal(endpoint, data)
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

func (s *Server) performTwilioMessagingRequestInternal(endpoint string, data url.Values) (string, error) {
	requestURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/%s", s.config.TwilioMessagingBaseURL, s.config.TwilioAccount, endpoint)
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

func xmlEscapeText(text string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(text))
	return buf.String()
}
