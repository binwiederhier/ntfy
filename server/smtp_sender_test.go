package server

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFormatMail_Basic(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `A simple message`
	expectedMessage := `A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}

func TestFormatMail_JustEmojis(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Tags:    []string{"grinning"},
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `=?utf-8?b?8J+YgCBBIHNpbXBsZSBtZXNzYWdl?=`
	expectedMessage := `A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}

func TestFormatMail_JustOtherTags(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Tags:    []string{"not-an-emoji"},
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `A simple message`
	expectedMessage := `A simple message

Tags: not-an-emoji

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}

func TestFormatMail_JustPriority(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:       "abc",
		Time:     1640382204,
		Event:    "message",
		Topic:    "alerts",
		Message:  "A simple message",
		Priority: 2,
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `A simple message`
	expectedMessage := `A simple message

Priority: low

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}

func TestFormatMail_UTF8Subject(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:      "abc",
		Time:    1640382204,
		Event:   "message",
		Topic:   "alerts",
		Message: "A simple message",
		Title:   " :: A not so simple title öäüß ¡Hola, señor!",
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `=?utf-8?b?IDo6IEEgbm90IHNvIHNpbXBsZSB0aXRsZSDDtsOkw7zDnyDCoUhvbGEsIHNl?= =?utf-8?b?w7FvciE=?=`
	expectedMessage := `A simple message

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}

func TestFormatMail_WithAllTheThings(t *testing.T) {
	actualFrom, actualSubject, actualMessage, _ := formatMail("https://ntfy.sh", "1.2.3.4", "ntfy@ntfy.sh", &message{
		ID:       "abc",
		Time:     1640382204,
		Event:    "message",
		Topic:    "alerts",
		Priority: 5,
		Tags:     []string{"warning", "skull", "tag123", "other"},
		Title:    "Oh no 🙈\nThis is a message across\nmultiple lines",
		Message:  "A message that contains monkeys 🙉\nNo really, though. Monkeys!",
	})
	expectedFrom := `"ntfy.sh/alerts" <ntfy@ntfy.sh>`
	expectedSubject := `=?utf-8?b?4pqg77iPIPCfkoAgT2ggbm8g8J+ZiCBUaGlzIGlzIGEgbWVzc2FnZSBhY3Jv?= =?utf-8?b?c3MgbXVsdGlwbGUgbGluZXM=?=`
	expectedMessage := `A message that contains monkeys 🙉
No really, though. Monkeys!

Tags: tag123, other
Priority: max

--
This message was sent by 1.2.3.4 at Fri, 24 Dec 2021 21:43:24 UTC via https://ntfy.sh/alerts`
	require.Equal(t, expectedMessage, actualMessage)
	require.Equal(t, expectedFrom, actualFrom)
	require.Equal(t, expectedSubject, actualSubject)
}
