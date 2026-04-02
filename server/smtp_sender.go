package server

import (
	_ "embed" // required by go:embed
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"sync"
	"time"

	"heckel.io/ntfy/v2/log"
	"heckel.io/ntfy/v2/mail"
	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

type mailer interface {
	Send(v *visitor, m *model.Message, to string) error
	Counts() (total int64, success int64, failure int64)
}

type smtpSender struct {
	config  *Config
	sender  *mail.Sender
	success int64
	failure int64
	mu      sync.Mutex
}

func (s *smtpSender) Send(v *visitor, m *model.Message, to string) error {
	return s.withCount(v, m, func() error {
		message, err := formatMail(s.config.BaseURL, v.ip.String(), s.sender.From(), to, m)
		if err != nil {
			return err
		}
		ev := logvm(v, m).
			Tag(tagEmail).
			Fields(log.Context{
				"email_via":  s.sender.Addr(),
				"email_user": s.sender.User(),
				"email_to":   to,
			})
		if ev.IsTrace() {
			ev.Field("email_body", message).Trace("Sending email")
		}
		ev.Info("Sending email")
		return s.sender.SendRaw(to, []byte(message))
	})
}

func (s *smtpSender) Counts() (total int64, success int64, failure int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.success + s.failure, s.success, s.failure
}

func (s *smtpSender) withCount(v *visitor, m *model.Message, fn func() error) error {
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

func formatMail(baseURL, senderIP, from, to string, m *model.Message) (string, error) {
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
