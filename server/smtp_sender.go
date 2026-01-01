package server

import (
	"bytes"
	_ "embed" // required by go:embed
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"slices"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/util"
)

var (
	errUnencryptedConnection     = errors.New("unencrypted connection")
	errWrongHostname             = errors.New("wrong host name")
	errNoSupportedAuth           = errors.New("no supported auth mechanisms found")
	errUnexpectedServerChallenge = errors.New("unexpected server challenge")
)

type mailer interface {
	Send(v *visitor, m *message, to string) error
	Counts() (total int64, success int64, failure int64)
}

type plainOrLoginAuth struct {
	identity   string
	username   string
	password   string
	host       string
	authMethod string
}

func PlainOrLoginAuth(identity, username, password, host string) smtp.Auth {
	return &plainOrLoginAuth{identity: identity, username: username, password: password, host: host}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

func (a *plainOrLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// Must have TLS, or else localhost server.
	// Note: If TLS is not true, then we can't trust ANYTHING in ServerInfo.
	// In particular, it doesn't matter if the server advertises PLAIN auth.
	// That might just be the attacker saying
	// "it's ok, you can trust me with your password."
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errUnencryptedConnection
	}
	if server.Name != a.host {
		return "", nil, errWrongHostname
	}

	if slices.Contains(server.Auth, "PLAIN") {
		a.authMethod = "PLAIN"
		resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
		return a.authMethod, resp, nil
	} else if slices.Contains(server.Auth, "LOGIN") {
		a.authMethod = "LOGIN"
		return a.authMethod, nil, nil
	} else {
		return "", nil, errNoSupportedAuth
	}
}

func (a *plainOrLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}

	if a.authMethod == "PLAIN" {
		// We've already sent everything.
		return nil, errUnexpectedServerChallenge
	}

	switch {
	case bytes.Equal(fromServer, []byte("Username:")):
		return []byte(a.username), nil
	case bytes.Equal(fromServer, []byte("Password:")):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnexpectedServerChallenge, fromServer)
	}
}

type smtpSender struct {
	config  *Config
	success int64
	failure int64
	mu      sync.Mutex
}

func (s *smtpSender) Send(v *visitor, m *message, to string) error {
	return s.withCount(v, m, func() error {
		host, _, err := net.SplitHostPort(s.config.SMTPSenderAddr)
		if err != nil {
			return err
		}
		message, err := formatMail(s.config.BaseURL, v.ip.String(), s.config.SMTPSenderFrom, to, m)
		if err != nil {
			return err
		}
		var auth smtp.Auth
		if s.config.SMTPSenderUser != "" {
			auth = PlainOrLoginAuth("", s.config.SMTPSenderUser, s.config.SMTPSenderPass, host)
		}
		ev := logvm(v, m).
			Tag(tagEmail).
			Fields(log.Context{
				"email_via":  s.config.SMTPSenderAddr,
				"email_user": s.config.SMTPSenderUser,
				"email_to":   to,
			})
		if ev.IsTrace() {
			ev.Field("email_body", message).Trace("Sending email")
		} else if ev.IsDebug() {
			ev.Debug("Sending email")
		}
		return smtp.SendMail(s.config.SMTPSenderAddr, auth, s.config.SMTPSenderFrom, []string{to}, []byte(message))
	})
}

func (s *smtpSender) Counts() (total int64, success int64, failure int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.success + s.failure, s.success, s.failure
}

func (s *smtpSender) withCount(v *visitor, m *message, fn func() error) error {
	err := fn()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		logvm(v, m).Err(err).Debug("Sending mail failed")
		s.failure++
	} else {
		s.success++
	}
	return err
}

func formatMail(baseURL, senderIP, from, to string, m *message) (string, error) {
	topicURL := baseURL + "/" + m.Topic
	subject := m.Title
	if subject == "" {
		subject = m.Message
	}
	subject = strings.ReplaceAll(strings.ReplaceAll(subject, "\r", ""), "\n", " ")
	message := m.Message
	trailer := ""
	if len(m.Tags) > 0 {
		emojis, tags, err := toEmojis(m.Tags)
		if err != nil {
			return "", err
		}
		if len(emojis) > 0 {
			subject = strings.Join(emojis, " ") + " " + subject
		}
		if len(tags) > 0 {
			trailer = "Tags: " + strings.Join(tags, ", ")
		}
	}
	if m.Priority != 0 && m.Priority != 3 {
		priority, err := util.PriorityString(m.Priority)
		if err != nil {
			return "", err
		}
		if trailer != "" {
			trailer += "\n"
		}
		trailer += fmt.Sprintf("Priority: %s", priority)
	}
	if trailer != "" {
		message += "\n\n" + trailer
	}
	date := time.Unix(m.Time, 0).UTC().Format(time.RFC1123Z)
	subject = mime.BEncoding.Encode("utf-8", subject)
	body := `From: "{shortTopicURL}" <{from}>
To: {to}
Date: {date}
Subject: {subject}
Content-Type: text/plain; charset="utf-8"

{message}

--
This message was sent by {ip} at {time} via {topicURL}`
	body = strings.ReplaceAll(body, "{from}", from)
	body = strings.ReplaceAll(body, "{to}", to)
	body = strings.ReplaceAll(body, "{date}", date)
	body = strings.ReplaceAll(body, "{subject}", subject)
	body = strings.ReplaceAll(body, "{message}", message)
	body = strings.ReplaceAll(body, "{topicURL}", topicURL)
	body = strings.ReplaceAll(body, "{shortTopicURL}", util.ShortTopicURL(topicURL))
	body = strings.ReplaceAll(body, "{time}", time.Unix(m.Time, 0).UTC().Format(time.RFC1123))
	body = strings.ReplaceAll(body, "{ip}", senderIP)
	return body, nil
}

var (
	//go:embed "mailer_emoji_map.json"
	emojisJSON string
)

func toEmojis(tags []string) (emojisOut []string, tagsOut []string, err error) {
	var emojiMap map[string]string
	if err = json.Unmarshal([]byte(emojisJSON), &emojiMap); err != nil {
		return nil, nil, err
	}
	tagsOut = make([]string, 0)
	emojisOut = make([]string, 0)
	for _, t := range tags {
		if emoji, ok := emojiMap[t]; ok {
			emojisOut = append(emojisOut, emoji)
		} else {
			tagsOut = append(tagsOut, t)
		}
	}
	return
}
