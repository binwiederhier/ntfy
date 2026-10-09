package mail

import (
	_ "embed" // required by go:embed
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	"heckel.io/ntfy/v2/model"
	"heckel.io/ntfy/v2/util"
)

var (
	//go:embed "mailer_emoji_map.json"
	emojisJSON string

	// emojiMap maps ntfy tag names to emoji, parsed once from the embedded JSON in init
	emojiMap map[string]string
)

func init() {
	if err := json.Unmarshal([]byte(emojisJSON), &emojiMap); err != nil {
		panic("mail: invalid embedded emoji map: " + err.Error())
	}
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
		emojis, tags := toEmojis(m.Tags)
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
	if m.Attachment != nil && m.Attachment.URL != "" {
		if trailer != "" {
			trailer += "\n"
		}
		trailer += "Attachment: " + formatAttachment(m.Attachment)
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

// formatAttachment links the attachment rather than attaching it, since emails never expire
// but uploaded files do (and would turn ntfy into a file host)
func formatAttachment(a *model.Attachment) string {
	details := make([]string, 0)
	if a.Name != "" {
		details = append(details, a.Name)
	}
	if a.Expires > 0 {
		details = append(details, "expires "+time.Unix(a.Expires, 0).UTC().Format(time.RFC1123))
	}
	if len(details) == 0 {
		return a.URL
	}
	return fmt.Sprintf("%s (%s)", a.URL, strings.Join(details, ", "))
}

func toEmojis(tags []string) (emojisOut []string, tagsOut []string) {
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
