package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"github.com/microcosm-cc/bluemonday"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"regexp"
	"strings"
	"sync"
)

var (
	errInvalidDomain          = errors.New("invalid domain")
	errInvalidAddress         = errors.New("invalid address")
	errInvalidTopic           = errors.New("invalid topic")
	errTooManyRecipients      = errors.New("too many recipients")
	errMultipartNestedTooDeep = errors.New("multipart message nested too deep")
	errUnsupportedContentType = errors.New("unsupported content type")
)

const (
	maxMultipartDepth = 2
)

// smtpBackend implements SMTP server methods.
type smtpBackend struct {
	config  *Config
	handler func(http.ResponseWriter, *http.Request)
	success int64
	failure int64
	mu      sync.Mutex
}

var _ smtp.Backend = (*smtpBackend)(nil)
var _ smtp.Session = (*smtpSession)(nil)

func newMailBackend(conf *Config, handler func(http.ResponseWriter, *http.Request)) *smtpBackend {
	return &smtpBackend{
		config:  conf,
		handler: handler,
	}
}

func (b *smtpBackend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	logem(conn).Debug("Incoming mail")
	return &smtpSession{backend: b, conn: conn}, nil
}

func (b *smtpBackend) Counts() (total int64, success int64, failure int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.success + b.failure, b.success, b.failure
}

// smtpSession is returned after EHLO.
type smtpSession struct {
	backend *smtpBackend
	conn    *smtp.Conn
	topic   string
	token   string
	mu      sync.Mutex
}

func (s *smtpSession) AuthPlain(username, _ string) error {
	logem(s.conn).Field("smtp_username", username).Debug("AUTH PLAIN (with username %s)", username)
	return nil
}

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	logem(s.conn).Field("smtp_mail_from", from).Debug("MAIL FROM: %s", from)
	return nil
}

func (s *smtpSession) Rcpt(to string) error {
	logem(s.conn).Field("smtp_rcpt_to", to).Debug("RCPT TO: %s", to)
	return s.withFailCount(func() error {
		token := ""
		conf := s.backend.config
		addressList, err := mail.ParseAddressList(to)
		if err != nil {
			return err
		} else if len(addressList) != 1 {
			return errTooManyRecipients
		}
		to = addressList[0].Address
		if !strings.HasSuffix(to, "@"+conf.SMTPServerDomain) {
			return errInvalidDomain
		}
		// Remove @ntfy.sh from end of email
		to = strings.TrimSuffix(to, "@"+conf.SMTPServerDomain)
		if conf.SMTPServerAddrPrefix != "" {
			if !strings.HasPrefix(to, conf.SMTPServerAddrPrefix) {
				return errInvalidAddress
			}
			// remove ntfy- from beginning of email
			to = strings.TrimPrefix(to, conf.SMTPServerAddrPrefix)
		}
		// If email contains token, split topic and token
		if strings.Contains(to, "+") {
			parts := strings.Split(to, "+")
			to = parts[0]
			token = parts[1]
		}
		if !topicRegex.MatchString(to) {
			return errInvalidTopic
		}
		s.mu.Lock()
		s.topic = to
		s.token = token
		s.mu.Unlock()
		return nil
	})
}

func (s *smtpSession) Data(r io.Reader) error {
	return s.withFailCount(func() error {
		conf := s.backend.config
		b, err := io.ReadAll(r) // Protected by MaxMessageBytes
		if err != nil {
			return err
		}
		ev := logem(s.conn)
		if ev.IsTrace() {
			ev.Field("smtp_data", string(b)).Trace("DATA")
		} else if ev.IsDebug() {
			ev.Field("smtp_data_len", len(b)).Debug("DATA")
		}
		msg, err := mail.ReadMessage(bytes.NewReader(b))
		if err != nil {
			return err
		}
		body, err := readMailBody(msg.Body, msg.Header)
		if err != nil {
			return err
		}
		body = strings.TrimSpace(body)
		if len(body) > conf.MessageLimit {
			body = body[:conf.MessageLimit]
		}
		m := newDefaultMessage(s.topic, body)
		subject := strings.TrimSpace(msg.Header.Get("Subject"))
		if subject != "" {
			dec := mime.WordDecoder{}
			subject, err := dec.DecodeHeader(subject)
			if err != nil {
				return err
			}
			m.Title = subject
		}
		if m.Title != "" && m.Message == "" {
			m.Message = m.Title // Flip them, this makes more sense
			m.Title = ""
		}
		if err := s.publishMessage(m); err != nil {
			return err
		}
		s.backend.mu.Lock()
		s.backend.success++
		s.backend.mu.Unlock()
		minc(metricEmailsReceivedSuccess)
		return nil
	})
}

func (s *smtpSession) publishMessage(m *message) error {
	// Extract remote address (for rate limiting)
	remoteAddr, _, err := net.SplitHostPort(s.conn.Conn().RemoteAddr().String())
	if err != nil {
		remoteAddr = s.conn.Conn().RemoteAddr().String()
	}
	// Call HTTP handler with fake HTTP request
	url := fmt.Sprintf("%s/%s", s.backend.config.BaseURL, m.Topic)
	req, err := http.NewRequest("POST", url, strings.NewReader(m.Message))
	req.RequestURI = "/" + m.Topic // just for the logs
	req.RemoteAddr = remoteAddr    // rate limiting!!
	req.Header.Set("X-Forwarded-For", remoteAddr)
	if err != nil {
		return err
	}
	if m.Title != "" {
		req.Header.Set("Title", m.Title)
	}
	if s.token != "" {
		req.Header.Add("Authorization", "Bearer "+s.token)
	}
	rr := httptest.NewRecorder()
	s.backend.handler(rr, req)
	if rr.Code != http.StatusOK {
		return errors.New("error: " + rr.Body.String())
	}
	return nil
}

func (s *smtpSession) Reset() {
	s.mu.Lock()
	s.topic = ""
	s.mu.Unlock()
}

func (s *smtpSession) Logout() error {
	return nil
}

func (s *smtpSession) withFailCount(fn func() error) error {
	err := fn()
	s.backend.mu.Lock()
	defer s.backend.mu.Unlock()
	if err != nil {
		// Almost all of these errors are parse errors, and user input errors.
		// We do not want to spam the log with WARN messages.
		logem(s.conn).Err(err).Debug("Incoming mail error")
		s.backend.failure++
		minc(metricEmailsReceivedFailure)
	}
	return err
}

func readMailBody(body io.Reader, header mail.Header) (string, error) {
	if header.Get("Content-Type") == "" {
		return readPlainTextMailBody(body, header.Get("Content-Transfer-Encoding"))
	}
	contentType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	canonicalContentType := strings.ToLower(contentType)
	if canonicalContentType == "text/plain" || canonicalContentType == "text/html" {
		return readTextMailBody(body, canonicalContentType, header.Get("Content-Transfer-Encoding"))
	} else if strings.HasPrefix(canonicalContentType, "multipart/") {
		return readMultipartMailBody(body, params)
	}
	return "", errUnsupportedContentType
}

func readMultipartMailBody(body io.Reader, params map[string]string) (string, error) {
	parts := make(map[string]string)
	if err := readMultipartMailBodyParts(body, params, 0, parts); err != nil && err != io.EOF {
		return "", err
	} else if s, ok := parts["text/plain"]; ok {
		return s, nil
	} else if s, ok := parts["text/html"]; ok {
		return s, nil
	}
	return "", io.EOF
}

func readMultipartMailBodyParts(body io.Reader, params map[string]string, depth int, parts map[string]string) error {
	if depth >= maxMultipartDepth {
		return errMultipartNestedTooDeep
	}
	mr := multipart.NewReader(body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil { // may be io.EOF
			return err
		}
		partContentType, partParams, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			return err
		}
		canonicalPartContentType := strings.ToLower(partContentType)
		if canonicalPartContentType == "text/plain" || canonicalPartContentType == "text/html" {
			s, err := readTextMailBody(part, canonicalPartContentType, part.Header.Get("Content-Transfer-Encoding"))
			if err != nil {
				return err
			}
			parts[canonicalPartContentType] = s
		} else if strings.HasPrefix(strings.ToLower(partContentType), "multipart/") {
			if err := readMultipartMailBodyParts(part, partParams, depth+1, parts); err != nil {
				return err
			}
		}
		// Continue with next part
	}
}

func readTextMailBody(reader io.Reader, contentType, transferEncoding string) (string, error) {
	if contentType == "text/plain" {
		return readPlainTextMailBody(reader, transferEncoding)
	} else if contentType == "text/html" {
		return readHTMLMailBody(reader, transferEncoding)
	}
	return "", fmt.Errorf("unsupported content type: %s", contentType)
}

func readPlainTextMailBody(reader io.Reader, transferEncoding string) (string, error) {
	if strings.ToLower(transferEncoding) == "base64" {
		reader = base64.NewDecoder(base64.StdEncoding, reader)
	} else if strings.ToLower(transferEncoding) == "quoted-printable" {
		reader = quotedprintable.NewReader(reader)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func readHTMLMailBody(reader io.Reader, transferEncoding string) (string, error) {
	body, err := readPlainTextMailBody(reader, transferEncoding)
	if err != nil {
		return "", err
	}
	stripped := bluemonday.
		StrictPolicy().
		AddSpaceWhenStrippingTag(true).
		Sanitize(body)
	return removeExtraEmptyLines(stripped), nil
}

func removeExtraEmptyLines(str string) string {
	// Replace lines that contain only spaces with empty lines
	re := regexp.MustCompile(`(?m)^\s+$`)
	str = re.ReplaceAllString(str, "")

	// Remove more than 2 consecutive empty lines
	re = regexp.MustCompile(`\n{3,}`)
	str = re.ReplaceAllString(str, "\n\n")

	return str
}
