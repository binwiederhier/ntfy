package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"heckel.io/ntfy/log"
	"heckel.io/ntfy/user"
	"heckel.io/ntfy/util"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	twilioCallFormat = `
<Response>
	<Pause length="1"/>
	<Say loop="3">
		You have a message from notify on topic %s. Message:
		<break time="1s"/>
		%s
		<break time="1s"/>
		End of message.
		<break time="1s"/>
		This message was sent by user %s. It will be repeated three times.
		To unsubscribe from calls like this, remove your phone number in the notify web app.
		<break time="3s"/>
	</Say>
	<Say>Goodbye.</Say>
</Response>`
)

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
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
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
	req.Header.Set("Authorization", util.BasicAuth(s.config.TwilioAccount, s.config.TwilioAuthToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
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
