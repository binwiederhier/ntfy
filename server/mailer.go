package server

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type mailer interface {
	Send(to string, m *message) error
}

type smtpMailer struct {
	config *Config
}

func (s *smtpMailer) Send(to string, m *message) error {
	host, _, err := net.SplitHostPort(s.config.SMTPAddr)
	if err != nil {
		return err
	}
	subject := m.Title
	if subject == "" {
		subject = m.Message
	}
	subject += " - " + m.Topic
	subject = strings.ReplaceAll(strings.ReplaceAll(subject, "\r", ""), "\n", " ")
	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n\r\n"+
		"%s\r\n", s.config.SMTPFrom, to, subject, m.Message))
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPass, host)
	return smtp.SendMail(s.config.SMTPAddr, auth, s.config.SMTPFrom, []string{to}, msg)
}
