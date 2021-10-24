package server

import (
	"bytes"
	_ "embed" // required for go:embed
	"encoding/json"
	"fmt"
	"golang.org/x/time/rate"
	"heckel.io/ntfy/config"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Server is the main server
type Server struct {
	config   *config.Config
	topics   map[string]*topic
	visitors map[string]*visitor
	mu       sync.Mutex
}

// visitor represents an API user, and its associated rate.Limiter used for rate limiting
type visitor struct {
	limiter *rate.Limiter
	seen    time.Time
}

// errHTTP is a generic HTTP error for any non-200 HTTP error
type errHTTP struct {
	Code   int
	Status string
}

func (e errHTTP) Error() string {
	return fmt.Sprintf("http: %s", e.Status)
}

const (
	messageLimit        = 1024
	visitorExpungeAfter = 30 * time.Minute
)

var (
	topicRegex = regexp.MustCompile(`^/[^/]+$`)
	jsonRegex  = regexp.MustCompile(`^/[^/]+/json$`)
	sseRegex   = regexp.MustCompile(`^/[^/]+/sse$`)
	rawRegex   = regexp.MustCompile(`^/[^/]+/raw$`)

	//go:embed "index.html"
	indexSource string

	errHTTPNotFound        = &errHTTP{http.StatusNotFound, http.StatusText(http.StatusNotFound)}
	errHTTPTooManyRequests = &errHTTP{http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests)}
)

func New(conf *config.Config) *Server {
	return &Server{
		config:   conf,
		topics:   make(map[string]*topic),
		visitors: make(map[string]*visitor),
	}
}

func (s *Server) Run() error {
	go func() {
		ticker := time.NewTicker(s.config.ManagerInterval)
		for {
			<-ticker.C
			s.updateStatsAndExpire()
		}
	}()
	return s.listenAndServe()
}

func (s *Server) listenAndServe() error {
	log.Printf("Listening on %s", s.config.ListenHTTP)
	http.HandleFunc("/", s.handle)
	return http.ListenAndServe(s.config.ListenHTTP, nil)
}

func (s *Server) updateStatsAndExpire() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Expire visitors from rate visitors map
	for ip, v := range s.visitors {
		if time.Since(v.seen) > visitorExpungeAfter {
			delete(s.visitors, ip)
		}
	}

	// Print stats
	var subscribers, messages int
	for _, t := range s.topics {
		subs, msgs := t.Stats()
		subscribers += subs
		messages += msgs
	}
	log.Printf("Stats: %d topic(s), %d subscriber(s), %d message(s) sent, %d visitor(s)",
		len(s.topics), subscribers, messages, len(s.visitors))
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if err := s.handleInternal(w, r); err != nil {
		if e, ok := err.(*errHTTP); ok {
			s.fail(w, r, e.Code, e)
		} else {
			s.fail(w, r, http.StatusInternalServerError, err)
		}
	}
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request) error {
	v := s.visitor(r.RemoteAddr)
	if !v.limiter.Allow() {
		return errHTTPTooManyRequests
	}
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
	return errHTTPNotFound
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
		return nil, errHTTPNotFound
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

// visitor creates or retrieves a rate.Limiter for the given visitor.
// This function was taken from https://www.alexedwards.net/blog/how-to-rate-limit-http-requests (MIT).
func (s *Server) visitor(remoteAddr string) *visitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr // This should not happen in real life; only in tests.
	}
	v, exists := s.visitors[ip]
	if !exists {
		v = &visitor{
			rate.NewLimiter(s.config.Limit, s.config.LimitBurst),
			time.Now(),
		}
		s.visitors[ip] = v
		return v
	}
	v.seen = time.Now()
	return v
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, code int, err error) {
	log.Printf("[%s] %s - %d - %s", r.RemoteAddr, r.Method, code, err.Error())
	w.WriteHeader(code)
	io.WriteString(w, fmt.Sprintf("%s\n", http.StatusText(code)))
}
