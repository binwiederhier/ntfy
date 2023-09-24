package server

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestReadBoolParam(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://ntfy.sh/mytopic?up=1&firebase=no", nil)
	up := readBoolParam(r, false, "x-up", "up")
	firebase := readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	r.Header.Set("X-Up", "yes")
	r.Header.Set("X-Firebase", "0")
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-firebase", "firebase")
	require.Equal(t, true, up)
	require.Equal(t, false, firebase)

	r, _ = http.NewRequest("GET", "https://ntfy.sh/mytopic", nil)
	up = readBoolParam(r, false, "x-up", "up")
	firebase = readBoolParam(r, true, "x-up", "up")
	require.Equal(t, false, up)
	require.Equal(t, true, firebase)
}

func TestRenderHTTPRequest_ValidShort(t *testing.T) {
	r, _ := http.NewRequest("POST", "http://ntfy.sh/mytopic?p=2", strings.NewReader("some message"))
	r.Header.Set("Title", "A title")
	expected := `POST /mytopic?p=2 HTTP/1.1
Title: A title

some message`
	require.Equal(t, expected, renderHTTPRequest(r))
}

func TestRenderHTTPRequest_ValidLong(t *testing.T) {
	body := strings.Repeat("a", 5000)
	r, _ := http.NewRequest("POST", "http://ntfy.sh/mytopic?p=2", strings.NewReader(body))
	r.Header.Set("Accept", "*/*")
	expected := `POST /mytopic?p=2 HTTP/1.1
Accept: */*

` + strings.Repeat("a", 4096) + " ... (peeked 4096 bytes)"
	require.Equal(t, expected, renderHTTPRequest(r))
}

func TestRenderHTTPRequest_InvalidShort(t *testing.T) {
	body := []byte{0xc3, 0x28}
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", bytes.NewReader(body))
	r.Header.Set("Accept", "*/*")
	expected := `GET /mytopic/json?since=all HTTP/1.1
Accept: */*

(peeked bytes not UTF-8, 2 bytes, hex: c328)`
	require.Equal(t, expected, renderHTTPRequest(r))
}

func TestRenderHTTPRequest_InvalidLong(t *testing.T) {
	body := make([]byte, 5000)
	rand.Read(body)
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", bytes.NewReader(body))
	r.Header.Set("Accept", "*/*")
	expected := `GET /mytopic/json?since=all HTTP/1.1
Accept: */*

(peeked bytes not UTF-8, peek limit of 4096 bytes reached, hex: ` + fmt.Sprintf("%x", body[:4096]) + ` ...)`
	require.Equal(t, expected, renderHTTPRequest(r))
}

func TestMaybeIgnoreSpecialHeader(t *testing.T) {
	require.Empty(t, maybeIgnoreSpecialHeader("priority", "u=1"))
	require.Empty(t, maybeIgnoreSpecialHeader("Priority", "u=1"))
	require.Empty(t, maybeIgnoreSpecialHeader("Priority", "u=1, i"))
}

func TestMaybeDecodeHeaders(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", nil)
	r.Header.Set("Priority", "u=1") // Cloudflare priority header
	r.Header.Set("X-Priority", "5") // ntfy priority header
	require.Equal(t, "5", readHeaderParam(r, "x-priority", "priority", "p"))
}
