package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFormatMail_Basic(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject: A simple message

A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}

func TestFormatMail_JustEmojis(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Tags:    []string{"grinning"},
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject: 😀 A simple message

A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}

func TestFormatMail_JustOtherTags(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Tags:    []string{"not-an-emoji"},
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject: A simple message

A simple message

Tags: not-an-emoji

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}

func TestFormatMail_JustPriority(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:       "abc",
		Time:     1640382204,
		Event:    "message",
		Topic:    "alerts",
		Message:  "A simple message",
		Priority: 2,
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject: A simple message

A simple message

Priority: low

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}

func TestFormatMail_UTF8Subject(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Title:   " :: A not so simple title öäüß ¡Hola, señor!",
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject:  :: A not so simple title öäüß ¡Hola, señor!

A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}

func TestFormatMail_WithAllTheThings(t *testing.T) {
	actual, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", "phil@example.com", &message{
		ID:       "abc",
		Time:     1640382204,
		Event:    "message",
		Topic:    "alerts",
		Priority: 5,
		Tags:     []string{"warning", "skull", "tag123", "other"},
		Title:    "Oh no 🙈\nThis is a message across\nmultiple lines",
		Message:  "A message that contains monkeys 🙉\nNo really, though. Monkeys!",
	})
	expected := `Content-Type: text/plain; charset="utf-8"
From: "ntfy.sh/alerts" <ntfy@ntfy.sh>
To: phil@example.com
Subject: ⚠️ 💀 Oh no 🙈 This is a message across multiple lines

A message that contains monkeys 🙉
No really, though. Monkeys!

Tags: tag123, other
Priority: max

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expected, actual)
}
