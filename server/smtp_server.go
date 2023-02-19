package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"sync"
)

var (
	errInvalidDomain          = errors.New("invalid domain")
	errInvalidAddress         = errors.New("invalid address")
	errInvalidTopic           = errors.New("invalid topic")
	errTooManyRecipients      = errors.New("too many recipients")
	errUnsupportedContentType = errors.New("unsupported content type")
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
		to = strings.TrimSuffix(to, "@"+conf.SMTPServerDomain)
		if conf.SMTPServerAddrPrefix != "" {
			if !strings.HasPrefix(to, conf.SMTPServerAddrPrefix) {
				return errInvalidAddress
			}
			to = strings.TrimPrefix(to, conf.SMTPServerAddrPrefix)
		}
		if !topicRegex.MatchString(to) {
			return errInvalidTopic
		}
		s.mu.Lock()
		s.topic = to
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
		body, err := readMailBody(msg)
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
	}
	return err
}

func readMailBody(msg *mail.Message) (string, error) {
	if msg.Header.Get("Content-Type") == "" {
		return readPlainTextMailBody(msg.Body, msg.Header.Get("Content-Transfer-Encoding"))
	}
	contentType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	if strings.ToLower(contentType) == "text/plain" {
		return readPlainTextMailBody(msg.Body, msg.Header.Get("Content-Transfer-Encoding"))
	} else if strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
		return readMultipartMailBody(msg, params)
	}
	return "", errUnsupportedContentType
}

func readMultipartMailBody(msg *mail.Message, params map[string]string) (string, error) {
	mr := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil { // may be io.EOF
			return "", err
		}
		partContentType, _, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			return "", err
		} else if strings.ToLower(partContentType) != "text/plain" {
			continue
		}
		return readPlainTextMailBody(part, part.Header.Get("Content-Transfer-Encoding"))
	}
}

func readPlainTextMailBody(reader io.Reader, transferEncoding string) (string, error) {
	if strings.ToLower(transferEncoding) == "base64" {
		reader = base64.NewDecoder(base64.StdEncoding, reader)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
