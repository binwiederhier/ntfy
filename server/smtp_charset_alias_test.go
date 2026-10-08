package server

import (
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestReadMailCharset_AliasSafety(t *testing.T) {
	for _, tt := range []struct {
		name, charset, input, want string
	}{
		{"iana_latin1", "ISO-8859-1", "x\x80y", "x\u0080y"},
		{"utf8_alias", "UtF8", "Żółć 🎅", "Żółć 🎅"},
		{"ascii_alias", "ascii", "Printer ready", "Printer ready"},
		{"windows_alias", "cp1252", "Price \x80", "Price €"},
		{"padded_alias", " \tCP1252 ", "Price \x80", "Price €"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := readMailCharset(tt.charset, strings.NewReader(tt.input))
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(body))
		})
	}
	for _, charset := range []string{"x-unknown", "utf-7", "unknown-8bit", "iso-2022-kr", "csiso2022kr", "iso-2022-cn", "iso-2022-cn-ext", "replacement"} {
		t.Run("reject/"+charset, func(t *testing.T) {
			reader, err := readMailCharset(charset, strings.NewReader("Do not discard this text"))
			require.ErrorContains(t, err, "unhandled charset")
			require.Nil(t, reader)
		})
	}
	for _, charset := range []string{"ISO-8859-2", "cp1252", "utf8"} {
		t.Run("read_error/"+charset, func(t *testing.T) {
			want := errors.New("input failed")
			body, err := readPlainTextMailBody(iotest.ErrReader(want), "8bit", charset)
			require.ErrorIs(t, err, want)
			require.Empty(t, body)
		})
	}
}

func TestSmtpBackend_CharsetAliasEndToEnd(t *testing.T) {
	for _, tt := range []struct{ charset, input, want string }{
		{"utf8", "Żółć 🎅 =F3", "Żółć 🎅 =F3"},
		{"ascii", "Printer ready =F3", "Printer ready =F3"},
		{"cp1252", "Price \x80 =F3", "Price € =F3"},
	} {
		for _, transfer := range []string{"8bit", "quoted-printable", "base64"} {
			for _, form := range []string{"plain", "html", "multipart", "nested"} {
				for _, attachments := range []bool{false, true} {
					for _, encodedSubject := range []bool{false, true} {
						name := fmt.Sprintf("%s/%s/%s/attachments=%t/subject=%t", tt.charset, transfer, form, attachments, encodedSubject)
						t.Run(name, func(t *testing.T) {
							conf := newTestConfig(t, "")
							conf.AttachmentCacheDir = ""
							if attachments {
								conf.AttachmentCacheDir = t.TempDir()
							}
							publisher := newTestServer(t, conf)
							s, conn, _, _ := newTestSMTPServer(t, publisher.handle)
							defer s.Close()
							defer conn.Close()
							require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
							textPart := "Content-Type: text/plain; charset=" + tt.charset + "\r\nContent-Transfer-Encoding: " + transfer + "\r\n\r\n" + encodeSMTPBody(t, tt.input, transfer) + "\r\n"
							htmlPart := "Content-Type: text/html; charset=" + tt.charset + "\r\nContent-Transfer-Encoding: " + transfer + "\r\n\r\n" + encodeSMTPBody(t, "<script>discard()</script><p>"+tt.input+"</p>", transfer) + "\r\n"
							email := textPart
							switch form {
							case "html":
								email = htmlPart
							case "multipart":
								email = "Content-Type: multipart/alternative; boundary=parts\r\n\r\n--parts\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n<p>fallback</p>\r\n--parts\r\n" + textPart + "--parts--\r\n"
							case "nested":
								// The container's transfer encoding must not corrupt its '=' boundary.
								email = "Content-Type: multipart/mixed; boundary=\"outer==\"\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n--outer==\r\nContent-Type: application/octet-stream; charset=x-unknown\r\n\r\nignored\r\n--outer==\r\nContent-Type: multipart/alternative; boundary=\"inner==\"\r\n\r\n--inner==\r\n" + htmlPart + "--inner==--\r\n--outer==--\r\n"
							}
							wantTitle := ""
							if encodedSubject {
								email = "Subject: =?cp1252?Q?Printer_=80?=\r\n" + email
								wantTitle = "Printer €"
							}
							sendCharsetSMTPData(t, textproto.NewConn(conn), email, 250)
							poll := request(t, publisher, "GET", "/mytopic/json?poll=1", "", nil)
							require.Equal(t, 200, poll.Code)
							messages := toMessages(t, poll.Body.String())
							require.Len(t, messages, 1)
							require.Equal(t, wantTitle, messages[0].Title)
							require.Equal(t, tt.want, messages[0].Message)
							require.Nil(t, messages[0].Attachment)
							total, success, failure := s.Backend.(*smtpBackend).Counts()
							require.EqualValues(t, 1, total)
							require.EqualValues(t, 1, success)
							require.Zero(t, failure)
						})
					}
				}
			}
		}
	}
}

func TestSmtpBackend_CharsetAliasRejectNoPublish(t *testing.T) {
	for _, tt := range []struct{ name, email, wantError string }{
		{"replacement_body", "Content-Type: text/plain; charset=iso-2022-kr\r\n\r\ntext\r\n", "unhandled charset"},
		{"unknown_body", "Content-Type: text/plain; charset=unknown-8bit\r\n\r\ntext\r\n", "unhandled charset"},
		{"replacement_subject", "Subject: =?iso-2022-kr?Q?title?=\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\ntext\r\n", "unhandled charset"},
		{"bad_base64", "Content-Type: text/plain; charset=cp1252\r\nContent-Transfer-Encoding: base64\r\n\r\n%%%%\r\n", "illegal base64 data"},
		{"short_base64", "Content-Type: text/plain; charset=cp1252\r\nContent-Transfer-Encoding: base64\r\n\r\nWmE\r\n", "unexpected EOF"},
		{"bad_content_type", "Content-Type: text/plain; charset=\r\n\r\ntext\r\n", "mime:"},
	} {
		for _, attachments := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/attachments=%t", tt.name, attachments), func(t *testing.T) {
				conf := newTestConfig(t, "")
				conf.AttachmentCacheDir = ""
				if attachments {
					conf.AttachmentCacheDir = t.TempDir()
				}
				publisher := newTestServer(t, conf)
				s, conn, _, _ := newTestSMTPServer(t, publisher.handle)
				defer s.Close()
				defer conn.Close()
				require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
				response := sendCharsetSMTPData(t, textproto.NewConn(conn), tt.email, 554)
				require.Contains(t, response, tt.wantError)
				poll := request(t, publisher, "GET", "/mytopic/json?poll=1", "", nil)
				require.Equal(t, 200, poll.Code)
				require.Empty(t, toMessages(t, poll.Body.String()))
				total, success, failure := s.Backend.(*smtpBackend).Counts()
				require.EqualValues(t, 1, total)
				require.Zero(t, success)
				require.EqualValues(t, 1, failure)
			})
		}
	}
}

func FuzzReadMailCharsetAliases(f *testing.F) {
	f.Add([]byte("Price \x80 =F3"), uint8(0))
	f.Add([]byte("Żółć 🎅"), uint8(1))
	f.Add([]byte("must not disappear"), uint8(6))
	labels := []string{"cp1252", "utf8", "ascii", "ISO-8859-2", "Shift_JIS", "replacement", "iso-2022-kr", "unknown-8bit"}
	f.Fuzz(func(t *testing.T, data []byte, label uint8) {
		if len(data) > 64*1024 {
			t.Skip()
		}
		reader, err := readMailCharset(labels[int(label)%len(labels)], strings.NewReader(string(data)))
		if err != nil {
			require.Nil(t, reader)
			return
		}
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.True(t, utf8.Valid(body))
	})
}
