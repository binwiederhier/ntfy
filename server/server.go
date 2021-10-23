package server

import (
	"bytes"
	_ "embed" // required for go:embed
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Server struct {
	topics map[string]*topic
	mu     sync.Mutex
}

type message struct {
	Time    int64  `json:"time"`
	Message string `json:"message"`
}

const (
	messageLimit = 1024
)

var (
	topicRegex = regexp.MustCompile(`^/[^/]+$`)
	jsonRegex  = regexp.MustCompile(`^/[^/]+/json$`)
	sseRegex   = regexp.MustCompile(`^/[^/]+/sse$`)

	//go:embed "index.html"
	indexSource string
)

func New() *Server {
	return &Server{
		topics: make(map[string]*topic),
	}
}

func (s *Server) Run() error {
	go s.runMonitor()
	return s.listenAndServe()
}

func (s *Server) listenAndServe() error {
	log.Printf("Listening on :9997")
	http.HandleFunc("/", s.handle)
	return http.ListenAndServe(":9997", nil)
}

func (s *Server) runMonitor() {
	for {
		time.Sleep(5 * time.Second)
		s.mu.Lock()
		log.Printf("topics: %d", len(s.topics))
		for _, t := range s.topics {
			t.mu.Lock()
			log.Printf("- %s: %d subscriber(s), %d message(s) sent, last active = %s",
				t.id, len(t.subscribers), t.messages, t.last.String())
			t.mu.Unlock()
		}
		// TODO kill dead topics
		s.mu.Unlock()
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.handleInternal(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, err.Error())
	}
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		return s.handleHome(w, r)
	} else if r.Method == http.MethodGet && jsonRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeJSON(w, r)
	} else if r.Method == http.MethodGet && sseRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeSSE(w, r)
	} else if (r.Method == http.MethodPut || r.Method == http.MethodPost) && topicRegex.MatchString(r.URL.Path) {
		return s.handlePublishHTTP(w, r)
	}
	http.NotFound(w, r)
	return nil
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) error {
	_, err := io.WriteString(w, indexSource)
	return err
}

func (s *Server) handlePublishHTTP(w http.ResponseWriter, r *http.Request) error {
	t, err := s.topic(r.URL.Path[1:])
	if err != nil {
		return err
	}
	reader := io.LimitReader(r.Body, messageLimit)
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	msg := &message{
		Time:    time.Now().UnixMilli(),
		Message: string(b),
	}
	return t.Publish(msg)
}

func (s *Server) handleSubscribeJSON(w http.ResponseWriter, r *http.Request) error {
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/json")) // Hack
	subscriberID := t.Subscribe(func(msg *message) error {
		if err := json.NewEncoder(w).Encode(&msg); err != nil {
			return err
		}
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		return nil
	})
	defer t.Unsubscribe(subscriberID)
	select {
	case <-t.ctx.Done():
	case <-r.Context().Done():
	}
	return nil
}

func (s *Server) handleSubscribeSSE(w http.ResponseWriter, r *http.Request) error {
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/sse")) // Hack
	subscriberID := t.Subscribe(func(msg *message) error {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return err
		}
		m := fmt.Sprintf("data: %s\n", buf.String())
		if _, err := io.WriteString(w, m); err != nil {
			return err
		}
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		return nil
	})
	defer t.Unsubscribe(subscriberID)
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, "event: open\n\n"); err != nil {
		return err
	}
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	select {
	case <-t.ctx.Done():
	case <-r.Context().Done():
	}
	return nil
}

func (s *Server) createTopic(id string) *topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.topics[id]; !ok {
		s.topics[id] = newTopic(id)
	}
	return s.topics[id]
}

func (s *Server) topic(topicID string) (*topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.topics[topicID]
	if !ok {
		return nil, errors.New("topic does not exist")
	}
	return c, nil
}
