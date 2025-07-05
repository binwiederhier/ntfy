package server

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"heckel.io/ntfy/v2/user"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestExtractIPAddress(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "  1.2.3.4  , 5.6.7.8")
	r.Header.Set("X-Client-IP", "9.10.11.12")
	r.Header.Set("X-Real-IP", "13.14.15.16, 1.1.1.1")
	r.Header.Set("Forwarded", "for=17.18.19.20;by=proxy.example.com, by=2.2.2.2;for=1.1.1.1")

	trustedProxies := []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")}

	require.Equal(t, "5.6.7.8", extractIPAddress(r, true, "X-Forwarded-For", trustedProxies).String())
	require.Equal(t, "9.10.11.12", extractIPAddress(r, true, "X-Client-IP", trustedProxies).String())
	require.Equal(t, "13.14.15.16", extractIPAddress(r, true, "X-Real-IP", trustedProxies).String())
	require.Equal(t, "17.18.19.20", extractIPAddress(r, true, "Forwarded", trustedProxies).String())
	require.Equal(t, "10.0.0.1", extractIPAddress(r, false, "X-Forwarded-For", trustedProxies).String())
}

func TestExtractIPAddress_UnixSocket(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", nil)
	r.RemoteAddr = "@"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8, 1.1.1.1")
	r.Header.Set("Forwarded", "by=bla.example.com;for=17.18.19.20")

	trustedProxies := []netip.Prefix{netip.MustParsePrefix("1.1.1.1/32")}

	require.Equal(t, "5.6.7.8", extractIPAddress(r, true, "X-Forwarded-For", trustedProxies).String())
	require.Equal(t, "17.18.19.20", extractIPAddress(r, true, "Forwarded", trustedProxies).String())
	require.Equal(t, "0.0.0.0", extractIPAddress(r, false, "X-Forwarded-For", trustedProxies).String())
}

func TestExtractIPAddress_MixedIPv4IPv6(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", nil)
	r.RemoteAddr = "[2001:db8:abcd::1]:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 2001:db8:abcd::2, 5.6.7.8")
	trustedProxies := []netip.Prefix{netip.MustParsePrefix("1.2.3.0/24")}
	require.Equal(t, "5.6.7.8", extractIPAddress(r, true, "X-Forwarded-For", trustedProxies).String())
}

func TestExtractIPAddress_TrustedIPv6Prefix(t *testing.T) {
	r, _ := http.NewRequest("GET", "http://ntfy.sh/mytopic/json?since=all", nil)
	r.RemoteAddr = "[2001:db8:abcd::1]:1234"
	r.Header.Set("X-Forwarded-For", "2001:db8:aaaa::1, 2001:db8:aaaa::2, 2001:db8:abcd:2::3")
	trustedProxies := []netip.Prefix{netip.MustParsePrefix("2001:db8:aaaa::/48")}
	require.Equal(t, "2001:db8:abcd:2::3", extractIPAddress(r, true, "X-Forwarded-For", trustedProxies).String())
}

func TestVisitorID(t *testing.T) {
	confWithDefaults := &Config{
		VisitorPrefixBitsIPv4: 32,
		VisitorPrefixBitsIPv6: 64,
	}
	confWithShortenedPrefixes := &Config{
		VisitorPrefixBitsIPv4: 16,
		VisitorPrefixBitsIPv6: 56,
	}
	userWithTier := &user.User{
		ID:   "u_123",
		Tier: &user.Tier{},
	}
	require.Equal(t, "ip:1.2.3.4", visitorID(netip.MustParseAddr("1.2.3.4"), nil, confWithDefaults))
	require.Equal(t, "ip:2a01:599:b26:2397::", visitorID(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), nil, confWithDefaults))
	require.Equal(t, "user:u_123", visitorID(netip.MustParseAddr("1.2.3.4"), userWithTier, confWithDefaults))
	require.Equal(t, "user:u_123", visitorID(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), userWithTier, confWithDefaults))
	require.Equal(t, "ip:1.2.0.0", visitorID(netip.MustParseAddr("1.2.3.4"), nil, confWithShortenedPrefixes))
	require.Equal(t, "ip:2a01:599:b26:2300::", visitorID(netip.MustParseAddr("2a01:599:b26:2397:dbe7:5aa2:95ce:1e83"), nil, confWithShortenedPrefixes))
}
