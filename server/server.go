package server

import (
	"bytes"
	"context"
	"embed"
	_ "embed" // required for go:embed
	"encoding/json"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"fmt"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"
	"heckel.io/ntfy/config"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TODO add "max connections open" limit
// TODO add "max messages in a topic" limit
// TODO add "max topics" limit

// Server is the main server
type Server struct {
	config   *config.Config
	topics   map[string]*topic
	visitors map[string]*visitor
	firebase subscriber
	messages int64
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

	staticRegex = regexp.MustCompile(`^/static/.+`)

	//go:embed "index.html"
	indexSource string

	//go:embed static
	webStaticFs embed.FS

	errHTTPBadRequest      = &errHTTP{http.StatusBadRequest, http.StatusText(http.StatusBadRequest)}
	errHTTPNotFound        = &errHTTP{http.StatusNotFound, http.StatusText(http.StatusNotFound)}
	errHTTPTooManyRequests = &errHTTP{http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests)}
)

func New(conf *config.Config) (*Server, error) {
	var firebaseSubscriber subscriber
	if conf.FirebaseKeyFile != "" {
		var err error
		firebaseSubscriber, err = createFirebaseSubscriber(conf)
		if err != nil {
			return nil, err
		}
	}
	return &Server{
		config:   conf,
		firebase: firebaseSubscriber,
		topics:   make(map[string]*topic),
		visitors: make(map[string]*visitor),
	}, nil
}

func createFirebaseSubscriber(conf *config.Config) (subscriber, error) {
	fb, err := firebase.NewApp(context.Background(), nil, option.WithCredentialsFile(conf.FirebaseKeyFile))
	if err != nil {
		return nil, err
	}
	msg, err := fb.Messaging(context.Background())
	if err != nil {
		return nil, err
	}
	return func(m *message) error {
		_, err := msg.Send(context.Background(), &messaging.Message{
			Data: map[string]string{
				"id":      m.ID,
				"time":    fmt.Sprintf("%d", m.Time),
				"event": m.Event,
				"topic":   m.Topic,
				"message": m.Message,
			},
			Notification: &messaging.Notification{
				Title:    m.Topic, // FIXME convert to ntfy.sh/$topic instead
				Body:     m.Message,
				ImageURL: "",
			},
			Topic: m.Topic,
		})
		return err
	}, nil
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
	} else if r.Method == http.MethodGet && staticRegex.MatchString(r.URL.Path) {
		return s.handleStatic(w, r)
	} else if (r.Method == http.MethodPut || r.Method == http.MethodPost) && topicRegex.MatchString(r.URL.Path) {
		return s.handlePublish(w, r)
	} else if r.Method == http.MethodGet && jsonRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeJSON(w, r)
	} else if r.Method == http.MethodGet && sseRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeSSE(w, r)
	} else if r.Method == http.MethodGet && rawRegex.MatchString(r.URL.Path) {
		return s.handleSubscribeRaw(w, r)
	} else if r.Method == http.MethodOptions {
		return s.handleOptions(w, r)
	}
	return errHTTPNotFound
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) error {
	_, err := io.WriteString(w, indexSource)
	return err
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) error {
	http.FileServer(http.FS(webStaticFs)).ServeHTTP(w, r)
	return nil
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) error {
	t := s.createTopic(r.URL.Path[1:])
	reader := io.LimitReader(r.Body, messageLimit)
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if err := t.Publish(newDefaultMessage(t.id, string(b))); err != nil {
		return err
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	s.mu.Lock()
	s.messages++
	s.mu.Unlock()
	return nil
}

func (s *Server) handleSubscribeJSON(w http.ResponseWriter, r *http.Request) error {
	encoder := func(msg *message) (string, error) {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return s.handleSubscribe(w, r, "json", "application/stream+json", encoder)
}

func (s *Server) handleSubscribeSSE(w http.ResponseWriter, r *http.Request) error {
	encoder := func(msg *message) (string, error) {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return "", err
		}
		if msg.Event != messageEvent {
			return fmt.Sprintf("event: %s\ndata: %s\n", msg.Event, buf.String()), nil // Browser's .onmessage() does not fire on this!
		}
		return fmt.Sprintf("data: %s\n", buf.String()), nil
	}
	return s.handleSubscribe(w, r, "sse", "text/event-stream", encoder)
}

func (s *Server) handleSubscribeRaw(w http.ResponseWriter, r *http.Request) error {
	encoder := func(msg *message) (string, error) {
		if msg.Event == "" { // only handle default events
			return strings.ReplaceAll(msg.Message, "\n", " ") + "\n", nil
		}
		return "\n", nil // "keepalive" and "open" events just send an empty line
	}
	return s.handleSubscribe(w, r, "raw", "text/plain", encoder)
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request, format string, contentType string, encoder messageEncoder) error {
	t := s.createTopic(strings.TrimSuffix(r.URL.Path[1:], "/"+format)) // Hack
	since, err := parseSince(r)
	if err != nil {
		return err
	}
	poll := r.URL.Query().Has("poll")
	sub := func(msg *message) error {
		m, err := encoder(msg)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(m)); err != nil {
			return err
		}
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		return nil
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	w.Header().Set("Content-Type", contentType)
	if poll {
		return sendOldMessages(t, since, sub)
	}
	subscriberID := t.Subscribe(sub)
	defer t.Unsubscribe(subscriberID)
	if err := sub(newOpenMessage(t.id)); err != nil { // Send out open message
		return err
	}
	if err := sendOldMessages(t, since, sub); err != nil {
		return err
	}
	for {
		select {
		case <-t.ctx.Done():
			return nil
		case <-r.Context().Done():
			return nil
		case <-time.After(s.config.KeepaliveInterval):
			if err := sub(newKeepaliveMessage(t.id)); err != nil { // Send keepalive message
				return err
			}
		}
	}
}

func sendOldMessages(t *topic, since time.Time, sub subscriber) error {
	if since.IsZero() {
		return nil
	}
	for _, m := range t.Messages(since) {
		if err := sub(m); err != nil {
			return err
		}
	}
	return nil
}

func parseSince(r *http.Request) (time.Time, error) {
	if !r.URL.Query().Has("since") {
		return time.Time{}, nil
	}
	if since, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64); err == nil {
		return time.Unix(since, 0), nil
	}
	if d, err := time.ParseDuration(r.URL.Query().Get("since")); err == nil {
		return time.Now().Add(-1 * d), nil
	}
	return time.Time{}, errHTTPBadRequest
}

func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST")
	return nil
}

func (s *Server) createTopic(id string) *topic {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.topics[id]; !ok {
		s.topics[id] = newTopic(id)
		if s.firebase != nil {
			s.topics[id].Subscribe(s.firebase)
		}
	}
	return s.topics[id]
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

	// Prune old messages, remove topics without subscribers
	for _, t := range s.topics {
		t.Prune(s.config.MessageBufferDuration)
		subs, msgs := t.Stats()
		if msgs == 0 && (subs == 0 || (s.firebase != nil && subs == 1)) {
			delete(s.topics, t.id)
		}
	}

	// Print stats
	var subscribers, messages int
	for _, t := range s.topics {
		subs, msgs := t.Stats()
		subscribers += subs
		messages += msgs
	}
	log.Printf("Stats: %d message(s) published, %d topic(s) active, %d subscriber(s), %d message(s) buffered, %d visitor(s)",
		s.messages, len(s.topics), subscribers, messages, len(s.visitors))
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
