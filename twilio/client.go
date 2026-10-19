// Package twilio talks to the Twilio API to make phone calls (for the "Call" feature) and to
// verify phone numbers. It holds the Twilio configuration, so that this functionality is
// decoupled from the ntfy server.
package twilio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

const (
	tagTwilio                  = "twilio"
	verificationStatusApproved = "approved"
)

// Client is the Twilio API client
type Client struct {
	config *Config
}

// NewClient creates a new Twilio Client with the given config
func NewClient(config *Config) *Client {
	return &Client{config: config}
}

// Call calls the Twilio API to make a phone call to the given phone number, using the given data
func (c *Client) Call(to string, data *CallData) error {
	tmpl := defaultCallFormatTemplate
	if c.config.CallFormat != nil {
		tmpl = c.config.CallFormat
	}
	var bodyBuf bytes.Buffer
	if err := tmpl.Execute(&bodyBuf, data.escaped()); err != nil {
		log.Tag(tagTwilio).Err(err).Warn("Error executing Twilio call format template")
		return err
	}
	body := bodyBuf.String()
	form := url.Values{}
	form.Set("From", c.config.PhoneNumber)
	form.Set("To", to)
	form.Set("Twiml", body)
	ev := log.Tag(tagTwilio).
		Field("twilio_to", to).
		FieldIf("twilio_body", body, log.TraceLevel).
		Debug("Sending Twilio request")
	requestURL := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Calls.json", c.config.CallsBaseURL, c.config.Account)
	response, code, err := c.request(requestURL, form)
	if err != nil {
		ev.Field("twilio_response", response).Err(err).Warn("Error sending Twilio request")
		return err
	} else if !success(code) {
		// Twilio rejects calls with a 4xx, e.g. for an invalid phone number, or if the account
		// is out of funds. Without this check, a rejected call would be counted as a success.
		ev.Field("twilio_status", code).Field("twilio_response", response).Warn("Twilio call failed with status code %d", code)
		return fmt.Errorf("twilio call failed with status code %d", code)
	}
	ev.FieldIf("twilio_response", response, log.TraceLevel).Debug("Received successful Twilio response")
	return nil
}

// Verify calls the Twilio Verify API to send a verification code to the given phone
// number, via the given channel ("sms" or "call")
func (c *Client) Verify(phoneNumber, channel string) error {
	ev := log.Tag(tagTwilio).Field("twilio_to", phoneNumber).Field("twilio_channel", channel).Debug("Sending phone verification")
	form := url.Values{}
	form.Set("To", phoneNumber)
	form.Set("Channel", channel)
	requestURL := fmt.Sprintf("%s/v2/Services/%s/Verifications", c.config.VerifyBaseURL, c.config.VerifyService)
	response, code, err := c.request(requestURL, form)
	if err != nil {
		ev.Err(err).Warn("Error sending Twilio phone verification request")
		return err
	} else if !success(code) {
		// Without this check, a rejected verification would look like a success to the caller,
		// and the user would be told to wait for an SMS that was never sent.
		ev.Field("twilio_status", code).Field("twilio_response", response).Warn("Twilio phone verification request failed with status code %d", code)
		return fmt.Errorf("twilio phone verification request failed with status code %d", code)
	}
	ev.FieldIf("twilio_response", response, log.TraceLevel).Debug("Received Twilio phone verification response")
	return nil
}

// CheckVerify calls the Twilio Verify API to check the verification code for the given
// phone number. It returns ErrVerificationExpired if the code has expired or never existed.
func (c *Client) CheckVerify(phoneNumber, code string) error {
	ev := log.Tag(tagTwilio).Field("twilio_to", phoneNumber).Debug("Checking phone verification")
	form := url.Values{}
	form.Set("To", phoneNumber)
	form.Set("Code", code)
	requestURL := fmt.Sprintf("%s/v2/Services/%s/VerificationCheck", c.config.VerifyBaseURL, c.config.VerifyService)
	req, err := c.newRequest(requestURL, form)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if ev.IsTrace() {
			response, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			ev.Field("twilio_response", string(response))
		}
		ev.Warn("Twilio phone verification failed with status code %d", resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			return ErrVerificationExpired
		}
		return fmt.Errorf("twilio phone verification failed with status code %d", resp.StatusCode)
	}
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	// Twilio answers a wrong code with 200 OK too; only status "approved" means verified
	var check verificationCheckResponse
	if err := json.Unmarshal(response, &check); err != nil {
		ev.Field("twilio_response", string(response)).Err(err).Warn("Cannot parse Twilio phone verification response")
		return err
	} else if check.Status != verificationStatusApproved {
		ev.Field("twilio_verify_status", check.Status).Debug("Twilio phone verification code invalid")
		return ErrVerificationCodeInvalid
	}
	if ev.IsTrace() {
		ev.Field("twilio_response", string(response)).Trace("Received successful Twilio phone verification response")
	} else if ev.IsDebug() {
		ev.Debug("Received successful Twilio phone verification response")
	}
	return nil
}

// request POSTs the given form to the given Twilio API URL, and returns the raw response body
// and status code. It does not treat a non-2xx status code as an error; that is up to the
// caller. The response body is returned even if the request failed, so that it can be logged.
func (c *Client) request(requestURL string, form url.Values) (string, int, error) {
	req, err := c.newRequest(requestURL, form)
	if err != nil {
		return "", 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(response), resp.StatusCode, nil
}

// success reports whether the given HTTP status code indicates success. Note that the Twilio
// Calls API returns 201 Created (not 200 OK) for a successfully queued call.
func success(code int) bool {
	return code >= 200 && code <= 299
}

// newRequest creates a form-encoded POST request against the Twilio API, with the auth and
// User-Agent headers set
func (c *Client) newRequest(requestURL string, form url.Values) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ntfy/"+c.config.BuildVersion)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", util.BasicAuth(c.config.Account, c.config.AuthToken))
	return req, nil
}
