package server

import (
	"bufio"
	"github.com/emersion/go-smtp"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSmtpBackend_Multipart(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
MIME-Version: 1.0
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: ntfy-mytopic@ntfy.sh
Content-Type: multipart/alternative; boundary="000000000000f3320b05d42915c9"

--000000000000f3320b05d42915c9
Content-Type: text/plain; charset="UTF-8"

what's up

--000000000000f3320b05d42915c9
Content-Type: text/html; charset="UTF-8"

<div dir="ltr">what&#39;s up<br clear="all"><div><br></div></div>

--000000000000f3320b05d42915c9--
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "and one more", r.Header.Get("Title"))
		require.Equal(t, "what's up", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_MultipartNoBody(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-emailtest@ntfy.sh
DATA
MIME-Version: 1.0
Date: Tue, 28 Dec 2021 01:33:34 +0100
Message-ID: <CAAvm7ABCDsi9vsuu0WTRXzZQBC8dXrDOLT8iCWdqrsmg@mail.gmail.com>
Subject: This email has a subject but no body
From: Phil <phil@example.com>
To: ntfy-emailtest@ntfy.sh
Content-Type: multipart/alternative; boundary="000000000000bcf4a405d429f8d4"

--000000000000bcf4a405d429f8d4
Content-Type: text/plain; charset="UTF-8"



--000000000000bcf4a405d429f8d4
Content-Type: text/html; charset="UTF-8"

<div dir="ltr"><br></div>

--000000000000bcf4a405d429f8d4--
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/emailtest", r.URL.Path)
		require.Equal(t, "", r.Header.Get("Title")) // We flipped message and body
		require.Equal(t, "This email has a subject but no body", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Plaintext(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: mytopic@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: mytopic@ntfy.sh
Content-Type: text/plain; charset="UTF-8"

what's up
.
`
	s, c, conf, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "and one more", r.Header.Get("Title"))
		require.Equal(t, "what's up", readAll(t, r.Body))
	})
	conf.SMTPServerAddrPrefix = ""
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Plaintext_No_ContentType(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: mytopic@ntfy.sh
DATA
Subject: Very short mail

what's up
.
`
	s, c, conf, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "Very short mail", r.Header.Get("Title"))
		require.Equal(t, "what's up", readAll(t, r.Body))
	})
	conf.SMTPServerAddrPrefix = ""
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Plaintext_EncodedSubject(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Subject: =?UTF-8?B?VGhyZWUgc2FudGFzIPCfjoXwn46F8J+OhQ==?=
From: Phil <phil@example.com>
To: ntfy-mytopic@ntfy.sh
Content-Type: text/plain; charset="UTF-8"

what's up
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Three santas 🎅🎅🎅", r.Header.Get("Title"))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Plaintext_TooLongTruncate(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: mytopic@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: mytopic@ntfy.sh
Content-Type: text/plain; charset="UTF-8"

you know this is a string.
it's a long string.
it's supposed to be longer than the max message length
which is 4096 bytes,
it used to be 512 bytes, but I increased that for the UnifiedPush support
the 512 bytes was a little short, some people said
but it kinda makes sense when you look at what it looks like one a phone
heck this wasn't even half of it so far.
so i'm gonna fill the rest of this with AAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAa
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
and with BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
that should do it
.
`
	s, c, conf, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		expected := `you know this is a string.
it's a long string.
it's supposed to be longer than the max message length
which is 4096 bytes,
it used to be 512 bytes, but I increased that for the UnifiedPush support
the 512 bytes was a little short, some people said
but it kinda makes sense when you look at what it looks like one a phone
heck this wasn't even half of it so far.
so i'm gonna fill the rest of this with AAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAa
AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
pppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppppp
and with BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB
BBBBBBBBBBBBBBBBBBBBBBBBB`
		require.Equal(t, 4096, len(expected)) // Sanity check
		require.Equal(t, expected, readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	conf.SMTPServerAddrPrefix = ""
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Plaintext_QuotedPrintable(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: mytopic@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: mytopic@ntfy.sh
Content-Type: text/plain; charset="UTF-8"
Content-Transfer-Encoding: quoted-printable

what's
=C3=A0&=C3=A9"'(-=C3=A8_=C3=A7=C3=A0)
=3D=3D=3D=3D=3D
up
.
`
	s, c, conf, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "and one more", r.Header.Get("Title"))
		require.Equal(t, `what's
à&é"'(-è_çà)
=====
up`, readAll(t, r.Body))
	})
	conf.SMTPServerAddrPrefix = ""
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Unsupported(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: mytopic@ntfy.sh
Content-Type: text/SOMETHINGELSE

what's up
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("This should not be called")
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "554 5.0.0 Error: transaction failed, blame it on the weather: unsupported content type")
}

func TestSmtpBackend_InvalidAddress(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: unsupported@ntfy.sh
DATA
Date: Tue, 28 Dec 2021 00:30:10 +0100
Subject: and one more
From: Phil <phil@example.com>
To: mytopic@ntfy.sh
Content-Type: text/plain

what's up
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("This should not be called")
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "451 4.0.0 invalid address")
}

func TestSmtpBackend_Base64Body(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: test@mydomain.me
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Content-Type: multipart/mixed; boundary="===============2138658284696597373=="
MIME-Version: 1.0
Subject: TrueNAS truenas.local: TrueNAS Test Message hostname: truenas.local
From: =?utf-8?q?Robbie?= <test@mydomain.me>
To: test@mydomain.me
Date: Thu, 16 Feb 2023 01:04:00 -0000
Message-ID: <truenas-20230216.010400.344514.b'8jfL'@truenas.local>

This is a multi-part message in MIME format.
--===============2138658284696597373==
Content-Type: text/plain; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

VGhpcyBpcyBhIHRlc3QgbWVzc2FnZSBmcm9tIFRydWVOQVMgQ09SRS4=

--===============2138658284696597373==
Content-Type: text/html; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

PCFET0NUWVBFIEhUTUwgUFVCTElDICItLy9XM0MvL0RURCBIVE1MIDQuMCBUcmFuc2l0aW9uYWwv
L0VOIj4KClRoaXMgaXMgYSB0ZXN0IG1lc3NhZ2UgZnJvbSBUcnVlTkFTIENPUkUuCg==

--===============2138658284696597373==--
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "TrueNAS truenas.local: TrueNAS Test Message hostname: truenas.local", r.Header.Get("Title"))
		require.Equal(t, "This is a test message from TrueNAS CORE.", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_MultipartQuotedPrintable(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
MIME-Version: 1.0
Date: Tue, 28 Dec 2021 00:30:10 +0100
Message-ID: <CAAvm79YP0C=Rt1N=KWmSUBB87KK2rRChmdzKqF1vCwMEUiVzLQ@mail.gmail.com>
Subject: and one more
From: Phil <phil@example.com>
To: ntfy-mytopic@ntfy.sh
Content-Type: multipart/alternative; boundary="000000000000f3320b05d42915c9"

--000000000000f3320b05d42915c9
Content-Type: text/html; charset="UTF-8"

html, ignore me

--000000000000f3320b05d42915c9
Content-Type: text/plain; charset="UTF-8"
Content-Transfer-Encoding: quoted-printable

what's
=C3=A0&=C3=A9"'(-=C3=A8_=C3=A7=C3=A0)
=3D=3D=3D=3D=3D
up

--000000000000f3320b05d42915c9--
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "and one more", r.Header.Get("Title"))
		require.Equal(t, `what's
à&é"'(-è_çà)
=====
up`, readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_NestedMultipartBase64(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: test@mydomain.me
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Content-Type: multipart/mixed; boundary="===============2138658284696597373=="
MIME-Version: 1.0
Subject: TrueNAS truenas.local: TrueNAS Test Message hostname: truenas.local
From: =?utf-8?q?Robbie?= <test@mydomain.me>
To: test@mydomain.me
Date: Thu, 16 Feb 2023 01:04:00 -0000
Message-ID: <truenas-20230216.010400.344514.b'8jfL'@truenas.local>

This is a multi-part message in MIME format.
--===============2138658284696597373==
Content-Type: multipart/alternative; boundary="===============2233989480071754745=="
MIME-Version: 1.0

--===============2233989480071754745==
Content-Type: text/plain; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

VGhpcyBpcyBhIHRlc3QgbWVzc2FnZSBmcm9tIFRydWVOQVMgQ09SRS4=

--===============2233989480071754745==
Content-Type: text/html; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

PCFET0NUWVBFIEhUTUwgUFVCTElDICItLy9XM0MvL0RURCBIVE1MIDQuMCBUcmFuc2l0aW9uYWwv
L0VOIj4KClRoaXMgaXMgYSB0ZXN0IG1lc3NhZ2UgZnJvbSBUcnVlTkFTIENPUkUuCg==

--===============2233989480071754745==--

--===============2138658284696597373==--
.
`

	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "TrueNAS truenas.local: TrueNAS Test Message hostname: truenas.local", r.Header.Get("Title"))
		require.Equal(t, "This is a test message from TrueNAS CORE.", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_NestedMultipartTooDeep(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: test@mydomain.me
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Content-Type: multipart/mixed; boundary="===============1=="
MIME-Version: 1.0
Subject: TrueNAS truenas.local: TrueNAS Test Message hostname: truenas.local
From: =?utf-8?q?Robbie?= <test@mydomain.me>
To: test@mydomain.me
Date: Thu, 16 Feb 2023 01:04:00 -0000
Message-ID: <truenas-20230216.010400.344514.b'8jfL'@truenas.local>

This is a multi-part message in MIME format.
--===============1==
Content-Type: multipart/alternative; boundary="===============2=="
MIME-Version: 1.0

--===============2==
Content-Type: multipart/alternative; boundary="===============3=="
MIME-Version: 1.0

--===============3==
Content-Type: text/plain; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

VGhpcyBpcyBhIHRlc3QgbWVzc2FnZSBmcm9tIFRydWVOQVMgQ09SRS4=

--===============3==
Content-Type: text/html; charset="utf-8"
MIME-Version: 1.0
Content-Transfer-Encoding: base64

PCFET0NUWVBFIEhUTUwgUFVCTElDICItLy9XM0MvL0RURCBIVE1MIDQuMCBUcmFuc2l0aW9uYWwv
L0VOIj4KClRoaXMgaXMgYSB0ZXN0IG1lc3NhZ2UgZnJvbSBUcnVlTkFTIENPUkUuCg==

--===============3==--

--===============2==--

--===============1==--
.
`

	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("This should not be called")
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "554 5.0.0 Error: transaction failed, blame it on the weather: multipart message nested too deep")
}

func TestSmtpBackend_HTMLEmail(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: test@mydomain.me
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Message-Id: <51610934ss4.mmailer@fritz.box>
From: <email@email.com>
To: <email@email.com>,
	<ntfy-subjectatntfy@ntfy.sh>
Date: Thu, 30 Mar 2023 02:56:53 +0000
Subject: A HTML email
Mime-Version: 1.0
Content-Type: text/html;
	charset="utf-8"
Content-Transfer-Encoding: quoted-printable

<=21DOCTYPE html>
<html>
<head>
<title>Alerttitle</title>
<meta http-equiv=3D"content-type" content=3D"text/html;charset=3Dutf-8"/>
</head>
<body style=3D"color: =23000000; background-color: =23f0eee6;">
<table width=3D"100%" align=3D"center" style=3D"border:solid 2px =23eeeeee=
; border-collapse: collapse;">
<tr>
<td>
<table style=3D"border-collapse: collapse;">







<tr>
<td style=3D"background: =23FFFFFF;">
<table style=3D"color: =23FFFFFF; background-color: =23006EC0; border-coll=
apse: collapse;">
<tr>
<td style=3D"width: 1000px; text-align: center; font-size: 18pt; font-fami=
ly: Arial, Helvetica, sans-serif; padding: 10px;">


headertext of table

</td>
</tr>
</table>
</td>
</tr>







<tr>
<td style=3D"padding: 10px 20px; background: =23FFFFFF;">
<table style=3D"border-collapse: collapse;">
<tr>
<td style=3D"width: 940px; font-size: 13pt; font-family: Arial, Helvetica,=
 sans-serif; text-align: left;">
" Very important information about a change in your
home automation setup 

Now the light is on
</td>
</tr>
</table>
</td>
</tr>



<tr>
<td style=3D"padding: 10px 20px; background: =23FFFFFF;">
<table>
<tr>
<td style=3D"width: 960px; font-size: 10pt; font-family: Arial, Helvetica,=
 sans-serif; text-align: left;">
<hr />
If you don't want to receive this message anymore, stop the push
 services in your <a href=3D"https:fritzbox" target=3D"_=
blank">FRITZ=21Box</a>=2E<br />
Here you can see the active push services: "System > Push Service"=2E
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<table style=3D"color: =23FFFFFF; background-color: =23006EC0;">
<tr>
<td style=3D"width: 1000px; font-size: 10pt; font-family: Arial, Helvetica=
, sans-serif; text-align: center; padding: 10px;">
This mail has ben sent by your <a style=3D"color: =23FFFFFF;" href=3D"https:=
//fritzbox" target=3D"_blank">FRITZ=21Box</a=
> automatically=2E
</td>
</tr>
</table>
</td>
</tr>
</table>
</td>
</tr>
</table>
</body>
</html>
.
`

	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "A HTML email", r.Header.Get("Title"))
		expected := `headertext of table

&#34; Very important information about a change in your
home automation setup

Now the light is on

If you don&#39;t want to receive this message anymore, stop the push
 services in your  FRITZ!Box . 
Here you can see the active push services: &#34;System &gt; Push Service&#34;.

This mail has ben sent by your  FRITZ!Box  automatically.`
		require.Equal(t, expected, readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

const spamEmail = `
EHLO example.com
MAIL FROM: test@mydomain.me
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Delivered-To: somebody@gmail.com
Received: by 2002:a05:651c:1248:b0:2bf:c263:285 with SMTP id h8csp1096496ljh;
        Mon, 30 Oct 2023 06:23:08 -0700 (PDT)
X-Google-Smtp-Source: AGHT+IFsB3WqbwbeefbeefbeefbeefbeefiXRNDHnIy2xBeaYHZCM3EC8DfPv55qDtgq9djTeBCF
X-Received: by 2002:a05:6808:147:b0:3af:66e5:5d3c with SMTP id h7-20020a056808014700b003af66e55d3cmr11662458oie.26.1698672188132;
        Mon, 30 Oct 2023 06:23:08 -0700 (PDT)
ARC-Seal: i=1; a=rsa-sha256; t=1698672188; cv=none;
        d=google.com; s=arc-20160816;
        b=XM96KvnTbr4h6bqrTPTuuDNXmFCr9Be/HvVhu+UsSQjP9RxPk0wDTPUPZ/HWIJs52y
         beeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeef
         BUmQ==
ARC-Message-Signature: i=1; a=rsa-sha256; c=relaxed/relaxed; d=google.com; s=arc-20160816;
        h=list-unsubscribe-post:list-unsubscribe:mime-version:subject:to
         :reply-to:from:date:message-id:dkim-signature:dkim-signature;
        bh=BERwBIp6fBgrZePFKQjyNMmgPkcnq1Zy1jPO8M0T4Ok=;
        fh=+kTCcNpX22TOI/SVSLygnrDqWeUt4zW7QKiv0TOVSGs=;
        b=lyIBRuOxPOTY2s36OqP7M7awlBKd4t5PX9mJOEJB0eTnTZqML+cplrXUIg2ZTlAAi9
         beeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeef
         tgVQ==
ARC-Authentication-Results: i=1; mx.google.com;
       dkim=pass header.i=@spamspam.com header.s=2020294246 header.b=G8y6xmtK;
       dkim=pass header.i=@auth.ccsend.com header.s=1000073432 header.b=ht8IksVK;
       spf=pass (google.com: domain of aigxeklyirlg+dvwkrmsgua==_1133104752381_suqcukvbeeynm/owplvdba==@in.constantcontact.com designates 208.75.123.226 as permitted sender) smtp.mailfrom="AigXeKlyIRLG+DvWkRMsGUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com";
       dmarc=pass (p=QUARANTINE sp=QUARANTINE dis=NONE) header.from=spamspam.com
Return-Path: <AigXeKlyIRLG+DvWkRMsGUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com>
Received: from ccm30.constantcontact.com (ccm30.constantcontact.com. [208.75.123.226])
        by mx.google.com with ESMTPS id h2-20020a05620a21c200b0076eeed38118si5450962qka.131.2023.10.30.06.23.07
        for <somebody@gmail.com>
        (version=TLS1_2 cipher=ECDHE-ECDSA-AES128-GCM-SHA256 bits=128/128);
        Mon, 30 Oct 2023 06:23:08 -0700 (PDT)
Received-SPF: pass (google.com: domain of aigxeklyirlg+dvwkrmsgua==_1133104752381_suqcukvbeeynm/owplvdba==@in.constantcontact.com designates 208.75.123.226 as permitted sender) client-ip=208.75.123.226;
Authentication-Results: mx.google.com;
       dkim=pass header.i=@spamspam.com header.s=2020294246 header.b=G8y6xmtK;
       dkim=pass header.i=@auth.ccsend.com header.s=1000073432 header.b=ht8IksVK;
       spf=pass (google.com: domain of aigxeklyirlg+dvwkrmsgua==_1133104752381_suqcukvbeeynm/owplvdba==@in.constantcontact.com designates 208.75.123.226 as permitted sender) smtp.mailfrom="AigXeKlyIRLG+DvWkRMsGUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com";
       dmarc=pass (p=QUARANTINE sp=QUARANTINE dis=NONE) header.from=spamspam.com
Return-Path: <AigXeKlyIRLG+DvWkRMsGUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com>
Received: from [10.252.0.3] ([10.252.0.3:53254] helo=p2-jbemailsyndicator12.ctct.net) by 10.249.225.20 (envelope-from <AigXeKlyIRLG+DvWkRMsGUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com>) (ecelerity 4.3.1.999 r(:)) with ESMTP id A4/82-60517-B3EAF356; Mon, 30 Oct 2023 09:23:07 -0400
DKIM-Signature: v=1; q=dns/txt; a=rsa-sha256; c=relaxed/relaxed; s=2020294246; d=spamspam.com; h=date:mime-version:subject:X-Feedback-ID:X-250ok-CID:message-id:from:reply-to:list-unsubscribe:list-unsubscribe-post:to; bh=BERwBIp6fBgrZePFKQjyNMmgPkcnq1Zy1jPO8M0T4Ok=; b=G8y6xmtKv8asfEXA9o8dP+6foQjclo6j5sFREYVIJBbj5YJ5tqoiv5B04/qoRkoTBFDhmjt+BUua7AqDgPSnwbP2iPSA4fTJehnHhut1PyVUp/9vqSYlhxQehfdhma8tPg8ArKfYIKmfKJwKRaQBU0JHCaB1m+5LNQQX3UjkxAg=
DKIM-Signature: v=1; q=dns/txt; a=rsa-sha256; c=relaxed/relaxed; s=1000073432; d=auth.ccsend.com; h=date:mime-version:subject:X-Feedback-ID:X-250ok-CID:message-id:from:reply-to:list-unsubscribe:list-unsubscribe-post:to; bh=BERwBIp6fBgrZePFKQjyNMmgPkcnq1Zy1jPO8M0T4Ok=; b=ht8IksVKYY/Kb3dUERWoeW4eVdYjKL6F4PEoIZOhfFXor6XAIbPnd3A/CPmbmoqFZjnKh5OdcUy1N5qEoj8w1Q3TmN8/ySQkqrlrmSDSZIHZMY7Qp9/TJrqUe4RMFOO1KKIN6Y0vGP1+dWe98msMAHwvi2qMjG9aEKLfFr2JUTQ=
Message-ID: <1140728754828.1133104752381.1941549819.0.260913JL.2002@synd.ccsend.com>
Date: Mon, 30 Oct 2023 09:23:07 -0400 (EDT)
From: spamspam Loan Servicing <marklake@spamspam.com>
Reply-To: marklake@spamspam.com
To: somebody@gmail.com
Subject: Buying a home? You deserve the confidence of Pre-Approval
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="----=_Part_75055660_144854819.1698672187348"
List-Unsubscribe: <https://visitor.constantcontact.com/do?p=un&m=beefbeefbeef>
List-Unsubscribe-Post: List-Unsubscribe=One-Click
X-Campaign-Activity-ID: 8a05de2a-5c88-44b1-be0e-f5a444cb0650
X-250ok-CID: 8a05de2a-5c88-44b1-be0e-f5a444cb0650
X-Channel-ID: b1441c50-a541-11ec-a79b-fa163e5bc304
X-Return-Path-Hint: AbeefbeefbeefbeefbeefUA==_1133104752381_sUQcUKVBEeynm/oWPlvDBA==@in.constantcontact.com
X-Roving-Campaignid: 1140728754811
X-Roving-Id: 1133104752381.1111111111
X-Feedback-ID: b1441c50-a541-11ec-beef-beefbeefbeefbeef5de2a-5c88-44b1-be0e-f5a444cb0650:1133104752381:CTCT
X-CTCT-ID: b13a9586-a541-11ec-beef-beefbeefbeef

------=_Part_75055660_144854819.1698672187348
Content-Type: text/plain; charset=utf-8
Content-Transfer-Encoding: quoted-printable

When you're buying a home, Pre-Approval gives you confidence you're in the =
right price range and shows sellers you mean business. xxxxxxxxx SELLING or=
 BUYING? Call: 844-590-2275 Get Your Homebuying PRE-APPROVAL IN 24-HOURS* G=
et Pre-Approved When you're buying a home, Pre-Approval gives you confidenc=
e you're in the right price range and shows sellers you mean business. xxx=
xxxxxxGet Pre-Approved today! Click or Call to Get Pre-Approved 844-590-227=
5 Get Pre-Approved nmlsconsumeraccess.org/ *The 24 hour timeframe is for mo=
st approvals, however if additional information is needed or a request is o=
n a holiday, the time for preapproval may be greater than 24 hours. This em=
ail is for informational purposes only and is not an offer, loan approval o=
r loan commitment. Mortgage rates are subject to change without notice. Som=
e terms and restrictions may apply to certain loan programs. Refinancing ex=
isting loans may result in total finance charges being higher over the life=
 of the loan, reduction in payments may partially reflect a longer loan ter=
m. This information is provided as guidance and illustrative purposes only =
and does not constitute legal or financial advice. We are not liable or bou=
nd legally for any answers provided to any user for our process or position=
 on an issue. This information may change from time to time and at any time=
 without notification. The most current information will be updated periodi=
cally and posted in the online forum. spamspam Loan Servicing, LLC. NMLS#39=
1521. nmlsconsumeraccess.org. You are receiving this information as a curre=
nt loan customer with spamspam Loan Servicing, LLC. Not licensed for lendin=
g activities in any of the U.S. territories. Not authorized to originate lo=
ans in the State of New York. Licensed by the Dept. of Financial Protection=
 and Innovation under the California Residential Mortgage .Lending Act #413=
1216. This email was sent to somebody@gmail.com Version 103023PCHPrAp=
9 xxxxxxxxx spamspam Loan Servicing | 4425 Ponce de Leon Blvd 5-251, Coral =
Gables, FL 33146-1837 Unsubscribe somebody@gmail.com Update Profile |=
 Our Privacy Policy | Constant Contact Data Notice Sent by marklake@spamspa=
m.com
------=_Part_75055660_144854819.1698672187348
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<!DOCTYPE HTML>
<html lang=3D"en-US"> <head>  <meta http-equiv=3D"Content-Type" content=3D"=
text/html; charset=3Dutf-8"> <meta name=3D"viewport" content=3D"width=3Ddev=
ice-width, initial-scale=3D1, maximum-scale=3D1">   <style type=3D"text/css=
" data-premailer=3D"ignore">=20
@media only screen and (max-width:480px) { .footer-main-width { width: 100%=
 !important; }  .footer-mobile-hidden { display: none !important; }  .foote=
r-mobile-hidden { display: none !important; }  .footer-column { display: bl=
ock !important; }  .footer-mobile-stack { display: block !important; }  .fo=
oter-mobile-stack-padding { padding-top: 3px; } }=20
/* IE: correctly scale images with w/h attbs */ img { -ms-interpolation-mod=
e: bicubic; }=20
.layout { min-width: 100%; }=20
table { table-layout: fixed; } .shell_outer-row { table-layout: auto; }=20
/* Gmail/Web viewport fix */ u + .body .shell_outer-row { width: 620px; }=
=20
/* LIST AND p STYLE OVERRIDES */ .text .text_content-cell p { margin: 0; pa=
dding: 0; margin-bottom: 0; } .text .text_content-cell ul, .text .text_cont=
ent-cell ol { padding: 0; margin: 0 0 0 40px; } .text .text_content-cell li=
 { padding: 0; margin: 0; /* line-height: 1.2; Remove after testing */ } /*=
 Text Link Style Reset */ a { text-decoration: underline; } /* iOS: Autolin=
k styles inherited */ a[x-apple-data-detectors] { text-decoration: underlin=
e !important; font-size: inherit !important; font-family: inherit !importan=
t; font-weight: inherit !important; line-height: inherit !important; color:=
 inherit !important; } /* FF/Chrome: Smooth font rendering */ .text .text_c=
ontent-cell { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing:=
 grayscale; }=20
</style> <!--[if gte mso 9]> <style id=3D"ol-styles">=20
/* OUTLOOK-SPECIFIC STYLES */ li { text-indent: -1em; padding: 0; margin: 0=
; /* line-height: 1.2; Remove after testing */ } ul, ol { padding: 0; margi=
n: 0 0 0 40px; } p { margin: 0; padding: 0; margin-bottom: 0; }=20
</style> <![endif]-->  <style>@media only screen and (max-width:480px) {
.button_content-cell {
padding-top: 10px !important; padding-right: 20px !important; padding-botto=
m: 10px !important; padding-left: 20px !important;
}
.button_border-row .button_content-cell {
padding-top: 10px !important; padding-right: 20px !important; padding-botto=
m: 10px !important; padding-left: 20px !important;
}
.column .content-padding-horizontal {
padding-left: 20px !important; padding-right: 20px !important;
}
.layout .column .content-padding-horizontal .content-padding-horizontal {
padding-left: 0px !important; padding-right: 0px !important;
}
.layout .column .content-padding-horizontal .block-wrapper_border-row .cont=
ent-padding-horizontal {
padding-left: 20px !important; padding-right: 20px !important;
}
.dataTable {
overflow: auto !important;
}
.dataTable .dataTable_content {
width: auto !important;
}
.image--mobile-scale .image_container img {
width: auto !important;
}
.image--mobile-center .image_container img {
margin-left: auto !important; margin-right: auto !important;
}
.layout-margin .layout-margin_cell {
padding: 0px 20px !important;
}
.layout-margin--uniform .layout-margin_cell {
padding: 20px 20px !important;
}
.scale {
width: 100% !important;
}
.stack {
display: block !important; box-sizing: border-box;
}
.hide {
display: none !important;
}
u + .body .shell_outer-row {
width: 100% !important;
}
.socialFollow_container {
text-align: center !important;
}
.text .text_content-cell {
font-size: 16px !important;
}
.text .text_content-cell h1 {
font-size: 24px !important;
}
.text .text_content-cell h2 {
font-size: 20px !important;
}
.text .text_content-cell h3 {
font-size: 20px !important;
}
.text--sectionHeading .text_content-cell {
font-size: 26px !important;
}
.text--heading .text_content-cell {
font-size: 26px !important;
}
.text--feature .text_content-cell h2 {
font-size: 20px !important;
}
.text--articleHeading .text_content-cell {
font-size: 20px !important;
}
.text--article .text_content-cell h3 {
font-size: 20px !important;
}
.text--featureHeading .text_content-cell {
font-size: 20px !important;
}
.text--feature .text_content-cell h3 {
font-size: 20px !important;
}
.text--dataTable .text_content-cell .dataTable .dataTable_content-cell {
font-size: 12px !important;
}
.text--dataTable .text_content-cell .dataTable th.dataTable_content-cell {
font-size: px !important;
}
}
</style>
</head> <body class=3D"body template template--en-US" data-template-version=
=3D"1.38.0" data-canonical-name=3D"CPE10001" lang=3D"en-US" align=3D"center=
" style=3D"-ms-text-size-adjust: 100%; -webkit-text-size-adjust: 100%; min-=
width: 100%; width: 100%; margin: 0px; padding: 0px;"> <div id=3D"preheader=
" style=3D"color: transparent; display: none; font-size: 1px; line-height: =
1px; max-height: 0px; max-width: 0px; opacity: 0; overflow: hidden;"><span =
data-entity-ref=3D"preheader">When you&#x27;re buying a home, Pre-Approval =
gives you confidence you&#x27;re in the right price range and shows sellers=
 you mean business. </span></div> <div id=3D"tracking-image" style=3D"color=
: transparent; display: none; font-size: 1px; line-height: 1px; max-height:=
 0px; max-width: 0px; opacity: 0; overflow: hidden;"><img src=3D"https://r2=
0.rs6.net/on.jsp?ca=beefbeefbe-beef-44b1-be0e-f5a444cb0650&a=3D113310475238=
1&c=3Db13a9586-a541-11ec-a79b-fa163e5bc304&ch=3Db1441c50-a541-11ec-a79b-fa1=
63e5bc304" / alt=3D""></div> <div class=3D"shell" lang=3D"en-US" style=3D"b=
ackground-color: #015288;">  <table class=3D"shell_panel-row" width=3D"100%=
" border=3D"0" cellpadding=3D"0" cellspacing=3D"0" style=3D"background-colo=
r: #015288;" bgcolor=3D"#015288"> <tr class=3D""> <td class=3D"shell_panel-=
cell" style=3D"" align=3D"center" valign=3D"top"> <table class=3D"shell_wid=
th-row scale" style=3D"width: 620px;" align=3D"center" border=3D"0" cellpad=
ding=3D"0" cellspacing=3D"0"> <tr> <td class=3D"shell_width-cell" style=3D"=
padding: 15px 10px;" align=3D"center" valign=3D"top"> <table class=3D"shell=
_content-row" width=3D"100%" align=3D"center" border=3D"0" cellpadding=3D"0=
" cellspacing=3D"0"> <tr> <td class=3D"shell_content-cell" style=3D"border-=
radius: 0px; background-color: #FFFFFF; padding: 0; border: 0px solid #0096=
d6;" align=3D"center" valign=3D"top" bgcolor=3D"#FFFFFF"> <table class=3D"l=
ayout layout--1-column" style=3D"table-layout: fixed;" width=3D"100%" borde=
r=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column colum=
n--1 scale stack" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"divider" width=3D"100%" cellpadding=3D"0" cellspacing=3D"0"=
 border=3D"0"> <tr> <td class=3D"divider_container" style=3D"padding-top: 0=
px; padding-bottom: 10px;" width=3D"100%" align=3D"center" valign=3D"top"> =
<table class=3D"divider_content-row" style=3D"width: 100%; height: 1px;" ce=
llpadding=3D"0" cellspacing=3D"0" border=3D"0"> <tr> <td class=3D"divider_c=
ontent-cell" style=3D"padding-bottom: 5px; height: 1px; line-height: 1px; b=
ackground-color: #0096D6; border-bottom-width: 0px;" height=3D"1" align=3D"=
center" bgcolor=3D"#0096D6"> <img alt=3D"" width=3D"5" height=3D"1" border=
=3D"0" hspace=3D"0" vspace=3D"0" src=3D"https://imgssl.constantcontact.com/=
letters/images/1101116784221/S.gif" style=3D"display: block; height: 1px; w=
idth: 5px;"> </td> </tr> </table> </td> </tr> </table> </td> </tr> </table>=
 <table class=3D"layout layout--1-column" style=3D"table-layout: fixed;" wi=
dth=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td cla=
ss=3D"column column--1 scale stack" style=3D"width: 100%;" align=3D"center"=
 valign=3D"top"><div class=3D"spacer" style=3D"line-height: 10px; height: 1=
0px;">&#x200a;</div></td> </tr> </table> <table class=3D"layout layout--1-c=
olumn" style=3D"table-layout: fixed;" width=3D"100%" border=3D"0" cellpaddi=
ng=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column column--1 scale stack"=
 style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"image image--padding-vertical image--mobile-scale image--mo=
bile-center" width=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0=
"> <tr> <td class=3D"image_container" align=3D"center" valign=3D"top" style=
=3D"padding-top: 10px; padding-bottom: 10px;"> <a href=3D"https://r20.rs6.n=
et/tn.jsp?f=3D001YKO1VR2jLW0SuSLZLfN7qCP9AwEGO0v-Vy-0SCUlMWvTEiCsv-QEMhmJe9=
ch=3DHu9wLy0fth6D8jxFBWPA_NhdnWcZZPivk0KUTgRJoVIo_si10jiydw=3D=3D" data-tra=
ckable=3D"true"><img data-image-content class=3D"image_content" width=3D"26=
2" src=3D"https://files.constantcontact.com/beefbeefbee/057bff2a-bdba-4165-=
b108-a7baa91c42c6.jpg" alt=3D"" style=3D"display: block; height: auto; max-=
width: 100%;"></a> </td> </tr> </table> </td> </tr> </table> <table class=
=3D"layout layout--heading layout--1-column" style=3D"background-color: #00=
527e; table-layout: fixed;" width=3D"100%" border=3D"0" cellpadding=3D"0" c=
ellspacing=3D"0" bgcolor=3D"#00527e"> <tr> <td class=3D"column column--1 sc=
ale stack" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"text text--padding-vertical" width=3D"100%" border=3D"0" ce=
llpadding=3D"0" cellspacing=3D"0" style=3D"table-layout: fixed;"> <tr> <td =
class=3D"text_content-cell content-padding-horizontal" style=3D"text-align:=
 center; font-family: Arial,Verdana,Helvetica,sans-serif; color: #000000; f=
ont-size: 14px; line-height: 1.2; display: block; word-wrap: break-word; pa=
dding: 10px 20px;" align=3D"center" valign=3D"top">
<h1 style=3D"font-family: Arial,Verdana,Helvetica,sans-serif; color: #606d7=
8; font-size: 26px; font-weight: bold; margin: 0;"><span style=3D"color: rg=
b(0, 150, 214);">SELLING or BUYING?</span></h1>
<p style=3D"margin: 0;"><span style=3D"font-size: 16px; color: rgb(255, 255=
, 255); font-weight: bold;">Call: 844-590-2275</span></p>
</td> </tr> </table> </td> </tr> </table> <table class=3D"layout layout--ar=
ticle layout--1-column" style=3D"table-layout: fixed;" width=3D"100%" borde=
r=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column colum=
n--1 scale stack" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"text text--heading text--padding-vertical" width=3D"100%" b=
order=3D"0" cellpadding=3D"0" cellspacing=3D"0" style=3D"table-layout: fixe=
d;"> <tr> <td class=3D"text_content-cell content-padding-horizontal" style=
=3D"text-align: center; font-family: Arial,Verdana,Helvetica,sans-serif; co=
lor: #606d78; font-size: 26px; line-height: 1.2; display: block; word-wrap:=
 break-word; font-weight: bold; padding: 10px 20px;" align=3D"center" valig=
n=3D"top">
<p style=3D"margin: 0;"><span style=3D"font-size: 30px; color: rgb(0, 150, =
214);">Get Your Homebuying</span></p>
<p style=3D"margin: 0;"><span style=3D"font-size: 30px; color: rgb(0, 82, 1=
26);">PRE-APPROVAL IN 24-HOURS</span><span style=3D"font-size: 30px; color:=
 rgb(0, 82, 126); font-weight: normal;">*</span></p>
</td> </tr> </table> <table class=3D"image image--padding-vertical image--m=
obile-scale image--mobile-center" width=3D"100%" border=3D"0" cellpadding=
=3D"0" cellspacing=3D"0"> <tr> <td class=3D"image_container content-padding=
-horizontal" align=3D"center" valign=3D"top" style=3D"padding: 10px 20px;">=
 <img data-image-content class=3D"image_content" width=3D"548" src=3D"https=
://files.constantcontact.com/df66e42d701/2092a2d7-0bda-4289-910b-bf50a2398d=
60.jpg" alt=3D"" style=3D"display: block; height: auto; max-width: 100%;"> =
</td> </tr> </table>  <table class=3D"button button--padding-vertical" widt=
h=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0" style=3D"table-=
layout: fixed;"> <tr> <td class=3D"button_container content-padding-horizon=
tal" align=3D"center" style=3D"padding: 10px 20px;">    <table class=3D"but=
ton_content-row" style=3D"width: inherit; border-radius: 3px; border-spacin=
g: 0; background-color: #0096D6; border: none;" border=3D"0" cellpadding=3D=
"0" cellspacing=3D"0" bgcolor=3D"#0096D6"> <tr> <td class=3D"button_content=
-cell" style=3D"padding: 10px 40px;" align=3D"center"> <a class=3D"button_l=
ink" href=3D"https://r20.rs6.net/tn.jsp?f=3D001YKO1VR2jLW0SuSLZLfN7qCP9AwEG=
O0v-Vy-0SCUlMWvTEiCsv-QEMuu9ZVVi6WGHhCias4f7-QkeggQvxIvbs-6TTaZHHhXLKf88NID=
dci4Ge7aYN-QihEgqblie1-DQ2Fa1BKLbT3AM8rtrgeYQgVxJ6cG8POsvFzv7JstrGkCkg3a3AE=
633LfQpAddyVLFkTv6oyS4T2j_YjYIPKDOZktqK_5rOR-Fh8cWGtUD8YPpPNnZ037z6_t9Nkemu=
hxG&c=3DA65qX-dQJPS0J4afCS7H0Je5N-_6Q8Nh2fNHkb5-5biUYd5B9SY3zA=3D=3D&ch=3DH=
u9wLy0fth6D8jxFBWPA_NhdnWcZZPivk0KUTgRJoVIo_si10jiydw=3D=3D" data-trackable=
=3D"true" style=3D"font-size: 16px; font-weight: bold; color: #FFFFFF; font=
-family: Helvetica,Arial,sans-serif; word-wrap: break-word; text-decoration=
: none;">Get Pre-Approved</a> </td> </tr> </table>    </td> </tr> </table> =
  <table class=3D"text text--padding-vertical" width=3D"100%" border=3D"0" =
cellpadding=3D"0" cellspacing=3D"0" style=3D"table-layout: fixed;"> <tr> <t=
d class=3D"text_content-cell content-padding-horizontal" style=3D"line-heig=
ht: 1; text-align: center; font-family: Arial,Verdana,Helvetica,sans-serif;=
 color: #000000; font-size: 14px; display: block; word-wrap: break-word; pa=
dding: 10px 20px;" align=3D"center" valign=3D"top">
<p style=3D"text-align: left; margin: 0;" align=3D"left"><br></p>
<p style=3D"margin: 0;"><span style=3D"font-size: 19px;">When you're buying=
 a home, Pre-Approval gives you confidence you're in the right price range =
and shows sellers you mean business. </span></p>
<p style=3D"margin: 0;"><span style=3D"font-size: 19px;">&#xfeff;Get Pre-Ap=
proved today!</span></p>
</td> </tr> </table> </td> </tr> </table> <table class=3D"layout layout--1-=
column" style=3D"table-layout: fixed;" width=3D"100%" border=3D"0" cellpadd=
ing=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column column--1 scale stack=
" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"text text--padding-vertical" width=3D"100%" border=3D"0" ce=
llpadding=3D"0" cellspacing=3D"0" style=3D"table-layout: fixed;"> <tr> <td =
class=3D"text_content-cell content-padding-horizontal" style=3D"text-align:=
 left; font-family: Arial,Verdana,Helvetica,sans-serif; color: #000000; fon=
t-size: 14px; line-height: 1.2; display: block; word-wrap: break-word; padd=
ing: 10px 20px;" align=3D"left" valign=3D"top">
<p style=3D"text-align: center; margin: 0;" align=3D"center"><br></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 23px; color: rgb(0, 82, 126); font-weight: bold; font-family: A=
rial, Verdana, Helvetica, sans-serif;">Click or Call to Get Pre-Approved </=
span></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 28px; color: rgb(0, 150, 214); font-weight: bold;">844-590-2275=
</span></p>
</td> </tr> </table> </td> </tr> </table> <table class=3D"layout layout--1-=
column" style=3D"table-layout: fixed;" width=3D"100%" border=3D"0" cellpadd=
ing=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column column--1 scale stack=
" style=3D"width: 100%;" align=3D"center" valign=3D"top"> <table class=3D"b=
utton button--padding-vertical" width=3D"100%" border=3D"0" cellpadding=3D"=
0" cellspacing=3D"0" style=3D"table-layout: fixed;"> <tr> <td class=3D"butt=
on_container content-padding-horizontal" align=3D"center" style=3D"padding:=
 10px 20px;">    <table class=3D"button_content-row" style=3D"background-co=
lor: #0096D6; width: inherit; border-radius: 3px; border-spacing: 0; border=
: none;" border=3D"0" cellpadding=3D"0" cellspacing=3D"0" bgcolor=3D"#0096D=
6"> <tr> <td class=3D"button_content-cell" style=3D"padding: 10px 40px;" al=
ign=3D"center"> <a class=3D"button_link" href=3D"https://r20.rs6.net/tn.jsp=
?f=3D001thisisfakethisisfakethisisfakev-Vy-0SCUlMWvTEiCsv-QEMuu9ZVVi6WGHhCi=
oVIo_si10jiydw=3D=3D" data-trackable=3D"true" style=3D"font-size: 16px; fon=
t-weight: bold; color: #FFFFFF; font-family: Helvetica,Arial,sans-serif; wo=
rd-wrap: break-word; text-decoration: none;">Get Pre-Approved</a> </td> </t=
r> </table>    </td> </tr> </table>   </td> </tr> </table> <table class=3D"=
layout layout--1-column" style=3D"table-layout: fixed;" width=3D"100%" bord=
er=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column colu=
mn--1 scale stack" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"image image--padding-vertical image--mobile-scale image--mo=
bile-center" width=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0=
"> <tr> <td class=3D"image_container" align=3D"center" valign=3D"top" style=
=3D"padding-top: 10px; padding-bottom: 10px;"> <img data-image-content clas=
s=3D"image_content" width=3D"87" src=3D"https://files.constantcontact.com/d=
f66e42d701/beefbeef-beef-beef-9a13-2779ab497b8d.png" alt=3D"" style=3D"disp=
lay: block; height: auto; max-width: 100%;"> </td> </tr> </table> </td> </t=
r> </table> <table class=3D"layout layout--1-column" style=3D"table-layout:=
 fixed;" width=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <=
tr> <td class=3D"column column--1 scale stack" style=3D"width: 100%;" align=
=3D"center" valign=3D"top">
<table class=3D"text text--padding-vertical" width=3D"100%" border=3D"0" ce=
llpadding=3D"0" cellspacing=3D"0" style=3D"table-layout: fixed;"> <tr> <td =
class=3D"text_content-cell content-padding-horizontal" style=3D"text-align:=
 left; font-family: Arial,Verdana,Helvetica,sans-serif; color: #000000; fon=
t-size: 14px; line-height: 1.2; display: block; word-wrap: break-word; padd=
ing: 10px 20px;" align=3D"left" valign=3D"top">
<p style=3D"text-align: center; margin: 0;" align=3D"center"><br></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><a href=3D"htt=
ps://r20.rs6.net/tn.jsp?f=3D001YKO1VR2jLW0SuSLZLfN7qCP9AwEGO0v-Vy-0SCUlMWvT=
EiCsv-QEMgYju54LKeEV1_a2OCyOAfG7VhZpxtOW89WM-s6S5iiXcmnbK-Z6XDc9LL569h6DE4L=
IRMWiBWHOlFB9TZWQVuX6Ycz3505y1keCrca4QArp&c=3DA65qX-dQJPS0J4afCS7H0Je5N-_6Q=
8Nh2fNHkb5-5biUYd5B9SY3zA=3D=3D&ch=3DHu9wLy0fth6D8jxFBWPA_NhdnWcZZPivk0KUTg=
RJoVIo_si10jiydw=3D=3D" target=3D"_blank" style=3D"font-size: 11px; color: =
rgb(153, 153, 153); text-decoration: underline; font-weight: normal; font-s=
tyle: normal;">nmlsconsumeraccess.org/</a></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(153, 153, 153);">*The 24 hour timeframe is for=
 most approvals, however if additional information is needed or a request i=
s on a holiday, the time for preapproval may be greater than 24 hours.</spa=
n></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(153, 153, 153); background-color: rgb(255, 255=
, 255);">This email is for informational purposes only and is not an offer,=
 loan approval or loan commitment. Mortgage rates are subject to change wit=
hout notice. Some terms and restrictions may apply to certain loan programs=
. Refinancing existing loans may result in total finance charges being high=
er over the life of the loan, reduction in payments may partially reflect a=
 longer loan term. This information is provided as guidance and illustrativ=
e purposes only and does not constitute legal or financial advice. We are n=
ot liable or bound legally for any answers provided to any user for our pro=
cess or position on an issue. This information may change from time to time=
 and at any time without notification. The most current information will be=
 updated periodically and posted in the online forum.</span></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(153, 153, 153); background-color: rgb(255, 255=
, 255);">spamspam Loan Servicing, LLC. NMLS#391521. nmlsconsumeraccess.org.=
 You are receiving this information as a current loan customer with spamspa=
m Loan Servicing, LLC. Not licensed for lending activities in any of the U.=
S. territories. Not authorized to originate loans in the State of New York.=
 Licensed by the Dept. of Financial Protection and Innovation under the Cal=
ifornia Residential Mortgage .Lending Act #4131216.</span></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><br></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(153, 153, 153);">This email was sent to <span =
data-id=3D"emailAddress">somebody@gmail.com</span></span></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(153, 153, 153);">Version 103023PCHPrAp9 </span=
></p>
<p style=3D"text-align: center; margin: 0;" align=3D"center"><span style=3D=
"font-size: 11px; color: rgb(162, 162, 162);">&#xfeff;</span></p>
</td> </tr> </table> </td> </tr> </table> <table class=3D"layout layout--1-=
column" style=3D"table-layout: fixed;" width=3D"100%" border=3D"0" cellpadd=
ing=3D"0" cellspacing=3D"0"> <tr> <td class=3D"column column--1 scale stack=
" style=3D"width: 100%;" align=3D"center" valign=3D"top">
<table class=3D"divider" width=3D"100%" cellpadding=3D"0" cellspacing=3D"0"=
 border=3D"0"> <tr> <td class=3D"divider_container" style=3D"padding-top: 1=
0px; padding-bottom: 0px;" width=3D"100%" align=3D"center" valign=3D"top"> =
<table class=3D"divider_content-row" style=3D"width: 100%; height: 1px;" ce=
llpadding=3D"0" cellspacing=3D"0" border=3D"0"> <tr> <td class=3D"divider_c=
ontent-cell" style=3D"padding-bottom: 2px; height: 1px; line-height: 1px; b=
ackground-color: #0096D6; border-bottom-width: 0px;" height=3D"1" align=3D"=
center" bgcolor=3D"#0096D6"> <img alt=3D"" width=3D"5" height=3D"1" border=
=3D"0" hspace=3D"0" vspace=3D"0" src=3D"https://imgssl.constantcontact.com/=
letters/images/1111111111111/S.gif" style=3D"display: block; height: 1px; w=
idth: 5px;"> </td> </tr> </table> </td> </tr> </table> </td> </tr> </table>=
  </td> </tr> </table> </td> </tr> </table> </td> </tr> <tr> <td class=3D"s=
hell_panel-cell shell_panel-cell--systemFooter" style=3D"" align=3D"center"=
 valign=3D"top"> <table class=3D"shell_width-row scale" style=3D"width: 100=
%;" align=3D"center" border=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr>=
 <td class=3D"shell_width-cell" style=3D"padding: 0px;" align=3D"center" va=
lign=3D"top"> <table class=3D"shell_content-row" width=3D"100%" align=3D"ce=
nter" border=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td class=3D"s=
hell_content-cell" style=3D"background-color: #FFFFFF; padding: 0; border: =
0 solid #0096d6;" align=3D"center" valign=3D"top" bgcolor=3D"#FFFFFF"> <tab=
le class=3D"layout layout--1-column" style=3D"table-layout: fixed;" width=
=3D"100%" border=3D"0" cellpadding=3D"0" cellspacing=3D"0"> <tr> <td class=
=3D"column column--1 scale stack" style=3D"width: 100%;" align=3D"center" v=
align=3D"top"> <table class=3D"footer" width=3D"100%" border=3D"0" cellpadd=
ing=3D"0" cellspacing=3D"0" style=3D"font-family: Verdana,Geneva,sans-serif=
; color: #5d5d5d; font-size: 12px;"> <tr> <td class=3D"footer_container" al=
ign=3D"center"> <table class=3D"footer-container" width=3D"100%" cellpaddin=
g=3D"0" cellspacing=3D"0" border=3D"0" style=3D"background-color: #ffffff; =
margin-left: auto; margin-right: auto; table-layout: auto !important;" bgco=
lor=3D"#ffffff">
<tr>
<td width=3D"100%" align=3D"center" valign=3D"top" style=3D"width: 100%;">
<div class=3D"footer-max-main-width" align=3D"center" style=3D"margin-left:=
 auto; margin-right: auto; max-width: 100%;">
<table width=3D"100%" cellpadding=3D"0" cellspacing=3D"0" border=3D"0">
<tr>
<td class=3D"footer-layout" align=3D"center" valign=3D"top" style=3D"paddin=
g: 16px 0px;">
<table class=3D"footer-main-width" style=3D"width: 580px;" border=3D"0" cel=
lpadding=3D"0" cellspacing=3D"0">
<tr>
<td class=3D"footer-text" align=3D"center" valign=3D"top" style=3D"color: #=
5d5d5d; font-family: Verdana,Geneva,sans-serif; font-size: 12px; padding: 4=
px 0px;">
<span class=3D"footer-column">spamspam Loan Servicing<span class=3D"footer-=
mobile-hidden"> | </span></span><span class=3D"footer-column">4425 Ponce de=
 Leon Blvd 5-251<span class=3D"footer-mobile-hidden">, </span></span><span =
class=3D"footer-column"></span><span class=3D"footer-column"></span><span c=
lass=3D"footer-column">Coral Gables, FL 33146-1837</span><span class=3D"foo=
ter-column"></span>
</td>
</tr>
<tr>
<td class=3D"footer-row" align=3D"center" valign=3D"top" style=3D"padding: =
10px 0px;">
<table cellpadding=3D"0" cellspacing=3D"0" border=3D"0">
<tr>
<td class=3D"footer-text" align=3D"center" valign=3D"top" style=3D"color: #=
5d5d5d; font-family: Verdana,Geneva,sans-serif; font-size: 12px; padding: 4=
px 0px;">
<a href=3D"https://visitor.constantcontact.com/do?p=3Dun&m=3D001g3dtlqhzM3v=
-44b1-be0e-f5a444cb0650" data-track=3D"false" style=3D"color: #5d5d5d;">Uns=
ubscribe somebody@gmail.com<span class=3D"partnerOptOut"></span></a>
<span class=3D"partnerOptOut"></span>
</td>
</tr>
<tr>
<td class=3D"footer-text" align=3D"center" valign=3D"top" style=3D"color: #=
5d5d5d; font-family: Verdana,Geneva,sans-serif; font-size: 12px; padding: 4=
px 0px;">
<a href=3D"https://visitor.constantcontact.com/do?p=3Doo&m=3D001g3dtlqhzM3v=
-44b1-be0e-f5a444cb0650" data-track=3D"false" style=3D"color: #5d5d5d;">Upd=
ate Profile</a> |
<a href=3D"https://spamspam.com/privacy-notice/" data-track=3D"false" style=
=3D"color: #5d5d5d;">Our Privacy Policy</a><span class=3D"footer-mobile-hid=
den"> |</span>
<a class=3D"footer-about-provider footer-mobile-stack footer-mobile-stack-p=
adding" href=3D"http://www.constantcontact.com/legal/about-constant-contact=
" data-track=3D"false" style=3D"color: #5d5d5d;">Constant Contact Data Noti=
ce</a>
</td>
</tr>
<tr>
<td class=3D"footer-text" align=3D"center" valign=3D"top" style=3D"color: #=
5d5d5d; font-family: Verdana,Geneva,sans-serif; font-size: 12px; padding: 4=
px 0px;">
Sent by
<a href=3D"mailto:marklake@spamspam.com" style=3D"color: #5d5d5d; text-deco=
ration: none;">marklake@spamspam.com</a>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td class=3D"footer-text" align=3D"center" valign=3D"top" style=3D"color: #=
5d5d5d; font-family: Verdana,Geneva,sans-serif; font-size: 12px; padding: 4=
px 0px;">
</td>
</tr>
</table>
</td>
</tr>
</table>
</div>
</td>
</tr>
</table> </td> </tr> </table>   </td> </tr> </table>  </td> </tr> </table> =
</td> </tr> </table> </td> </tr>  </table> </div>  </body> </html>

------=_Part_75055660_144854819.1698672187348--
.
`

func TestSmtpBackend_Spam_Text(t *testing.T) {
	email := spamEmail
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "Buying a home? You deserve the confidence of Pre-Approval", r.Header.Get("Title"))
		actual := readAll(t, r.Body)
		expected := "When you're buying a home, Pre-Approval gives you confidence you're in the right price range and shows sellers you mean business. xxxxxxxxx SELLING or BUYING? Call: 844-590-2275 Get Your Homebuying PRE-APPROVAL IN 24-HOURS* Get Pre-Approved When you're buying a home, Pre-Approval gives you confidence you're in the right price range and shows sellers you mean business. xxxxxxxxxGet Pre-Approved today! Click or Call to Get Pre-Approved 844-590-2275 Get Pre-Approved nmlsconsumeraccess.org/ *The 24 hour timeframe is for most approvals, however if additional information is needed or a request is on a holiday, the time for preapproval may be greater than 24 hours. This email is for informational purposes only and is not an offer, loan approval or loan commitment. Mortgage rates are subject to change without notice. Some terms and restrictions may apply to certain loan programs. Refinancing existing loans may result in total finance charges being higher over the life of the loan, reduction in payments may partially reflect a longer loan term. This information is provided as guidance and illustrative purposes only and does not constitute legal or financial advice. We are not liable or bound legally for any answers provided to any user for our process or position on an issue. This information may change from time to time and at any time without notification. The most current information will be updated periodically and posted in the online forum. spamspam Loan Servicing, LLC. NMLS#391521. nmlsconsumeraccess.org. You are receiving this information as a current loan customer with spamspam Loan Servicing, LLC. Not licensed for lending activities in any of the U.S. territories. Not authorized to originate loans in the State of New York. Licensed by the Dept. of Financial Protection and Innovation under the California Residential Mortgage .Lending Act #4131216. This email was sent to somebody@gmail.com Version 103023PCHPrAp9 xxxxxxxxx spamspam Loan Servicing | 4425 Ponce de Leon Blvd 5-251, Coral Gables, FL 33146-1837 Unsubscribe somebody@gmail.com Update Profile | Our Privacy Policy | Constant Contact Data Notice Sent by marklake@spamspam.com"
		require.Equal(t, expected, actual)
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_Spam_HTML(t *testing.T) {
	email := strings.ReplaceAll(spamEmail, "text/plain", "text/not-plain-anymore") // We artificially force HTML parsing here
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "Buying a home? You deserve the confidence of Pre-Approval", r.Header.Get("Title"))
		actual := readAll(t, r.Body)
		expected := `When you&#39;re buying a home, Pre-Approval gives you confidence you&#39;re in the right price range and shows sellers you mean business.                                  
                                      ` + "\u200a" + `            

  SELLING or BUYING?  
  Call: 844-590-2275  

  Get Your Homebuying  
  PRE-APPROVAL IN 24-HOURS  *  
                                     Get Pre-Approved                        

  When you&#39;re buying a home, Pre-Approval gives you confidence you&#39;re in the right price range and shows sellers you mean business.   
  ` + "\ufeff" + `Get Pre-Approved today!  

  Click or Call to Get Pre-Approved   
  844-590-2275  
                                  Get Pre-Approved                              

  nmlsconsumeraccess.org/  
  *The 24 hour timeframe is for most approvals, however if additional information is needed or a request is on a holiday, the time for preapproval may be greater than 24 hours.  
  This email is for informational purposes only and is not an offer, loan approval or loan commitment. Mortgage rates are subject to change without notice. Some terms and restrictions may apply to certain loan programs Refinancing existing loans may result in total finance charges being higher over the life of the loan, reduction in payments may partially reflect a longer loan term. This information is provided as guidance and illustrative purposes only and does not constitute legal or financial advice. We are not liable or bound legally for any answers provided to any user for our process or position on an issue. This information may change from time to time and at any time without notification. The most current information will be updated periodically and posted in the online forum.  
  spamspam Loan Servicing, LLC. NMLS#391521. nmlsconsumeraccess.org. You are receiving this information as a current loan customer with spamspam Loan Servicing, LLC. Not licensed for lending activities in any of the U.S. territories. Not authorized to originate loans in the State of New York. Licensed by the Dept. of Financial Protection and Innovation under the California Residential Mortgage .Lending Act #4131216.  

  This email was sent to  somebody@gmail.com   
  Version 103023PCHPrAp9   
  ` + "\ufeff" + `  

 spamspam Loan Servicing  |    4425 Ponce de Leon Blvd 5-251 ,        Coral Gables, FL 33146-1837   

 Unsubscribe somebody@gmail.com   

 Update Profile  |
 Our Privacy Policy   | 
 Constant Contact Data Notice 

Sent by
 marklake@spamspam.com`
		require.Equal(t, expected, actual)
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_HTMLOnly_FromDiskStation(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: synology@mydomain.me
RCPT TO: synology@mydomain.me
DATA
From: "=?UTF-8?B?Um9iYmll?=" <synology@mydomain.me>
To: <synology@mydomain.me>
Message-Id: <640e6f562895d.6c9584bcfa491ac9c546b480b32ffc1d@mydomain.me>
MIME-Version: 1.0
Subject: =?UTF-8?B?W1N5bm9sb2d5IE5BU10gVGVzdCBNZXNzYWdlIGZyb20gTGl0dHNfTkFT?=
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: 8bit

Congratulations! You have successfully set up the email notification on Synology_NAS.<BR>For further system configurations, please visit http://192.168.1.28:5000/, http://172.16.60.5:5000/.<BR>(If you cannot connect to the server, please contact the administrator.)<BR><BR>From Synology_NAS<BR><BR><BR>
.
`
	s, c, conf, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/synology", r.URL.Path)
		require.Equal(t, "[Synology NAS] Test Message from Litts_NAS", r.Header.Get("Title"))
		actual := readAll(t, r.Body)
		expected := `Congratulations! You have successfully set up the email notification on Synology_NAS. For further system configurations, please visit http://192.168.1.28:5000/, http://172.16.60.5:5000/. (If you cannot connect to the server, please contact the administrator.)  From Synology_NAS`
		require.Equal(t, expected, actual)
	})
	conf.SMTPServerDomain = "mydomain.me"
	conf.SMTPServerAddrPrefix = ""
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_PlaintextWithToken(t *testing.T) {
	email := `EHLO example.com
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic+tk_KLORUqSqvNRLpY11DfkHVbHu9NGG2@ntfy.sh
DATA
Subject: Very short mail

what's up
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "Very short mail", r.Header.Get("Title"))
		require.Equal(t, "Bearer tk_KLORUqSqvNRLpY11DfkHVbHu9NGG2", r.Header.Get("Authorization"))
		require.Equal(t, "what's up", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

func TestSmtpBackend_PlaintextWithPlainAuth(t *testing.T) {
	email := `EHLO example.com
AUTH PLAIN dGVzdAB0ZXN0ADEyMzQ=
MAIL FROM: phil@example.com
RCPT TO: ntfy-mytopic@ntfy.sh
DATA
Subject: Very short mail

what's up
.
`
	s, c, _, scanner := newTestSMTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mytopic", r.URL.Path)
		require.Equal(t, "Very short mail", r.Header.Get("Title"))
		require.Equal(t, "Basic dGVzdDoxMjM0", r.Header.Get("Authorization"))
		require.Equal(t, "what's up", readAll(t, r.Body))
	})
	defer s.Close()
	defer c.Close()
	writeAndReadUntilLine(t, email, c, scanner, "250 2.0.0 OK: queued")
}

type smtpHandlerFunc func(http.ResponseWriter, *http.Request)

func newTestSMTPServer(t *testing.T, handler smtpHandlerFunc) (s *smtp.Server, c net.Conn, conf *Config, scanner *bufio.Scanner) {
	conf = newTestConfig(t)
	conf.SMTPServerListen = ":25"
	conf.SMTPServerDomain = "ntfy.sh"
	conf.SMTPServerAddrPrefix = "ntfy-"
	backend := newMailBackend(conf, handler)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s = smtp.NewServer(backend)
	s.Domain = conf.SMTPServerDomain
	s.AllowInsecureAuth = true
	go func() {
		require.Nil(t, s.Serve(l))
	}()
	c, err = net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	scanner = bufio.NewScanner(c)
	return
}

func writeAndReadUntilLine(t *testing.T, email string, conn net.Conn, scanner *bufio.Scanner, expectedLine string) {
	_, err := io.WriteString(conn, email)
	require.Nil(t, err)
	readUntilLine(t, conn, scanner, expectedLine)
}

func readUntilLine(t *testing.T, conn net.Conn, scanner *bufio.Scanner, expectedLine string) {
	cancelChan := make(chan bool)
	go func() {
		select {
		case <-cancelChan:
		case <-time.After(3 * time.Second):
			conn.Close()
			t.Error("Failed waiting for expected output")
		}
	}()
	var output string
	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) == expectedLine {
			cancelChan <- true
			return
		}
		output += text + "\n"
	}
	t.Fatalf("Expected line '%s' not found in output:\n%s", expectedLine, output)
}
