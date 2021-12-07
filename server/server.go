package server

import (
	"bytes"
	"context"
	"embed" // required for go:embed
	"encoding/json"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"fmt"
	"google.golang.org/api/option"
	"heckel.io/ntfy/config"
	"heckel.io/ntfy/util"
	"html/template"
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

// TODO add "max messages in a topic" limit
// TODO implement "since=<ID>"

// Server is the main server, providing the UI and API for ntfy
type Server struct {
	config   *config.Config
	topics   map[string]*topic
	visitors map[string]*visitor
	firebase subscriber
	messages int64
	cache    cache
	mu       sync.Mutex
}

// errHTTP is a generic HTTP error for any non-200 HTTP error
type errHTTP struct {
	Code   int
	Status string
}

func (e errHTTP) Error() string {
	return fmt.Sprintf("http: %s", e.Status)
}

type indexPage struct {
	Topic         string
	CacheDuration string
}

type sinceTime time.Time

func (t sinceTime) IsAll() bool {
	return t == sinceAllMessages
}

func (t sinceTime) IsNone() bool {
	return t == sinceNoMessages
}

func (t sinceTime) Time() time.Time {
	return time.Time(t)
}

var (
	sinceAllMessages = sinceTime(time.Unix(0, 0))
	sinceNoMessages  = sinceTime(time.Unix(1, 0))
)

const (
	messageLimit = 512
)

var (
	topicRegex = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}$`) // Regex must match JS & Android app!
	jsonRegex  = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/json$`)
	sseRegex   = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/sse$`)
	rawRegex   = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/raw$`)

	staticRegex = regexp.MustCompile(`^/static/.+`)
	docsRegex   = regexp.MustCompile(`^/docs(|/.*)$`)

	//go:embed "index.gohtml"
	indexSource   string
	indexTemplate = template.Must(template.New("index").Parse(indexSource))

	//go:embed "example.html"
	exampleSource string

	//go:embed static
	webStaticFs       embed.FS
	webStaticFsCached = &util.CachingEmbedFS{ModTime: time.Now(), FS: webStaticFs}

	//go:embed docs
	docsStaticFs     embed.FS
	docsStaticCached = &util.CachingEmbedFS{ModTime: time.Now(), FS: docsStaticFs}

	errHTTPBadRequest      = &errHTTP{http.StatusBadRequest, http.StatusText(http.StatusBadRequest)}
	errHTTPNotFound        = &errHTTP{http.StatusNotFound, http.StatusText(http.StatusNotFound)}
	errHTTPTooManyRequests = &errHTTP{http.StatusTooManyRequests, http.StatusText(http.StatusTooManyRequests)}
)

// New instantiates a new Server. It creates the cache and adds a Firebase
// subscriber (if configured).
func New(conf *config.Config) (*Server, error) {
	var firebaseSubscriber subscriber
	if conf.FirebaseKeyFile != "" {
		var err error
		firebaseSubscriber, err = createFirebaseSubscriber(conf)
		if err != nil {
			return nil, err
		}
	}
	cache, err := createCache(conf)
	if err != nil {
		return nil, err
	}
	topics, err := cache.Topics()
	if err != nil {
		return nil, err
	}
	for _, t := range topics {
		if firebaseSubscriber != nil {
			t.Subscribe(firebaseSubscriber)
		}
	}
	return &Server{
		config:   conf,
		cache:    cache,
		firebase: firebaseSubscriber,
		topics:   topics,
		visitors: make(map[string]*visitor),
	}, nil
}

func createCache(conf *config.Config) (cache, error) {
	if conf.CacheFile != "" {
		return newSqliteCache(conf.CacheFile)
	}
	return newMemCache(), nil
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
			Topic: m.Topic,
			Data: map[string]string{
				"id":       m.ID,
				"time":     fmt.Sprintf("%d", m.Time),
				"event":    m.Event,
				"topic":    m.Topic,
				"priority": fmt.Sprintf("%d", m.Priority),
				"tags":     strings.Join(m.Tags, ","),
				"title":    m.Title,
				"message":  m.Message,
			},
		})
		return err
	}, nil
}

// Run executes the main server. It listens on HTTP (+ HTTPS, if configured), and starts
// a manager go routine to print stats and prune messages.
func (s *Server) Run() error {
	go func() {
		ticker := time.NewTicker(s.config.ManagerInterval)
		for {
			<-ticker.C
			s.updateStatsAndExpire()
		}
	}()
	listenStr := fmt.Sprintf("%s/http", s.config.ListenHTTP)
	if s.config.ListenHTTPS != "" {
		listenStr += fmt.Sprintf(" %s/https", s.config.ListenHTTPS)
	}
	log.Printf("Listening on %s", listenStr)

	http.HandleFunc("/", s.handle)
	errChan := make(chan error)
	go func() {
		errChan <- http.ListenAndServe(s.config.ListenHTTP, nil)
	}()
	if s.config.ListenHTTPS != "" {
		go func() {
			errChan <- http.ListenAndServeTLS(s.config.ListenHTTPS, s.config.CertFile, s.config.KeyFile, nil)
		}()
	}
	return <-errChan
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
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		return s.handleHome(w, r)
	} else if r.Method == http.MethodGet && r.URL.Path == "/example.html" {
		return s.handleExample(w, r)
	} else if r.Method == http.MethodHead && r.URL.Path == "/" {
		return s.handleEmpty(w, r)
	} else if r.Method == http.MethodGet && staticRegex.MatchString(r.URL.Path) {
		return s.handleStatic(w, r)
	} else if r.Method == http.MethodGet && docsRegex.MatchString(r.URL.Path) {
		return s.handleDocs(w, r)
	} else if r.Method == http.MethodOptions {
		return s.handleOptions(w, r)
	} else if r.Method == http.MethodGet && topicRegex.MatchString(r.URL.Path) {
		return s.handleHome(w, r)
	} else if (r.Method == http.MethodPut || r.Method == http.MethodPost) && topicRegex.MatchString(r.URL.Path) {
		return s.withRateLimit(w, r, s.handlePublish)
	} else if r.Method == http.MethodGet && jsonRegex.MatchString(r.URL.Path) {
		return s.withRateLimit(w, r, s.handleSubscribeJSON)
	} else if r.Method == http.MethodGet && sseRegex.MatchString(r.URL.Path) {
		return s.withRateLimit(w, r, s.handleSubscribeSSE)
	} else if r.Method == http.MethodGet && rawRegex.MatchString(r.URL.Path) {
		return s.withRateLimit(w, r, s.handleSubscribeRaw)
	}
	return errHTTPNotFound
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) error {
	return indexTemplate.Execute(w, &indexPage{
		Topic:         r.URL.Path[1:],
		CacheDuration: util.DurationToHuman(s.config.CacheDuration),
	})
}

func (s *Server) handleEmpty(_ http.ResponseWriter, _ *http.Request) error {
	return nil
}

func (s *Server) handleExample(w http.ResponseWriter, _ *http.Request) error {
	_, err := io.WriteString(w, exampleSource)
	return err
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) error {
	http.FileServer(http.FS(webStaticFsCached)).ServeHTTP(w, r)
	return nil
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) error {
	http.FileServer(http.FS(docsStaticCached)).ServeHTTP(w, r)
	return nil
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	t, err := s.topicFromID(r.URL.Path[1:])
	if err != nil {
		return err
	}
	reader := io.LimitReader(r.Body, messageLimit)
	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m := newDefaultMessage(t.id, string(b))
	if m.Message == "" {
		return errHTTPBadRequest
	}
	title, priority, tags := parseHeaders(r.Header)
	m.Title = title
	m.Priority = priority
	m.Tags = tags
	if err := t.Publish(m); err != nil {
		return err
	}
	if err := s.cache.AddMessage(m); err != nil {
		return err
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return err
	}
	s.mu.Lock()
	s.messages++
	s.mu.Unlock()
	return nil
}

func parseHeaders(header http.Header) (title string, priority int, tags []string) {
	title = readHeader(header, "x-title", "title", "ti", "t")
	priorityStr := readHeader(header, "x-priority", "priority", "prio", "p")
	if priorityStr != "" {
		switch strings.ToLower(priorityStr) {
		case "1", "min":
			priority = 1
		case "2", "low":
			priority = 2
		case "4", "high":
			priority = 4
		case "5", "max", "urgent":
			priority = 5
		default:
			priority = 3
		}
	}
	tagsStr := readHeader(header, "x-tags", "tags", "ta")
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}
	return title, priority, tags
}

func readHeader(header http.Header, names ...string) string {
	for _, name := range names {
		value := header.Get(name)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) handleSubscribeJSON(w http.ResponseWriter, r *http.Request, v *visitor) error {
	encoder := func(msg *message) (string, error) {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return s.handleSubscribe(w, r, v, "json", "application/x-ndjson", encoder)
}

func (s *Server) handleSubscribeSSE(w http.ResponseWriter, r *http.Request, v *visitor) error {
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
	return s.handleSubscribe(w, r, v, "sse", "text/event-stream", encoder)
}

func (s *Server) handleSubscribeRaw(w http.ResponseWriter, r *http.Request, v *visitor) error {
	encoder := func(msg *message) (string, error) {
		if msg.Event == messageEvent { // only handle default events
			return strings.ReplaceAll(msg.Message, "\n", " ") + "\n", nil
		}
		return "\n", nil // "keepalive" and "open" events just send an empty line
	}
	return s.handleSubscribe(w, r, v, "raw", "text/plain", encoder)
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request, v *visitor, format string, contentType string, encoder messageEncoder) error {
	if err := v.AddSubscription(); err != nil {
		return errHTTPTooManyRequests
	}
	defer v.RemoveSubscription()
	topicsStr := strings.TrimSuffix(r.URL.Path[1:], "/"+format) // Hack
	topicIDs := strings.Split(topicsStr, ",")
	topics, err := s.topicsFromIDs(topicIDs...)
	if err != nil {
		return err
	}
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
	w.Header().Set("Access-Control-Allow-Origin", "*")            // CORS, allow cross-origin requests
	w.Header().Set("Content-Type", contentType+"; charset=utf-8") // Android/Volley client needs charset!
	if poll {
		return s.sendOldMessages(topics, since, sub)
	}
	subscriberIDs := make([]int, 0)
	for _, t := range topics {
		subscriberIDs = append(subscriberIDs, t.Subscribe(sub))
	}
	defer func() {
		for i, subscriberID := range subscriberIDs {
			topics[i].Unsubscribe(subscriberID) // Order!
		}
	}()
	if err := sub(newOpenMessage(topicsStr)); err != nil { // Send out open message
		return err
	}
	if err := s.sendOldMessages(topics, since, sub); err != nil {
		return err
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-time.After(s.config.KeepaliveInterval):
			v.Keepalive()
			if err := sub(newKeepaliveMessage(topicsStr)); err != nil { // Send keepalive message
				return err
			}
		}
	}
}

func (s *Server) sendOldMessages(topics []*topic, since sinceTime, sub subscriber) error {
	if since.IsNone() {
		return nil
	}
	for _, t := range topics {
		messages, err := s.cache.Messages(t.id, since)
		if err != nil {
			return err
		}
		for _, m := range messages {
			if err := sub(m); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseSince returns a timestamp identifying the time span from which cached messages should be received.
//
// Values in the "since=..." parameter can be either a unix timestamp or a duration (e.g. 12h), or
// "all" for all messages.
func parseSince(r *http.Request) (sinceTime, error) {
	if !r.URL.Query().Has("since") {
		if r.URL.Query().Has("poll") {
			return sinceAllMessages, nil
		}
		return sinceNoMessages, nil
	}
	if r.URL.Query().Get("since") == "all" {
		return sinceAllMessages, nil
	}
	if s, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64); err == nil {
		return sinceTime(time.Unix(s, 0)), nil
	}
	if d, err := time.ParseDuration(r.URL.Query().Get("since")); err == nil {
		return sinceTime(time.Now().Add(-1 * d)), nil
	}
	return sinceNoMessages, errHTTPBadRequest
}

func (s *Server) handleOptions(w http.ResponseWriter, _ *http.Request) error {
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST")
	return nil
}

func (s *Server) topicFromID(id string) (*topic, error) {
	topics, err := s.topicsFromIDs(id)
	if err != nil {
		return nil, err
	}
	return topics[0], nil
}

func (s *Server) topicsFromIDs(ids ...string) ([]*topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make([]*topic, 0)
	for _, id := range ids {
		if _, ok := s.topics[id]; !ok {
			if len(s.topics) >= s.config.GlobalTopicLimit {
				return nil, errHTTPTooManyRequests
			}
			s.topics[id] = newTopic(id, time.Now())
			if s.firebase != nil {
				s.topics[id].Subscribe(s.firebase)
			}
		}
		topics = append(topics, s.topics[id])
	}
	return topics, nil
}

func (s *Server) updateStatsAndExpire() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Expire visitors from rate visitors map
	for ip, v := range s.visitors {
		if v.Stale() {
			delete(s.visitors, ip)
		}
	}

	// Prune cache
	if err := s.cache.Prune(s.config.CacheDuration); err != nil {
		log.Printf("error pruning cache: %s", err.Error())
	}

	// Prune old messages, remove subscriptions without subscribers
	var subscribers, messages int
	for _, t := range s.topics {
		subs := t.Subscribers()
		msgs, err := s.cache.MessageCount(t.id)
		if err != nil {
			log.Printf("cannot get stats for topic %s: %s", t.id, err.Error())
			continue
		}
		if msgs == 0 && (subs == 0 || (s.firebase != nil && subs == 1)) { // Firebase is a subscriber!
			delete(s.topics, t.id)
			continue
		}
		subscribers += subs
		messages += msgs
	}

	// Print stats
	log.Printf("Stats: %d message(s) published, %d topic(s) active, %d subscriber(s), %d message(s) buffered, %d visitor(s)",
		s.messages, len(s.topics), subscribers, messages, len(s.visitors))
}

func (s *Server) withRateLimit(w http.ResponseWriter, r *http.Request, handler func(w http.ResponseWriter, r *http.Request, v *visitor) error) error {
	v := s.visitor(r)
	if err := v.RequestAllowed(); err != nil {
		return err
	}
	return handler(w, r, v)
}

// visitor creates or retrieves a rate.Limiter for the given visitor.
// This function was taken from https://www.alexedwards.net/blog/how-to-rate-limit-http-requests (MIT).
func (s *Server) visitor(r *http.Request) *visitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	remoteAddr := r.RemoteAddr
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr // This should not happen in real life; only in tests.
	}
	if s.config.BehindProxy && r.Header.Get("X-Forwarded-For") != "" {
		ip = r.Header.Get("X-Forwarded-For")
	}
	v, exists := s.visitors[ip]
	if !exists {
		s.visitors[ip] = newVisitor(s.config)
		return s.visitors[ip]
	}
	v.seen = time.Now()
	return v
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, code int, err error) {
	log.Printf("[%s] %s - %d - %s", r.RemoteAddr, r.Method, code, err.Error())
	w.WriteHeader(code)
	_, _ = io.WriteString(w, fmt.Sprintf("%s\n", http.StatusText(code)))
}
