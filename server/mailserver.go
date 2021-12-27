package server

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/emersion/go-smtp"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"sync"
)

// mailBackend implements SMTP server methods.
type mailBackend struct {
	s *Server
}

func (b *mailBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return &Session{s: b.s}, nil
}

func (b *mailBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &Session{s: b.s}, nil
}

// Session is returned after EHLO.
type Session struct {
	s        *Server
	from, to string
	mu       sync.Mutex
}

func (s *Session) AuthPlain(username, password string) error {
	return nil
}

func (s *Session) Mail(from string, opts smtp.MailOptions) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.from = from
	log.Println("Mail from:", from)
	return nil
}

func (s *Session) Rcpt(to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.to = to
	log.Println("Rcpt to:", to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	log.Println("Data:", string(b))
	msg, err := mail.ReadMessage(bytes.NewReader(b))
	if err != nil {
		return err
	}
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return err
	}
	topic := strings.TrimSuffix(s.to, "@ntfy.sh")
	url := fmt.Sprintf("%s/%s", s.s.config.BaseURL, topic)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	subject := msg.Header.Get("Subject")
	if subject != "" {
		req.Header.Set("Title", subject)
	}
	rr := httptest.NewRecorder()
	s.s.handle(rr, req)
	if rr.Code != http.StatusOK {
		return errors.New("error: " + rr.Body.String())
	}
	return nil
}

func (s *Session) Reset() {
	s.mu.Lock()
	s.from = ""
	s.to = ""
	s.mu.Unlock()
}

func (s *Session) Logout() error {
	return nil
}
