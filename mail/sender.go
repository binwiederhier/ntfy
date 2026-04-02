package mail

import (
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

const (
	verifyCodeExpiry  = 10 * time.Minute
	verifyCodeLength  = 6
	verifyCodeSubject = "ntfy email verification"
)

// Config holds the SMTP configuration for the mail sender
type Config struct {
	SMTPAddr string // SMTP server address (host:port)
	SMTPUser string // SMTP auth username
	SMTPPass string // SMTP auth password
	From     string // Sender email address
}

// Sender sends emails and manages email verification codes
type Sender struct {
	config    *Config
	codes     map[string]verifyCode // Verification codes, keyed by email
	mu        sync.Mutex
	closeChan chan struct{}
}

type verifyCode struct {
	code    string
	expires time.Time
}

// NewSender creates a new mail Sender with the given SMTP config
func NewSender(config *Config) *Sender {
	s := &Sender{
		config:    config,
		codes:     make(map[string]verifyCode),
		closeChan: make(chan struct{}),
	}
	go s.expireLoop()
	return s
}

// Close stops the background expiry loop
func (s *Sender) Close() {
	close(s.closeChan)
}

// Addr returns the SMTP server address
func (s *Sender) Addr() string {
	return s.config.SMTPAddr
}

// User returns the SMTP username
func (s *Sender) User() string {
	return s.config.SMTPUser
}

// From returns the sender email address
func (s *Sender) From() string {
	return s.config.From
}

// SendRaw sends a raw email message via SMTP
func (s *Sender) SendRaw(to string, message []byte) error {
	host, _, err := net.SplitHostPort(s.config.SMTPAddr)
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if s.config.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPass, host)
	}
	return smtp.SendMail(s.config.SMTPAddr, auth, s.config.From, []string{to}, message)
}

// Send sends a plain text email via SMTP
func (s *Sender) Send(to, subject, body string) error {
	date := time.Now().UTC().Format(time.RFC1123Z)
	encodedSubject := mime.BEncoding.Encode("utf-8", subject)
	message := `From: ntfy <{from}>
To: {to}
Date: {date}
Subject: {subject}
Content-Type: text/plain; charset="utf-8"

{body}`
	message = strings.ReplaceAll(message, "{from}", s.config.From)
	message = strings.ReplaceAll(message, "{to}", to)
	message = strings.ReplaceAll(message, "{date}", date)
	message = strings.ReplaceAll(message, "{subject}", encodedSubject)
	message = strings.ReplaceAll(message, "{body}", body)
	log.Tag("mail").Field("email_to", to).Debug("Sending email")
	return s.SendRaw(to, []byte(message))
}

// SendVerification generates a random code, stores it in-memory, and sends a verification email
func (s *Sender) SendVerification(to string) error {
	code := util.RandomString(verifyCodeLength)
	s.mu.Lock()
	s.codes[to] = verifyCode{
		code:    code,
		expires: time.Now().Add(verifyCodeExpiry),
	}
	s.mu.Unlock()
	body := fmt.Sprintf("Your ntfy email verification code is: %s\n\nThis code expires in 10 minutes.", code)
	return s.Send(to, verifyCodeSubject, body)
}

// CheckVerification checks if the code matches and hasn't expired. Removes the entry on success.
func (s *Sender) CheckVerification(email, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vc, ok := s.codes[email]
	if !ok || time.Now().After(vc.expires) || vc.code != code {
		return false
	}
	delete(s.codes, email)
	return true
}

func (s *Sender) expireLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.expireVerificationCodes()
		case <-s.closeChan:
			return
		}
	}
}

func (s *Sender) expireVerificationCodes() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for email, vc := range s.codes {
		if now.After(vc.expires) {
			delete(s.codes, email)
		}
	}
}
