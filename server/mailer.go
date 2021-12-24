package server

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

type mailer interface {
	Send(from, to string, m *message) error
}

type smtpMailer struct {
	config *Config
}

func (s *smtpMailer) Send(from, to string, m *message) error {
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
	message := m.Message
	if len(m.Tags) > 0 {
		message += "\nTags: " + strings.Join(m.Tags, ", ") // FIXME emojis
	}
	if m.Priority != 0 && m.Priority != 3 {
		message += fmt.Sprintf("\nPriority: %d", m.Priority) // FIXME to string
	}
	message += fmt.Sprintf("\n\n--\nMessage was sent via %s by client %s", m.Topic, from) // FIXME short URL
	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n\r\n"+
		"%s\r\n", s.config.SMTPFrom, to, subject, message))
	auth := smtp.PlainAuth("", s.config.SMTPUser, s.config.SMTPPass, host)
	return smtp.SendMail(s.config.SMTPAddr, auth, s.config.SMTPFrom, []string{to}, msg)
}
