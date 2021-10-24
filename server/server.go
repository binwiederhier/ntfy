package server

import (
	"bytes"
	_ "embed" // required for go:embed
	"encoding/json"
	"errors"
	"fmt"
	"heckel.io/ntfy/config"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Server struct {
	config *config.Config
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
	rawRegex   = regexp.MustCompile(`^/[^/]+/raw$`)

	//go:embed "index.html"
	indexSource string

	errTopicNotFound = errors.New("topic not found")
)

func New(conf *config.Config) *Server {
	return &Server{
		config: conf,
		topics: make(map[string]*topic),
	}
}

func (s *Server) Run() error {
	go s.runMonitor()
	return s.listenAndServe()
}

func (s *Server) listenAndServe() error {
	log.Printf("Listening on %s", s.config.ListenHTTP)
	http.HandleFunc("/", s.handle)
	return http.ListenAndServe(s.config.ListenHTTP, nil)
}

func (s *Server) runMonitor() {
	for {
		time.Sleep(30 * time.Second)
		s.mu.Lock()
		var subscribers, messages int
		for _, t := range s.topics {
			subs, msgs := t.Stats()
			subscribers += subs
			messages += msgs
		}
		log.Printf("Stats: %d topic(s), %d subscriber(s), %d message(s) sent", len(s.topics), subscribers, messages)
		s.mu.Unlock()
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.handleInternal(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, err.Error()+"\n")
	}
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) error {
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		return s.handleHome(w, r)
	} else if r.Method == http.MethodGet && jsonRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeJSON(w, r)
	} else if r.Method == http.MethodGet && sseRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeSSE(w, r)
	} else if r.Method == http.MethodGet && rawRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeRaw(w, r)
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
	defer s.unsubscribe(t, subscriberID)
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
	defer s.unsubscribe(t, subscriberID)
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

func (s *Server) handleSubscribeRaw(w http.ResponseWriter, r *http.Request) error {
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/raw")) // Hack
	subscriberID := t.Subscribe(func(msg *message) error {
		m := strings.ReplaceAll(msg.Message, "\n", " ") + "\n"
		if _, err := io.WriteString(w, m); err != nil {
			return err
		}
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		return nil
	})
	defer s.unsubscribe(t, subscriberID)
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
		return nil, errTopicNotFound
	}
	return c, nil
}

func (s *Server) unsubscribe(t *topic, subscriberID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if subscribers := t.Unsubscribe(subscriberID); subscribers == 0 {
		delete(s.topics, t.id)
	}
}
