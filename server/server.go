package server

import (
	"bytes"
	_ "embed" // required for go:embed
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
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
	mu sync.Mutex
}

type message struct {
	Time int64 `json:"time"`
	Message string `json:"message"`
}

const (
	messageLimit = 1024
)

var (
	topicRegex    = regexp.MustCompile(`^/[^/]+$`)
	wsRegex    = regexp.MustCompile(`^/[^/]+/ws$`)
	jsonRegex    = regexp.MustCompile(`^/[^/]+/json$`)
	wsUpgrader = websocket.Upgrader{
		ReadBufferSize:  messageLimit,
		WriteBufferSize: messageLimit,
	}

	//go:embed "index.html"
	indexSource string
)

func New() *Server {
	return &Server{
		topics: make(map[string]*topic),
	}
}

func (s *Server) Run() error {
	go func() {
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
	}()
	log.Printf("Listening on :9997")
	http.HandleFunc("/", s.handle)
	return http.ListenAndServe(":9997", nil)
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
	} else if r.Method == http.MethodGet && wsRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeWS(w, r)
	} else if r.Method == http.MethodGet && jsonRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeHTTP(w, r)
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

func (s *Server) handleSubscribeHTTP(w http.ResponseWriter, r *http.Request) error {
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/json")) // Hack
	subscriberID := t.Subscribe(func (msg *message) error {
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

func (s *Server) handleSubscribeWS(w http.ResponseWriter, r *http.Request) error {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/ws")) // Hack
	t.Subscribe(func (msg *message) error {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return err
		}
		defer conn.Close()
		/*conn.SetWriteDeadline(time.Now().Add(writeWait))
		if !ok {
			// The hub closed the channel.
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}*/

		w, err := conn.NextWriter(websocket.TextMessage)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg.Message)); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		return nil
	})
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
