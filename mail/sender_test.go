package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSender_AuthLogin(t *testing.T) {
	smtpServer := newTestSMTPServer(t, "LOGIN")
	sender := NewSender(smtpServer.config("phil", "secret"))
	require.Nil(t, sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify"))
	require.Equal(t, "LOGIN", smtpServer.mechanism())
	require.Contains(t, smtpServer.data(), "https://ntfy.example.com/verify")
}

func TestSender_AuthLogin_WrongPassword(t *testing.T) {
	smtpServer := newTestSMTPServer(t, "LOGIN")
	sender := NewSender(smtpServer.config("phil", "wrong"))
	require.Error(t, sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify"))
	require.Empty(t, smtpServer.data())
}

func TestSender_AuthLogin_RefusesUnencryptedRemoteHost(t *testing.T) {
	// 127.0.0.2 is not "localhost" to net/smtp, so without TLS it must not send the password
	smtpServer := newTestSMTPServerOn(t, "127.0.0.2:0", "LOGIN")
	sender := NewSender(smtpServer.config("phil", "secret"))
	err := sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify")
	require.ErrorContains(t, err, "unencrypted connection")
	require.Empty(t, smtpServer.mechanism())
}

func TestSender_AuthLogin_StartTLS(t *testing.T) {
	// LOGIN is only offered after STARTTLS (like Outlook), on a non-localhost address, so the
	// password can only be sent because the connection is encrypted
	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" {
		t.Skip("trusting the test certificate via SSL_CERT_FILE only works on Linux/BSD")
	}
	smtpServer := newTestSMTPServerOn(t, "127.0.0.2:0", "LOGIN")
	smtpServer.tlsConfig = newTestTLSConfig(t, "127.0.0.2")
	sender := NewSender(smtpServer.config("phil", "secret"))
	require.Nil(t, sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify"))
	require.Equal(t, "LOGIN", smtpServer.mechanism())
	require.True(t, smtpServer.authenticatedOverTLS())
	require.Contains(t, smtpServer.data(), "https://ntfy.example.com/verify")
}

func TestSender_AuthPlainPreferredOverLogin(t *testing.T) {
	smtpServer := newTestSMTPServer(t, "LOGIN PLAIN")
	sender := NewSender(smtpServer.config("phil", "secret"))
	require.Nil(t, sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify"))
	require.Equal(t, "PLAIN", smtpServer.mechanism())
}

func TestSender_AuthNoSupportedMechanism(t *testing.T) {
	smtpServer := newTestSMTPServer(t, "CRAM-MD5")
	sender := NewSender(smtpServer.config("phil", "secret"))
	require.Error(t, sender.SendEmailVerification("someone@example.com", "https://ntfy.example.com/verify"))
	require.Empty(t, smtpServer.data())
}

// testSMTPServer is a minimal SMTP server that only supports the given AUTH mechanisms
// and accepts the user "phil" with password "secret". If tlsConfig is set, it offers
// STARTTLS and only advertises AUTH after the connection is encrypted.
type testSMTPServer struct {
	listener   net.Listener
	mechanisms string
	tlsConfig  *tls.Config
	usedMech   string
	usedTLS    bool
	received   string
	mu         sync.Mutex
}

func newTestSMTPServer(t *testing.T, mechanisms string) *testSMTPServer {
	return newTestSMTPServerOn(t, "127.0.0.1:0", mechanisms)
}

func newTestSMTPServerOn(t *testing.T, addr, mechanisms string) *testSMTPServer {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("cannot listen on %s: %s", addr, err.Error())
	}
	s := &testSMTPServer{listener: listener, mechanisms: mechanisms}
	t.Cleanup(func() {
		listener.Close()
	})
	go s.serve()
	return s
}

func (s *testSMTPServer) config(user, pass string) *Config {
	return &Config{
		BaseURL:  "https://ntfy.example.com",
		SMTPAddr: s.listener.Addr().String(),
		SMTPUser: user,
		SMTPPass: pass,
		From:     "ntfy@example.com",
	}
}

func (s *testSMTPServer) mechanism() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usedMech
}

func (s *testSMTPServer) authenticatedOverTLS() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usedTLS
}

func (s *testSMTPServer) data() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received
}

func (s *testSMTPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *testSMTPServer) handle(conn net.Conn) {
	defer func() {
		conn.Close()
	}()
	r := bufio.NewReader(conn)
	encrypted := false
	reply := func(format string, args ...any) {
		fmt.Fprintf(conn, format+"\r\n", args...)
	}
	readLine := func() string {
		line, _ := r.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	decode := func(s string) string {
		b, _ := base64.StdEncoding.DecodeString(s)
		return string(b)
	}
	authenticated := false
	reply("220 localhost ESMTP test")
	for {
		line := readLine()
		cmd := strings.ToUpper(line)
		switch {
		case line == "":
			return
		case strings.HasPrefix(cmd, "EHLO") && s.tlsConfig != nil && !encrypted:
			reply("250-localhost")
			reply("250 STARTTLS")
		case strings.HasPrefix(cmd, "EHLO"):
			reply("250-localhost")
			reply("250 AUTH %s", s.mechanisms)
		case strings.HasPrefix(cmd, "STARTTLS") && s.tlsConfig != nil && !encrypted:
			reply("220 Ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn, r, encrypted = tlsConn, bufio.NewReader(tlsConn), true
		case strings.HasPrefix(cmd, "AUTH LOGIN") && strings.Contains(s.mechanisms, "LOGIN"):
			reply("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
			user := decode(readLine())
			reply("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
			pass := decode(readLine())
			authenticated = s.authenticate("LOGIN", user, pass, encrypted, reply)
		case strings.HasPrefix(cmd, "AUTH PLAIN ") && strings.Contains(s.mechanisms, "PLAIN"):
			parts := strings.Split(decode(line[len("AUTH PLAIN "):]), "\x00")
			authenticated = len(parts) == 3 && s.authenticate("PLAIN", parts[1], parts[2], encrypted, reply)
		case strings.HasPrefix(cmd, "AUTH"):
			reply("504 Unrecognized authentication type")
		case strings.HasPrefix(cmd, "MAIL") && !authenticated:
			reply("530 Authentication required")
		case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"), strings.HasPrefix(cmd, "RSET"):
			reply("250 OK")
		case strings.HasPrefix(cmd, "DATA"):
			reply("354 Go ahead")
			var data strings.Builder
			for l := readLine(); l != "."; l = readLine() {
				data.WriteString(l + "\n")
			}
			s.mu.Lock()
			s.received = data.String()
			s.mu.Unlock()
			reply("250 OK")
		case strings.HasPrefix(cmd, "QUIT"):
			reply("221 Bye")
			return
		default:
			reply("502 Command not implemented")
		}
	}
}

func (s *testSMTPServer) authenticate(mechanism, user, pass string, encrypted bool, reply func(format string, args ...any)) bool {
	if user != "phil" || pass != "secret" {
		reply("535 Authentication failed")
		return false
	}
	s.mu.Lock()
	s.usedMech = mechanism
	s.usedTLS = encrypted
	s.mu.Unlock()
	reply("235 Authentication successful")
	return true
}

// newTestTLSConfig creates a self-signed certificate for the given IP and makes the Go client trust it
// via SSL_CERT_FILE, since net/smtp.SendMail does not accept a custom TLS config. The system cert pool
// is loaded once per process (during the first handshake), so all tests share one certificate.
func newTestTLSConfig(t *testing.T, ip string) *tls.Config {
	testTLSOnce.Do(func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.Nil(t, err)
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1),
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			IPAddresses:  []net.IP{net.ParseIP(ip)},
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		require.Nil(t, err)
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		certFile := filepath.Join(t.TempDir(), "cert.pem")
		require.Nil(t, os.WriteFile(certFile, certPEM, 0600))
		require.Nil(t, os.Setenv("SSL_CERT_FILE", certFile))
		testTLSCert = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	})
	return &tls.Config{Certificates: []tls.Certificate{testTLSCert}}
}

var (
	testTLSOnce sync.Once
	testTLSCert tls.Certificate
)
