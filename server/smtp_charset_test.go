package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestReadMailBody_Charset(t *testing.T) {
	const polish = "Zażółć gęślą jaźń"
	const latin2 = "Za\xbf\xf3\xb3\xe6 g\xea\xb6l\xb1 ja\xbc\xf1"
	tests := []struct {
		name, contentType, transferEncoding, body, want, wantError string
	}{
		{"latin2_8bit", "text/plain; charset=ISO-8859-2", "8bit", latin2, polish, ""},
		{"latin2_base64", "text/plain; charset=ISO-8859-2", "base64", base64.StdEncoding.EncodeToString([]byte(latin2)), polish, ""},
		{"latin2_qp", "text/plain; charset=ISO-8859-2", "quoted-printable", "Za=BF=F3=B3=E6 g=EA=B6l=B1 ja=BC=F1", polish, ""},
		{"mixed_case", "TEXT/PLAIN; CHARSET=\"iSo-8859-2\"", "BaSe64", base64.StdEncoding.EncodeToString([]byte(latin2)), polish, ""},
		{"alias", "text/plain; charset=csISOLatin2", "8bit", latin2, polish, ""},
		{"windows1250", "text/plain; charset=windows-1250", "8bit", "Drukarka b\xb3\xb9d \x80", "Drukarka błąd €", ""},
		{"latin1", "text/plain; charset=ISO-8859-1", "8bit", "Gr\xfc\xdfe", "Grüße", ""},
		{"latin1_not_windows1252", "text/plain; charset=ISO-8859-1", "8bit", "x\x80y", "x\u0080y", ""},
		{"shift_jis", "text/plain; charset=Shift_JIS", "base64", base64.StdEncoding.EncodeToString([]byte("\x93\xfa\x96\x7b")), "日本", ""},
		{"utf8", "text/plain; charset=UTF-8", "8bit", polish + " 🎅", polish + " 🎅", ""},
		{"ascii", "text/plain; charset=US-ASCII", "7bit", "Printer ready", "Printer ready", ""},
		{"missing_charset", "text/plain", "8bit", polish, polish, ""},
		{"missing_content_type", "", "8bit", polish, polish, ""},
		{"missing_content_type_base64", "", "base64", base64.StdEncoding.EncodeToString([]byte(polish)), polish, ""},
		{"empty_legacy", "text/plain; charset=ISO-8859-2", "8bit", "", "", ""},
		{"html", "text/html; charset=ISO-8859-2", "8bit", "<p>" + latin2 + "</p>", polish, ""},
		{"html_qp_sanitized", "text/html; charset=ISO-8859-2", "quoted-printable", "<script>alert(1)</script><p>Za=BF=F3=B3=E6 g=EA=B6l=B1 ja=BC=F1</p>", polish, ""},
		{"html_base64", "text/html; charset=ISO-8859-2", "base64", base64.StdEncoding.EncodeToString([]byte("<p>" + latin2 + "</p>")), polish, ""},
		{"unknown", "text/plain; charset=x-unknown", "8bit", "Printer ready", "", `mime: unhandled charset "x-unknown"`},
		{"unsupported", "text/plain; charset=utf-7", "8bit", "Printer ready", "", `mime: unhandled charset "utf-7"`},
		{"invalid_base64", "text/plain; charset=ISO-8859-2", "base64", "%%%", "", "illegal base64 data"},
		{"truncated_base64", "text/plain; charset=ISO-8859-2", "base64", "WmE", "", "unexpected EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := mail.Header{}
			if tt.contentType != "" {
				header["Content-Type"] = []string{tt.contentType}
			}
			header["Content-Transfer-Encoding"] = []string{tt.transferEncoding}
			body, err := readMailBody(strings.NewReader(tt.body), header)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.Empty(t, body)
				return
			}
			require.NoError(t, err)
			require.True(t, utf8.ValidString(body))
			require.Equal(t, tt.want, strings.TrimSpace(body))
		})
	}
}

func TestReadMailBody_CharsetCompatibility(t *testing.T) {
	// Keep undeclared, UTF-8 and US-ASCII bodies byte-for-byte as before. In
	// particular, do not guess a charset or replace invalid UTF-8 with U+FFFD.
	for _, contentType := range []string{"", "text/plain", "text/plain; charset=UTF-8", "text/plain; charset=US-ASCII"} {
		t.Run(contentType, func(t *testing.T) {
			const input = "not UTF-8: \xff"
			body, err := readMailBody(strings.NewReader(input), mail.Header{"Content-Type": {contentType}})
			require.NoError(t, err)
			require.Equal(t, input, body)
		})
	}
}

func TestReadMailBody_CharsetMultipart(t *testing.T) {
	for _, transferEncoding := range []string{"8bit", "quoted-printable", "QuOtEd-PrInTaBlE", "base64"} {
		for _, contentType := range []string{"text/plain", "text/html"} {
			t.Run(contentType+"/"+transferEncoding, func(t *testing.T) {
				// The literal =F3 catches accidental double quoted-printable decoding.
				body := "Drukarka b\xb3\xb1d =F3"
				if contentType == "text/html" {
					body = "<p>" + body + "</p>"
				}
				var data bytes.Buffer
				writer := multipart.NewWriter(&data)
				part, err := writer.CreatePart(textproto.MIMEHeader{
					"Content-Type":              {contentType + "; charset=ISO-8859-2"},
					"Content-Transfer-Encoding": {transferEncoding},
				})
				require.NoError(t, err)
				_, err = io.WriteString(part, encodeSMTPBody(t, body, transferEncoding))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				decoded, err := readMailBody(&data, mail.Header{"Content-Type": {"multipart/alternative; boundary=" + writer.Boundary()}})
				require.NoError(t, err)
				require.Equal(t, "Drukarka błąd =F3", strings.TrimSpace(decoded))
			})
		}
	}

	t.Run("nested_and_per_part_charset", func(t *testing.T) {
		body := "--outer\r\nContent-Type: multipart/alternative; boundary=inner\r\n\r\n" +
			"--inner\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n<p>HTML fallback</p>\r\n" +
			"--inner\r\nContent-Type: text/plain; charset=ISO-8859-2\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nDrukarka b=B3=B1d\r\n" +
			"--inner--\r\n--outer--\r\n"
		decoded, err := readMailBody(strings.NewReader(body), mail.Header{"Content-Type": {"multipart/mixed; boundary=outer"}})
		require.NoError(t, err)
		require.Equal(t, "Drukarka błąd", strings.TrimSpace(decoded))
	})

	for _, charset := range []string{"x-unknown", "utf-7"} {
		t.Run(charset, func(t *testing.T) {
			body := "--boundary\r\nContent-Type: text/plain; charset=" + charset + "\r\n\r\ntext\r\n--boundary--\r\n"
			decoded, err := readMailBody(strings.NewReader(body), mail.Header{"Content-Type": {"multipart/alternative; boundary=boundary"}})
			require.ErrorContains(t, err, "unhandled charset")
			require.Empty(t, decoded)
		})
	}
}

func TestSmtpBackend_BodyCharsetPublish(t *testing.T) {
	for _, attachments := range []bool{false, true} {
		for _, transferEncoding := range []string{"8bit", "quoted-printable", "base64"} {
			t.Run(fmt.Sprintf("attachments=%t/%s", attachments, transferEncoding), func(t *testing.T) {
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
				client := textproto.NewConn(conn)
				sendCharsetSMTPData(t, client, "Subject: =?ISO-8859-2?Q?Drukarka_b=B3=B1d?=\r\n"+
					"Content-Type: text/plain; charset=ISO-8859-2\r\nContent-Transfer-Encoding: "+transferEncoding+"\r\n\r\n"+
					encodeSMTPBody(t, "Za\xbf\xf3\xb3\xe6 g\xea\xb6l\xb1 ja\xbc\xf1", transferEncoding)+"\r\n", 250)
				response := request(t, publisher, "GET", "/mytopic/json?poll=1", "", nil)
				require.Equal(t, 200, response.Code)
				messages := toMessages(t, response.Body.String())
				require.Len(t, messages, 1)
				require.Equal(t, "Drukarka błąd", messages[0].Title)
				require.Equal(t, "Zażółć gęślą jaźń", messages[0].Message)
				require.Nil(t, messages[0].Attachment)
			})
		}
	}
}

func TestSmtpBackend_BodyCharsetTruncate(t *testing.T) {
	for _, tt := range []struct {
		name, charset, encoded, decoded string
	}{
		{"latin2", "ISO-8859-2", "\xb3", "ł"},
		{"utf8_two_bytes", "UTF-8", "ł", "ł"},
		{"utf8_three_bytes", "UTF-8", "日", "日"},
		{"utf8_four_bytes", "UTF-8", "🎅", "🎅"},
	} {
		for room := 1; room <= len(tt.decoded); room++ {
			t.Run(fmt.Sprintf("%s/room=%d", tt.name, room), func(t *testing.T) {
				conf := newTestConfig(t, "")
				conf.AttachmentCacheDir = ""
				publisher := newTestServer(t, conf)
				s, conn, smtpConf, _ := newTestSMTPServer(t, publisher.handle)
				defer s.Close()
				defer conn.Close()
				prefix := strings.Repeat("a", smtpConf.MessageSizeLimit-room)
				body := prefix + tt.encoded + "tail"
				want := prefix
				if room == len(tt.decoded) {
					want += tt.decoded
				}
				require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
				client := textproto.NewConn(conn)
				// Soft line breaks keep the wire input within SMTP line limits,
				// without inserting newlines into the decoded notification.
				sendCharsetSMTPData(t, client, "Content-Type: text/plain; charset="+tt.charset+"\r\n"+
					"Content-Transfer-Encoding: quoted-printable\r\n\r\n"+
					encodeSMTPBody(t, body, "quoted-printable")+"\r\n", 250)
				response := request(t, publisher, "GET", "/mytopic/json?poll=1", "", nil)
				require.Equal(t, 200, response.Code)
				messages := toMessages(t, response.Body.String())
				require.Len(t, messages, 1)
				require.Equal(t, want, messages[0].Message)
				require.True(t, utf8.ValidString(messages[0].Message))
				require.Nil(t, messages[0].Attachment)
			})
		}
	}
}

func TestSmtpBackend_BodyCharsetReject(t *testing.T) {
	for _, charset := range []string{"x-unknown", "utf-7"} {
		t.Run(charset, func(t *testing.T) {
			conf := newTestConfig(t, "")
			conf.AttachmentCacheDir = t.TempDir()
			publisher := newTestServer(t, conf)
			s, conn, _, _ := newTestSMTPServer(t, publisher.handle)
			defer s.Close()
			defer conn.Close()
			require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
			client := textproto.NewConn(conn)
			response := sendCharsetSMTPData(t, client, "Content-Type: text/plain; charset="+charset+"\r\n\r\ntext\r\n", 554)
			require.Contains(t, response, "unhandled charset")
			poll := request(t, publisher, "GET", "/mytopic/json?poll=1", "", nil)
			require.Equal(t, 200, poll.Code)
			require.Empty(t, toMessages(t, poll.Body.String()))
		})
	}
}

func encodeSMTPBody(t *testing.T, body, transferEncoding string) string {
	t.Helper()
	switch strings.ToLower(transferEncoding) {
	case "base64":
		return base64.StdEncoding.EncodeToString([]byte(body))
	case "quoted-printable":
		var encoded bytes.Buffer
		writer := quotedprintable.NewWriter(&encoded)
		_, err := io.WriteString(writer, body)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return encoded.String()
	default:
		return body
	}
}

func sendCharsetSMTPData(t *testing.T, client *textproto.Conn, email string, wantCode int) string {
	t.Helper()
	_, _, err := client.ReadResponse(220)
	require.NoError(t, err)
	for _, step := range []struct {
		command string
		code    int
	}{
		{"EHLO example.com", 250},
		{"MAIL FROM:<printer@example.com>", 250},
		{"RCPT TO:<ntfy-mytopic@ntfy.sh>", 250},
		{"DATA", 354},
	} {
		require.NoError(t, client.PrintfLine("%s", step.command))
		_, _, err := client.ReadResponse(step.code)
		require.NoError(t, err)
	}
	data := client.DotWriter()
	_, err = io.WriteString(data, email)
	require.NoError(t, err)
	require.NoError(t, data.Close())
	_, response, err := client.ReadResponse(wantCode)
	require.NoError(t, err)
	return response
}

func FuzzReadMailBodyCharset(f *testing.F) {
	f.Add([]byte("Drukarka b\xb3\xb1d =F3"), uint8(0))
	f.Add([]byte("Drukarka b=B3=B1d =3DF3"), uint8(1))
	f.Add([]byte("RHJ1a2Fya2EgYrOxZA=="), uint8(2))
	f.Add([]byte("<script>alert(1)</script><p>\xb3</p>"), uint8(3))
	f.Add([]byte("%%%"), uint8(2))
	f.Fuzz(func(t *testing.T, data []byte, variant uint8) {
		if len(data) > 64*1024 {
			t.Skip()
		}
		contentType := "text/plain; charset=ISO-8859-2"
		if variant&1 != 0 {
			contentType = "text/html; charset=windows-1250"
		}
		transferEncoding := []string{"8bit", "quoted-printable", "base64"}[int(variant)%3]
		body, err := readMailBody(bytes.NewReader(data), mail.Header{
			"Content-Type":              {contentType},
			"Content-Transfer-Encoding": {transferEncoding},
		})
		if err != nil {
			require.Empty(t, body)
			return
		}
		require.True(t, utf8.ValidString(body))
	})
}
