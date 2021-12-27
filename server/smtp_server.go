package server

import (
	"bytes"
	"errors"
	"github.com/emersion/go-smtp"
	"io"
	"io/ioutil"
	"log"
	"net/mail"
	"strings"
	"sync"
)

// smtpBackend implements SMTP server methods.
type smtpBackend struct {
	config *Config
	sub    subscriber
}

func newMailBackend(conf *Config, sub subscriber) *smtpBackend {
	return &smtpBackend{
		config: conf,
		sub:    sub,
	}
}

func (b *smtpBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return &smtpSession{config: b.config, sub: b.sub}, nil
}

func (b *smtpBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &smtpSession{config: b.config, sub: b.sub}, nil
}

// smtpSession is returned after EHLO.
type smtpSession struct {
	config   *Config
	sub      subscriber
	from, to string
	mu       sync.Mutex
}

func (s *smtpSession) AuthPlain(username, password string) error {
	return nil
}

func (s *smtpSession) Mail(from string, opts smtp.MailOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	addressList, err := mail.ParseAddressList(to)
	if err != nil {
		return err
	} else if len(addressList) != 1 {
		return errors.New("only one recipient supported")
	} else if !strings.HasSuffix(addressList[0].Address, "@"+s.config.SMTPServerDomain) {
		return errors.New("invalid domain")
	} else if s.config.SMTPServerAddrPrefix != "" && !strings.HasPrefix(addressList[0].Address, s.config.SMTPServerAddrPrefix) {
		return errors.New("invalid address")
	}
	// FIXME check topic format
	s.to = addressList[0].Address
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	log.Println("Data:", string(b))
	msg, err := mail.ReadMessage(bytes.NewReader(b))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return err
	}
	topic := strings.TrimSuffix(s.to, "@"+s.config.SMTPServerDomain)
	m := newDefaultMessage(topic, string(body))
	subject := msg.Header.Get("Subject")
	if subject != "" {
		m.Title = subject
	}
	return s.sub(m)
}

func (s *smtpSession) Reset() {
	s.mu.Lock()
	s.from = ""
	s.to = ""
	s.mu.Unlock()
}

func (s *smtpSession) Logout() error {
	return nil
}
