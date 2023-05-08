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
		require.Equal(t,  `what's
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
		//fmt.Println(text)
	}
	t.Fatalf("Expected line '%s' not found in output:\n%s", expectedLine, output)
}
