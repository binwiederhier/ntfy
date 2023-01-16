package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"heckel.io/ntfy/user"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"heckel.io/ntfy/log"

	"github.com/emersion/go-smtp"
	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
	"heckel.io/ntfy/util"
)

/*
	TODO
		payments:
		- send dunning emails when overdue
		- payment methods
		- unmarshal to stripe.Subscription instead of gjson
		- Make ResetTier reset the stripe fields
		- delete subscription when account deleted
		- remove tier.paid
		- add tier.visible

		Limits & rate limiting:
			users without tier: should the stats be persisted? are they meaningful?
				-> test that the visitor is based on the IP address!
			login/account endpoints
		update last_seen when API is accessed
		Make sure account endpoints make sense for admins

		triggerChange after publishing a message
		UI:
		- flicker of upgrade banner
		- JS constants
		Sync:
			- "mute" setting
			- figure out what settings are "web" or "phone"
			- sync problems with "deleteAfter=0" and "displayName="
		Delete visitor when tier is changed to refresh rate limiters
		Tests:
		- Change tier from higher to lower tier (delete reservations)
		- Message rate limiting and reset tests
		- test that the visitor is based on the IP address when a user has no tier
*/

// Server is the main server, providing the UI and API for ntfy
type Server struct {
	config            *Config
	httpServer        *http.Server
	httpsServer       *http.Server
	unixListener      net.Listener
	smtpServer        *smtp.Server
	smtpServerBackend *smtpBackend
	smtpSender        mailer
	topics            map[string]*topic
	visitors          map[string]*visitor // ip:<ip> or user:<user>
	firebaseClient    *firebaseClient
	messages          int64
	userManager       *user.Manager // Might be nil!
	messageCache      *messageCache
	fileCache         *fileCache
	closeChan         chan bool
	mu                sync.Mutex
}

// handleFunc extends the normal http.HandlerFunc to be able to easily return errors
type handleFunc func(http.ResponseWriter, *http.Request, *visitor) error

var (
	// If changed, don't forget to update Android App and auth_sqlite.go
	topicRegex             = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)               // No /!
	topicPathRegex         = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}$`)              // Regex must match JS & Android app!
	externalTopicPathRegex = regexp.MustCompile(`^/[^/]+\.[^/]+/[-_A-Za-z0-9]{1,64}$`) // Extended topic path, for web-app, e.g. /example.com/mytopic
	jsonPathRegex          = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/json$`)
	ssePathRegex           = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/sse$`)
	rawPathRegex           = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/raw$`)
	wsPathRegex            = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/ws$`)
	authPathRegex          = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}(,[-_A-Za-z0-9]{1,64})*/auth$`)
	publishPathRegex       = regexp.MustCompile(`^/[-_A-Za-z0-9]{1,64}/(publish|send|trigger)$`)

	webConfigPath                                     = "/config.js"
	healthPath                                        = "/v1/health"
	accountPath                                       = "/v1/account"
	accountTokenPath                                  = "/v1/account/token"
	accountPasswordPath                               = "/v1/account/password"
	accountSettingsPath                               = "/v1/account/settings"
	accountSubscriptionPath                           = "/v1/account/subscription"
	accountReservationPath                            = "/v1/account/reservation"
	accountBillingPortalPath                          = "/v1/account/billing/portal"
	accountBillingWebhookPath                         = "/v1/account/billing/webhook"
	accountBillingSubscriptionPath                    = "/v1/account/billing/subscription"
	accountBillingSubscriptionCheckoutSuccessTemplate = "/v1/account/billing/subscription/success/{CHECKOUT_SESSION_ID}"
	accountBillingSubscriptionCheckoutSuccessRegex    = regexp.MustCompile(`/v1/account/billing/subscription/success/(.+)$`)
	accountReservationSingleRegex                     = regexp.MustCompile(`/v1/account/reservation/([-_A-Za-z0-9]{1,64})$`)
	accountSubscriptionSingleRegex                    = regexp.MustCompile(`^/v1/account/subscription/([-_A-Za-z0-9]{16})$`)
	matrixPushPath                                    = "/_matrix/push/v1/notify"
	staticRegex                                       = regexp.MustCompile(`^/static/.+`)
	docsRegex                                         = regexp.MustCompile(`^/docs(|/.*)$`)
	fileRegex                                         = regexp.MustCompile(`^/file/([-_A-Za-z0-9]{1,64})(?:\.[A-Za-z0-9]{1,16})?$`)
	disallowedTopics                                  = []string{"docs", "static", "file", "app", "account", "settings", "pricing", "signup", "login", "reset-password"} // If updated, also update in Android and web app
	urlRegex                                          = regexp.MustCompile(`^https?://`)

	//go:embed site
	webFs        embed.FS
	webFsCached  = &util.CachingEmbedFS{ModTime: time.Now(), FS: webFs}
	webSiteDir   = "/site"
	webHomeIndex = "/home.html" // Landing page, only if "web-root: home"
	webAppIndex  = "/app.html"  // React app

	//go:embed docs
	docsStaticFs     embed.FS
	docsStaticCached = &util.CachingEmbedFS{ModTime: time.Now(), FS: docsStaticFs}
)

const (
	firebaseControlTopic            = "~control"                // See Android if changed
	firebasePollTopic               = "~poll"                   // See iOS if changed
	emptyMessageBody                = "triggered"               // Used if message body is empty
	newMessageBody                  = "New message"             // Used in poll requests as generic message
	defaultAttachmentMessage        = "You received a file: %s" // Used if message body is empty, and there is an attachment
	encodingBase64                  = "base64"
	unifiedpushTopicPrefix          = "up"
	unifiedPushSubscriptionDuration = 12 * time.Hour
)

// WebSocket constants
const (
	wsWriteWait  = 2 * time.Second
	wsBufferSize = 1024
	wsReadLimit  = 64 // We only ever receive PINGs
	wsPongWait   = 15 * time.Second
)

// New instantiates a new Server. It creates the cache and adds a Firebase
// subscriber (if configured).
func New(conf *Config) (*Server, error) {
	var mailer mailer
	if conf.SMTPSenderAddr != "" {
		mailer = &smtpSender{config: conf}
	}
	messageCache, err := createMessageCache(conf)
	if err != nil {
		return nil, err
	}
	topics, err := messageCache.Topics()
	if err != nil {
		return nil, err
	}
	var fileCache *fileCache
	if conf.AttachmentCacheDir != "" {
		fileCache, err = newFileCache(conf.AttachmentCacheDir, conf.AttachmentTotalSizeLimit)
		if err != nil {
			return nil, err
		}
	}
	var userManager *user.Manager
	if conf.AuthFile != "" {
		userManager, err = user.NewManager(conf.AuthFile, conf.AuthStartupQueries, conf.AuthDefault)
		if err != nil {
			return nil, err
		}
	}
	var firebaseClient *firebaseClient
	if conf.FirebaseKeyFile != "" {
		sender, err := newFirebaseSender(conf.FirebaseKeyFile)
		if err != nil {
			return nil, err
		}
		firebaseClient = newFirebaseClient(sender, userManager)
	}
	return &Server{
		config:         conf,
		messageCache:   messageCache,
		fileCache:      fileCache,
		firebaseClient: firebaseClient,
		smtpSender:     mailer,
		topics:         topics,
		userManager:    userManager,
		visitors:       make(map[string]*visitor),
	}, nil
}

func createMessageCache(conf *Config) (*messageCache, error) {
	if conf.CacheDuration == 0 {
		return newNopCache()
	} else if conf.CacheFile != "" {
		return newSqliteCache(conf.CacheFile, conf.CacheStartupQueries, conf.CacheDuration, conf.CacheBatchSize, conf.CacheBatchTimeout, false)
	}
	return newMemCache()
}

// Run executes the main server. It listens on HTTP (+ HTTPS, if configured), and starts
// a manager go routine to print stats and prune messages.
func (s *Server) Run() error {
	var listenStr string
	if s.config.ListenHTTP != "" {
		listenStr += fmt.Sprintf(" %s[http]", s.config.ListenHTTP)
	}
	if s.config.ListenHTTPS != "" {
		listenStr += fmt.Sprintf(" %s[https]", s.config.ListenHTTPS)
	}
	if s.config.ListenUnix != "" {
		listenStr += fmt.Sprintf(" %s[unix]", s.config.ListenUnix)
	}
	if s.config.SMTPServerListen != "" {
		listenStr += fmt.Sprintf(" %s[smtp]", s.config.SMTPServerListen)
	}
	log.Info("Listening on%s, ntfy %s, log level is %s", listenStr, s.config.Version, log.CurrentLevel().String())
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handle)
	errChan := make(chan error)
	s.mu.Lock()
	s.closeChan = make(chan bool)
	if s.config.ListenHTTP != "" {
		s.httpServer = &http.Server{Addr: s.config.ListenHTTP, Handler: mux}
		go func() {
			errChan <- s.httpServer.ListenAndServe()
		}()
	}
	if s.config.ListenHTTPS != "" {
		s.httpsServer = &http.Server{Addr: s.config.ListenHTTPS, Handler: mux}
		go func() {
			errChan <- s.httpsServer.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		}()
	}
	if s.config.ListenUnix != "" {
		go func() {
			var err error
			s.mu.Lock()
			os.Remove(s.config.ListenUnix)
			s.unixListener, err = net.Listen("unix", s.config.ListenUnix)
			if err != nil {
				s.mu.Unlock()
				errChan <- err
				return
			}
			defer s.unixListener.Close()
			if s.config.ListenUnixMode > 0 {
				if err := os.Chmod(s.config.ListenUnix, s.config.ListenUnixMode); err != nil {
					s.mu.Unlock()
					errChan <- err
					return
				}
			}
			s.mu.Unlock()
			httpServer := &http.Server{Handler: mux}
			errChan <- httpServer.Serve(s.unixListener)
		}()
	}
	if s.config.SMTPServerListen != "" {
		go func() {
			errChan <- s.runSMTPServer()
		}()
	}
	s.mu.Unlock()
	go s.runManager()
	go s.runStatsResetter()
	go s.runDelayedSender()
	go s.runFirebaseKeepaliver()

	return <-errChan
}

// Stop stops HTTP (+HTTPS) server and all managers
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpServer != nil {
		s.httpServer.Close()
	}
	if s.httpsServer != nil {
		s.httpsServer.Close()
	}
	if s.unixListener != nil {
		s.unixListener.Close()
	}
	if s.smtpServer != nil {
		s.smtpServer.Close()
	}
	close(s.closeChan)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	v, err := s.visitor(r) // Note: Always returns v, even when error is returned
	if err == nil {
		log.Debug("%s Dispatching request", logHTTPPrefix(v, r))
		if log.IsTrace() {
			log.Trace("%s Entire request (headers and body):\n%s", logHTTPPrefix(v, r), renderHTTPRequest(r))
		}
		err = s.handleInternal(w, r, v)
	}
	if err != nil {
		if websocket.IsWebSocketUpgrade(r) {
			isNormalError := strings.Contains(err.Error(), "i/o timeout")
			if isNormalError {
				log.Debug("%s WebSocket error (this error is okay, it happens a lot): %s", logHTTPPrefix(v, r), err.Error())
			} else {
				log.Info("%s WebSocket error: %s", logHTTPPrefix(v, r), err.Error())
			}
			return // Do not attempt to write to upgraded connection
		}
		if matrixErr, ok := err.(*errMatrix); ok {
			writeMatrixError(w, r, v, matrixErr)
			return
		}
		httpErr, ok := err.(*errHTTP)
		if !ok {
			httpErr = errHTTPInternalError
		}
		isNormalError := httpErr.HTTPCode == http.StatusNotFound || httpErr.HTTPCode == http.StatusBadRequest
		if isNormalError {
			log.Debug("%s Connection closed with HTTP %d (ntfy error %d): %s", logHTTPPrefix(v, r), httpErr.HTTPCode, httpErr.Code, err.Error())
		} else {
			log.Info("%s Connection closed with HTTP %d (ntfy error %d): %s", logHTTPPrefix(v, r), httpErr.HTTPCode, httpErr.Code, err.Error())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
		w.WriteHeader(httpErr.HTTPCode)
		io.WriteString(w, httpErr.JSON()+"\n")
	}
}

func (s *Server) handleInternal(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		return s.ensureWebEnabled(s.handleHome)(w, r, v)
	} else if r.Method == http.MethodHead && r.URL.Path == "/" {
		return s.ensureWebEnabled(s.handleEmpty)(w, r, v)
	} else if r.Method == http.MethodGet && r.URL.Path == healthPath {
		return s.handleHealth(w, r, v)
	} else if r.Method == http.MethodGet && r.URL.Path == webConfigPath {
		return s.ensureWebEnabled(s.handleWebConfig)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountPath {
		return s.ensureUserManager(s.handleAccountCreate)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountTokenPath {
		return s.ensureUser(s.handleAccountTokenIssue)(w, r, v)
	} else if r.Method == http.MethodGet && r.URL.Path == accountPath {
		return s.handleAccountGet(w, r, v) // Allowed by anonymous
	} else if r.Method == http.MethodDelete && r.URL.Path == accountPath {
		return s.ensureUser(s.handleAccountDelete)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountPasswordPath {
		return s.ensureUser(s.handleAccountPasswordChange)(w, r, v)
	} else if r.Method == http.MethodPatch && r.URL.Path == accountTokenPath {
		return s.ensureUser(s.handleAccountTokenExtend)(w, r, v)
	} else if r.Method == http.MethodDelete && r.URL.Path == accountTokenPath {
		return s.ensureUser(s.handleAccountTokenDelete)(w, r, v)
	} else if r.Method == http.MethodPatch && r.URL.Path == accountSettingsPath {
		return s.ensureUser(s.handleAccountSettingsChange)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountSubscriptionPath {
		return s.ensureUser(s.handleAccountSubscriptionAdd)(w, r, v)
	} else if r.Method == http.MethodPatch && accountSubscriptionSingleRegex.MatchString(r.URL.Path) {
		return s.ensureUser(s.handleAccountSubscriptionChange)(w, r, v)
	} else if r.Method == http.MethodDelete && accountSubscriptionSingleRegex.MatchString(r.URL.Path) {
		return s.ensureUser(s.handleAccountSubscriptionDelete)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountReservationPath {
		return s.ensureUser(s.handleAccountReservationAdd)(w, r, v)
	} else if r.Method == http.MethodDelete && accountReservationSingleRegex.MatchString(r.URL.Path) {
		return s.ensureUser(s.handleAccountReservationDelete)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountBillingSubscriptionPath {
		return s.ensureUser(s.handleAccountBillingSubscriptionChange)(w, r, v)
	} else if r.Method == http.MethodDelete && r.URL.Path == accountBillingSubscriptionPath {
		return s.ensureStripeCustomer(s.handleAccountBillingSubscriptionDelete)(w, r, v)
	} else if r.Method == http.MethodGet && accountBillingSubscriptionCheckoutSuccessRegex.MatchString(r.URL.Path) {
		return s.ensureUserManager(s.handleAccountCheckoutSessionSuccessGet)(w, r, v) // No user context!
	} else if r.Method == http.MethodPost && r.URL.Path == accountBillingPortalPath {
		return s.ensureStripeCustomer(s.handleAccountBillingPortalSessionCreate)(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == accountBillingWebhookPath {
		return s.ensureUserManager(s.handleAccountBillingWebhook)(w, r, v)
	} else if r.Method == http.MethodGet && r.URL.Path == matrixPushPath {
		return s.handleMatrixDiscovery(w)
	} else if r.Method == http.MethodGet && staticRegex.MatchString(r.URL.Path) {
		return s.ensureWebEnabled(s.handleStatic)(w, r, v)
	} else if r.Method == http.MethodGet && docsRegex.MatchString(r.URL.Path) {
		return s.ensureWebEnabled(s.handleDocs)(w, r, v)
	} else if (r.Method == http.MethodGet || r.Method == http.MethodHead) && fileRegex.MatchString(r.URL.Path) && s.config.AttachmentCacheDir != "" {
		return s.limitRequests(s.handleFile)(w, r, v)
	} else if r.Method == http.MethodOptions {
		return s.ensureWebEnabled(s.handleOptions)(w, r, v)
	} else if (r.Method == http.MethodPut || r.Method == http.MethodPost) && r.URL.Path == "/" {
		return s.limitRequests(s.transformBodyJSON(s.authorizeTopicWrite(s.handlePublish)))(w, r, v)
	} else if r.Method == http.MethodPost && r.URL.Path == matrixPushPath {
		return s.limitRequests(s.transformMatrixJSON(s.authorizeTopicWrite(s.handlePublishMatrix)))(w, r, v)
	} else if (r.Method == http.MethodPut || r.Method == http.MethodPost) && topicPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicWrite(s.handlePublish))(w, r, v)
	} else if r.Method == http.MethodGet && publishPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicWrite(s.handlePublish))(w, r, v)
	} else if r.Method == http.MethodGet && jsonPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicRead(s.handleSubscribeJSON))(w, r, v)
	} else if r.Method == http.MethodGet && ssePathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicRead(s.handleSubscribeSSE))(w, r, v)
	} else if r.Method == http.MethodGet && rawPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicRead(s.handleSubscribeRaw))(w, r, v)
	} else if r.Method == http.MethodGet && wsPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicRead(s.handleSubscribeWS))(w, r, v)
	} else if r.Method == http.MethodGet && authPathRegex.MatchString(r.URL.Path) {
		return s.limitRequests(s.authorizeTopicRead(s.handleTopicAuth))(w, r, v)
	} else if r.Method == http.MethodGet && (topicPathRegex.MatchString(r.URL.Path) || externalTopicPathRegex.MatchString(r.URL.Path)) {
		return s.ensureWebEnabled(s.handleTopic)(w, r, v)
	}
	return errHTTPNotFound
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if s.config.WebRootIsApp {
		r.URL.Path = webAppIndex
	} else {
		r.URL.Path = webHomeIndex
	}
	return s.handleStatic(w, r, v)
}

func (s *Server) handleTopic(w http.ResponseWriter, r *http.Request, v *visitor) error {
	unifiedpush := readBoolParam(r, false, "x-unifiedpush", "unifiedpush", "up") // see PUT/POST too!
	if unifiedpush {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
		_, err := io.WriteString(w, `{"unifiedpush":{"version":1}}`+"\n")
		return err
	}
	r.URL.Path = webAppIndex
	return s.handleStatic(w, r, v)
}

func (s *Server) handleEmpty(_ http.ResponseWriter, _ *http.Request, _ *visitor) error {
	return nil
}

func (s *Server) handleTopicAuth(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	_, err := io.WriteString(w, `{"success":true}`+"\n")
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	response := &apiHealthResponse{
		Healthy: true,
	}
	w.Header().Set("Content-Type", "text/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleWebConfig(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	appRoot := "/"
	if !s.config.WebRootIsApp {
		appRoot = "/app"
	}
	response := &apiConfigResponse{
		BaseURL:            "", // Will translate to window.location.origin
		AppRoot:            appRoot,
		EnableLogin:        s.config.EnableLogin,
		EnableSignup:       s.config.EnableSignup,
		EnablePayments:     s.config.EnablePayments,
		EnableReservations: s.config.EnableReservations,
		DisallowedTopics:   disallowedTopics,
	}
	b, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/javascript")
	_, err = io.WriteString(w, fmt.Sprintf("// Generated server configuration\nvar config = %s;\n", string(b)))
	return err
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	r.URL.Path = webSiteDir + r.URL.Path
	util.Gzip(http.FileServer(http.FS(webFsCached))).ServeHTTP(w, r)
	return nil
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request, _ *visitor) error {
	util.Gzip(http.FileServer(http.FS(docsStaticCached))).ServeHTTP(w, r)
	return nil
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if s.config.AttachmentCacheDir == "" {
		return errHTTPInternalError
	}
	matches := fileRegex.FindStringSubmatch(r.URL.Path)
	if len(matches) != 2 {
		return errHTTPInternalErrorInvalidPath
	}
	messageID := matches[1]
	file := filepath.Join(s.config.AttachmentCacheDir, messageID)
	stat, err := os.Stat(file)
	if err != nil {
		return errHTTPNotFound
	}
	if r.Method == http.MethodGet {
		if err := v.BandwidthLimiter().Allow(stat.Size()); err != nil {
			return errHTTPTooManyRequestsLimitAttachmentBandwidth
		}
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	if r.Method == http.MethodGet {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(util.NewContentTypeWriter(w, r.URL.Path), f)
		return err
	}
	return nil
}

func (s *Server) handleMatrixDiscovery(w http.ResponseWriter) error {
	if s.config.BaseURL == "" {
		return errHTTPInternalErrorMissingBaseURL
	}
	return writeMatrixDiscoveryResponse(w)
}

func (s *Server) handlePublishWithoutResponse(r *http.Request, v *visitor) (*message, error) {
	t, err := s.topicFromPath(r.URL.Path)
	if err != nil {
		return nil, err
	}

	var v_billing *visitor
	if strings.HasPrefix(t.ID, unifiedpushTopicPrefix) {
		v_billing := t.getBillee()
		if v_billing != nil {
			// instant reject and won't even store it if there's no one registered for a UP topic in the past some time
	// need to find error code for device not available try again later
			return nil, errHTTPInternalError
		}
	}

	if err := v.MessageAllowed(); err != nil {
		return nil, errHTTPTooManyRequestsLimitMessages
	}
	body, err := util.Peek(r.Body, s.config.MessageLimit)
	if err != nil {
		return nil, err
	}
	m := newDefaultMessage(t.ID, "")
	cache, firebase, email, unifiedpush, err := s.parsePublishParams(r, v, m)
	if err != nil {
		return nil, err
	}
	if m.PollID != "" {
		m = newPollRequestMessage(t.ID, m.PollID)
	}
	if v.user != nil {
		m.User = v.user.Name
	}
	m.Expires = time.Now().Add(v.Limits().MessagesExpiryDuration).Unix()
	if err := s.handlePublishBody(r, v, m, body, unifiedpush); err != nil {
		return nil, err
	}
	if m.Message == "" {
		m.Message = emptyMessageBody
	}
	// we do not need to handle delays, because 1. for UP delays are not needed 2. if the up app server is adding a delay it is shooting itself in the foot
	delayed := m.Time > time.Now().Unix()
	log.Debug("%s Received message: event=%s, user=%s, body=%d byte(s), delayed=%t, firebase=%t, cache=%t, up=%t, email=%s",
		logMessagePrefix(v, m), m.Event, m.User, len(m.Message), delayed, firebase, cache, unifiedpush, email)
	if log.IsTrace() {
		log.Trace("%s Message body: %s", logMessagePrefix(v, m), util.MaybeMarshalJSON(m))
	}
	if !delayed {
		if err := t.Publish(v, m); err != nil {
			return nil, err
		}
		if s.firebaseClient != nil && firebase {
			go s.sendToFirebase(v, m)
		}
		// same as delays, it should count against app servers
		if s.smtpSender != nil && email != "" {
			v.IncrementEmails()
			go s.sendEmail(v, m, email)
		}
		if s.config.UpstreamBaseURL != "" {
			go s.forwardPollRequest(v, m)
		}
	} else {
		log.Debug("%s Message delayed, will process later", logMessagePrefix(v, m))
	}
	if cache {
		log.Debug("%s Adding message to cache", logMessagePrefix(v, m))
		if err := s.messageCache.AddMessage(m); err != nil {
			return nil, err
		}
	}
	if v_billing != nil {
		v_billing.IncrementMessages()
	} else {
		v.IncrementMessages()
	}
	if s.userManager != nil && v.user != nil {
		s.userManager.EnqueueStats(v.user)
	}
	s.mu.Lock()
	s.messages++
	s.mu.Unlock()
	return m, nil
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request, v *visitor) error {
	m, err := s.handlePublishWithoutResponse(r, v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	if err := json.NewEncoder(w).Encode(m); err != nil {
		return err
	}
	return nil
}

func (s *Server) handlePublishMatrix(w http.ResponseWriter, r *http.Request, v *visitor) error {
	_, err := s.handlePublishWithoutResponse(r, v)
	if err != nil {
		return &errMatrix{pushKey: r.Header.Get(matrixPushKeyHeader), err: err}
	}
	return writeMatrixSuccess(w)
}

func (s *Server) sendToFirebase(v *visitor, m *message) {
	log.Debug("%s Publishing to Firebase", logMessagePrefix(v, m))
	if err := s.firebaseClient.Send(v, m); err != nil {
		if err == errFirebaseTemporarilyBanned {
			log.Debug("%s Unable to publish to Firebase: %v", logMessagePrefix(v, m), err.Error())
		} else {
			log.Warn("%s Unable to publish to Firebase: %v", logMessagePrefix(v, m), err.Error())
		}
	}
}

func (s *Server) sendEmail(v *visitor, m *message, email string) {
	log.Debug("%s Sending email to %s", logMessagePrefix(v, m), email)
	if err := s.smtpSender.Send(v, m, email); err != nil {
		log.Warn("%s Unable to send email to %s: %v", logMessagePrefix(v, m), email, err.Error())
	}
}

func (s *Server) forwardPollRequest(v *visitor, m *message) {
	topicURL := fmt.Sprintf("%s/%s", s.config.BaseURL, m.Topic)
	topicHash := fmt.Sprintf("%x", sha256.Sum256([]byte(topicURL)))
	forwardURL := fmt.Sprintf("%s/%s", s.config.UpstreamBaseURL, topicHash)
	log.Debug("%s Publishing poll request to %s", logMessagePrefix(v, m), forwardURL)
	req, err := http.NewRequest("POST", forwardURL, strings.NewReader(""))
	if err != nil {
		log.Warn("%s Unable to publish poll request: %v", logMessagePrefix(v, m), err.Error())
		return
	}
	req.Header.Set("X-Poll-ID", m.ID)
	var httpClient = &http.Client{
		Timeout: time.Second * 10,
	}
	response, err := httpClient.Do(req)
	if err != nil {
		log.Warn("%s Unable to publish poll request: %v", logMessagePrefix(v, m), err.Error())
		return
	} else if response.StatusCode != http.StatusOK {
		log.Warn("%s Unable to publish poll request, unexpected HTTP status: %d", logMessagePrefix(v, m), response.StatusCode)
		return
	}
}

func (s *Server) parsePublishParams(r *http.Request, v *visitor, m *message) (cache bool, firebase bool, email string, unifiedpush bool, err error) {
	cache = readBoolParam(r, true, "x-cache", "cache")
	firebase = readBoolParam(r, true, "x-firebase", "firebase")
	m.Title = readParam(r, "x-title", "title", "t")
	m.Click = readParam(r, "x-click", "click")
	icon := readParam(r, "x-icon", "icon")
	filename := readParam(r, "x-filename", "filename", "file", "f")
	attach := readParam(r, "x-attach", "attach", "a")
	if attach != "" || filename != "" {
		m.Attachment = &attachment{}
	}
	if filename != "" {
		m.Attachment.Name = filename
	}
	if attach != "" {
		if !urlRegex.MatchString(attach) {
			return false, false, "", false, errHTTPBadRequestAttachmentURLInvalid
		}
		m.Attachment.URL = attach
		if m.Attachment.Name == "" {
			u, err := url.Parse(m.Attachment.URL)
			if err == nil {
				m.Attachment.Name = path.Base(u.Path)
				if m.Attachment.Name == "." || m.Attachment.Name == "/" {
					m.Attachment.Name = ""
				}
			}
		}
		if m.Attachment.Name == "" {
			m.Attachment.Name = "attachment"
		}
	}
	if icon != "" {
		if !urlRegex.MatchString(icon) {
			return false, false, "", false, errHTTPBadRequestIconURLInvalid
		}
		m.Icon = icon
	}
	email = readParam(r, "x-email", "x-e-mail", "email", "e-mail", "mail", "e")
	if email != "" {
		if err := v.EmailAllowed(); err != nil {
			return false, false, "", false, errHTTPTooManyRequestsLimitEmails
		}
	}
	if s.smtpSender == nil && email != "" {
		return false, false, "", false, errHTTPBadRequestEmailDisabled
	}
	messageStr := strings.ReplaceAll(readParam(r, "x-message", "message", "m"), "\\n", "\n")
	if messageStr != "" {
		m.Message = messageStr
	}
	m.Priority, err = util.ParsePriority(readParam(r, "x-priority", "priority", "prio", "p"))
	if err != nil {
		return false, false, "", false, errHTTPBadRequestPriorityInvalid
	}
	tagsStr := readParam(r, "x-tags", "tags", "tag", "ta")
	if tagsStr != "" {
		m.Tags = make([]string, 0)
		for _, s := range util.SplitNoEmpty(tagsStr, ",") {
			m.Tags = append(m.Tags, strings.TrimSpace(s))
		}
	}
	delayStr := readParam(r, "x-delay", "delay", "x-at", "at", "x-in", "in")
	if delayStr != "" {
		if !cache {
			return false, false, "", false, errHTTPBadRequestDelayNoCache
		}
		if email != "" {
			return false, false, "", false, errHTTPBadRequestDelayNoEmail // we cannot store the email address (yet)
		}
		delay, err := util.ParseFutureTime(delayStr, time.Now())
		if err != nil {
			return false, false, "", false, errHTTPBadRequestDelayCannotParse
		} else if delay.Unix() < time.Now().Add(s.config.MinDelay).Unix() {
			return false, false, "", false, errHTTPBadRequestDelayTooSmall
		} else if delay.Unix() > time.Now().Add(s.config.MaxDelay).Unix() {
			return false, false, "", false, errHTTPBadRequestDelayTooLarge
		}
		m.Time = delay.Unix()
		m.Sender = v.ip // Important for rate limiting
	}
	actionsStr := readParam(r, "x-actions", "actions", "action")
	if actionsStr != "" {
		m.Actions, err = parseActions(actionsStr)
		if err != nil {
			return false, false, "", false, wrapErrHTTP(errHTTPBadRequestActionsInvalid, err.Error())
		}
	}
	unifiedpush = readBoolParam(r, false, "x-unifiedpush", "unifiedpush", "up") // see GET too!
	if unifiedpush {
		firebase = false
		unifiedpush = true
	}
	m.PollID = readParam(r, "x-poll-id", "poll-id")
	if m.PollID != "" {
		unifiedpush = false
		cache = false
		email = ""
	}
	return cache, firebase, email, unifiedpush, nil
}

// handlePublishBody consumes the PUT/POST body and decides whether the body is an attachment or the message.
//
//  1. curl -X POST -H "Poll: 1234" ntfy.sh/...
//     If a message is flagged as poll request, the body does not matter and is discarded
//  2. curl -T somebinarydata.bin "ntfy.sh/mytopic?up=1"
//     If body is binary, encode as base64, if not do not encode
//  3. curl -H "Attach: http://example.com/file.jpg" ntfy.sh/mytopic
//     Body must be a message, because we attached an external URL
//  4. curl -T short.txt -H "Filename: short.txt" ntfy.sh/mytopic
//     Body must be attachment, because we passed a filename
//  5. curl -T file.txt ntfy.sh/mytopic
//     If file.txt is <= 4096 (message limit) and valid UTF-8, treat it as a message
//  6. curl -T file.txt ntfy.sh/mytopic
//     If file.txt is > message limit, treat it as an attachment
func (s *Server) handlePublishBody(r *http.Request, v *visitor, m *message, body *util.PeekedReadCloser, unifiedpush bool) error {
	if m.Event == pollRequestEvent { // Case 1
		return s.handleBodyDiscard(body)
	} else if unifiedpush {
		return s.handleBodyAsMessageAutoDetect(m, body) // Case 2
	} else if m.Attachment != nil && m.Attachment.URL != "" {
		return s.handleBodyAsTextMessage(m, body) // Case 3
	} else if m.Attachment != nil && m.Attachment.Name != "" {
		return s.handleBodyAsAttachment(r, v, m, body) // Case 4
	} else if !body.LimitReached && utf8.Valid(body.PeekedBytes) {
		return s.handleBodyAsTextMessage(m, body) // Case 5
	}
	return s.handleBodyAsAttachment(r, v, m, body) // Case 6
}

func (s *Server) handleBodyDiscard(body *util.PeekedReadCloser) error {
	_, err := io.Copy(io.Discard, body)
	_ = body.Close()
	return err
}

func (s *Server) handleBodyAsMessageAutoDetect(m *message, body *util.PeekedReadCloser) error {
	if utf8.Valid(body.PeekedBytes) {
		m.Message = string(body.PeekedBytes) // Do not trim
	} else {
		m.Message = base64.StdEncoding.EncodeToString(body.PeekedBytes)
		m.Encoding = encodingBase64
	}
	return nil
}

func (s *Server) handleBodyAsTextMessage(m *message, body *util.PeekedReadCloser) error {
	if !utf8.Valid(body.PeekedBytes) {
		return errHTTPBadRequestMessageNotUTF8
	}
	if len(body.PeekedBytes) > 0 { // Empty body should not override message (publish via GET!)
		m.Message = strings.TrimSpace(string(body.PeekedBytes)) // Truncates the message to the peek limit if required
	}
	if m.Attachment != nil && m.Attachment.Name != "" && m.Message == "" {
		m.Message = fmt.Sprintf(defaultAttachmentMessage, m.Attachment.Name)
	}
	return nil
}

func (s *Server) handleBodyAsAttachment(r *http.Request, v *visitor, m *message, body *util.PeekedReadCloser) error {
	if s.fileCache == nil || s.config.BaseURL == "" || s.config.AttachmentCacheDir == "" {
		return errHTTPBadRequestAttachmentsDisallowed
	}
	vinfo, err := v.Info()
	if err != nil {
		return err
	}
	attachmentExpiry := time.Now().Add(vinfo.Limits.AttachmentExpiryDuration).Unix()
	if m.Time > attachmentExpiry {
		return errHTTPBadRequestAttachmentsExpiryBeforeDelivery
	}
	contentLengthStr := r.Header.Get("Content-Length")
	if contentLengthStr != "" { // Early "do-not-trust" check, hard limit see below
		contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
		if err == nil && (contentLength > vinfo.Stats.AttachmentTotalSizeRemaining || contentLength > vinfo.Limits.AttachmentFileSizeLimit) {
			return errHTTPEntityTooLargeAttachment
		}
	}
	if m.Attachment == nil {
		m.Attachment = &attachment{}
	}
	var ext string
	m.Sender = v.ip // Important for attachment rate limiting
	m.Attachment.Expires = attachmentExpiry
	m.Attachment.Type, ext = util.DetectContentType(body.PeekedBytes, m.Attachment.Name)
	m.Attachment.URL = fmt.Sprintf("%s/file/%s%s", s.config.BaseURL, m.ID, ext)
	if m.Attachment.Name == "" {
		m.Attachment.Name = fmt.Sprintf("attachment%s", ext)
	}
	if m.Message == "" {
		m.Message = fmt.Sprintf(defaultAttachmentMessage, m.Attachment.Name)
	}
	limiters := []util.Limiter{
		v.BandwidthLimiter(),
		util.NewFixedLimiter(vinfo.Limits.AttachmentFileSizeLimit),
		util.NewFixedLimiter(vinfo.Stats.AttachmentTotalSizeRemaining),
	}
	m.Attachment.Size, err = s.fileCache.Write(m.ID, body, limiters...)
	if err == util.ErrLimitReached {
		return errHTTPEntityTooLargeAttachment
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Server) handleSubscribeJSON(w http.ResponseWriter, r *http.Request, v *visitor) error {
	encoder := func(msg *message) (string, error) {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(&msg); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	return s.handleSubscribeHTTP(w, r, v, "application/x-ndjson", encoder)
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
	return s.handleSubscribeHTTP(w, r, v, "text/event-stream", encoder)
}

func (s *Server) handleSubscribeRaw(w http.ResponseWriter, r *http.Request, v *visitor) error {
	encoder := func(msg *message) (string, error) {
		if msg.Event == messageEvent { // only handle default events
			return strings.ReplaceAll(msg.Message, "\n", " ") + "\n", nil
		}
		return "\n", nil // "keepalive" and "open" events just send an empty line
	}
	return s.handleSubscribeHTTP(w, r, v, "text/plain", encoder)
}

func (s *Server) handleSubscribeHTTP(w http.ResponseWriter, r *http.Request, v *visitor, contentType string, encoder messageEncoder) error {
	log.Debug("%s HTTP stream connection opened", logHTTPPrefix(v, r))
	defer log.Debug("%s HTTP stream connection closed", logHTTPPrefix(v, r))
	if err := v.SubscriptionAllowed(); err != nil {
		return errHTTPTooManyRequestsLimitSubscriptions
	}
	defer v.RemoveSubscription()
	topics, topicsStr, err := s.topicsFromPath(r.URL.Path)
	if err != nil {
		return err
	}
	poll, since, scheduled, filters, err := parseSubscribeParams(r)
	if err != nil {
		return err
	}
	var wlock sync.Mutex
	defer func() {
		// Hack: This is the fix for a horrible data race that I have not been able to figure out in quite some time.
		// It appears to be happening when the Go HTTP code reads from the socket when closing the request (i.e. AFTER
		// this function returns), and causes a data race with the ResponseWriter. Locking wlock here silences the
		// data race detector. See https://github.com/binwiederhier/ntfy/issues/338#issuecomment-1163425889.
		wlock.TryLock()
	}()
	sub := func(v *visitor, msg *message) error {
		if !filters.Pass(msg) {
			return nil
		}
		m, err := encoder(msg)
		if err != nil {
			return err
		}
		wlock.Lock()
		defer wlock.Unlock()
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
		return s.sendOldMessages(topics, since, scheduled, v, sub)
	}
	subscriberIDs := make([]int, 0)
	for _, t := range topics {
		subscriberIDs = append(subscriberIDs, t.Subscribe(sub, v))
	}
	defer func() {
		for i, subscriberID := range subscriberIDs {
			topics[i].Unsubscribe(subscriberID) // Order!
		}
	}()
	if err := sub(v, newOpenMessage(topicsStr)); err != nil { // Send out open message
		return err
	}
	if err := s.sendOldMessages(topics, since, scheduled, v, sub); err != nil {
		return err
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-time.After(s.config.KeepaliveInterval):
			log.Trace("%s Sending keepalive message", logHTTPPrefix(v, r))
			v.Keepalive()
			if err := sub(v, newKeepaliveMessage(topicsStr)); err != nil { // Send keepalive message
				return err
			}
		}
	}
}

func (s *Server) handleSubscribeWS(w http.ResponseWriter, r *http.Request, v *visitor) error {
	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		return errHTTPBadRequestWebSocketsUpgradeHeaderMissing
	}
	if err := v.SubscriptionAllowed(); err != nil {
		return errHTTPTooManyRequestsLimitSubscriptions
	}
	defer v.RemoveSubscription()
	log.Debug("%s WebSocket connection opened", logHTTPPrefix(v, r))
	defer log.Debug("%s WebSocket connection closed", logHTTPPrefix(v, r))
	topics, topicsStr, err := s.topicsFromPath(r.URL.Path)
	if err != nil {
		return err
	}
	poll, since, scheduled, filters, err := parseSubscribeParams(r)
	if err != nil {
		return err
	}
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  wsBufferSize,
		WriteBufferSize: wsBufferSize,
		CheckOrigin: func(r *http.Request) bool {
			return true // We're open for business!
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	var wlock sync.Mutex
	g, ctx := errgroup.WithContext(context.Background())
	g.Go(func() error {
		pongWait := s.config.KeepaliveInterval + wsPongWait
		conn.SetReadLimit(wsReadLimit)
		if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
			return err
		}
		conn.SetPongHandler(func(appData string) error {
			log.Trace("%s Received WebSocket pong", logHTTPPrefix(v, r))
			return conn.SetReadDeadline(time.Now().Add(pongWait))
		})
		for {
			_, _, err := conn.NextReader()
			if err != nil {
				return err
			}
		}
	})
	g.Go(func() error {
		ping := func() error {
			wlock.Lock()
			defer wlock.Unlock()
			if err := conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
				return err
			}
			log.Trace("%s Sending WebSocket ping", logHTTPPrefix(v, r))
			return conn.WriteMessage(websocket.PingMessage, nil)
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(s.config.KeepaliveInterval):
				v.Keepalive()
				if err := ping(); err != nil {
					return err
				}
			}
		}
	})
	sub := func(v *visitor, msg *message) error {
		if !filters.Pass(msg) {
			return nil
		}
		wlock.Lock()
		defer wlock.Unlock()
		if err := conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
			return err
		}
		return conn.WriteJSON(msg)
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // CORS, allow cross-origin requests
	if poll {
		return s.sendOldMessages(topics, since, scheduled, v, sub)
	}
	subscriberIDs := make([]int, 0)
	for _, t := range topics {
		subscriberIDs = append(subscriberIDs, t.Subscribe(sub, v))
	}
	defer func() {
		for i, subscriberID := range subscriberIDs {
			topics[i].Unsubscribe(subscriberID) // Order!
		}
	}()
	if err := sub(v, newOpenMessage(topicsStr)); err != nil { // Send out open message
		return err
	}
	if err := s.sendOldMessages(topics, since, scheduled, v, sub); err != nil {
		return err
	}
	err = g.Wait()
	if err != nil && websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
		log.Trace("%s WebSocket connection closed: %s", logHTTPPrefix(v, r), err.Error())
		return nil // Normal closures are not errors; note: "1006 (abnormal closure)" is treated as normal, because people disconnect a lot
	}
	return err
}

func parseSubscribeParams(r *http.Request) (poll bool, since sinceMarker, scheduled bool, filters *queryFilter, err error) {
	poll = readBoolParam(r, false, "x-poll", "poll", "po")
	scheduled = readBoolParam(r, false, "x-scheduled", "scheduled", "sched")
	since, err = parseSince(r, poll)
	if err != nil {
		return
	}
	filters, err = parseQueryFilters(r)
	if err != nil {
		return
	}
	return
}

// sendOldMessages selects old messages from the messageCache and calls sub for each of them. It uses since as the
// marker, returning only messages that are newer than the marker.
func (s *Server) sendOldMessages(topics []*topic, since sinceMarker, scheduled bool, v *visitor, sub subscriber) error {
	if since.IsNone() {
		return nil
	}
	messages := make([]*message, 0)
	for _, t := range topics {
		topicMessages, err := s.messageCache.Messages(t.ID, since, scheduled)
		if err != nil {
			return err
		}
		messages = append(messages, topicMessages...)
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Time < messages[j].Time
	})
	for _, m := range messages {
		if err := sub(v, m); err != nil {
			return err
		}
	}
	return nil
}

// parseSince returns a timestamp identifying the time span from which cached messages should be received.
//
// Values in the "since=..." parameter can be either a unix timestamp or a duration (e.g. 12h), or
// "all" for all messages.
func parseSince(r *http.Request, poll bool) (sinceMarker, error) {
	since := readParam(r, "x-since", "since", "si")

	// Easy cases (empty, all, none)
	if since == "" {
		if poll {
			return sinceAllMessages, nil
		}
		return sinceNoMessages, nil
	} else if since == "all" {
		return sinceAllMessages, nil
	} else if since == "none" {
		return sinceNoMessages, nil
	}

	// ID, timestamp, duration
	if validMessageID(since) {
		return newSinceID(since), nil
	} else if s, err := strconv.ParseInt(since, 10, 64); err == nil {
		return newSinceTime(s), nil
	} else if d, err := time.ParseDuration(since); err == nil {
		return newSinceTime(time.Now().Add(-1 * d).Unix()), nil
	}
	return sinceNoMessages, errHTTPBadRequestSinceInvalid
}

func (s *Server) handleOptions(w http.ResponseWriter, _ *http.Request, _ *visitor) error {
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, PATCH, DELETE")
	w.Header().Set("Access-Control-Allow-Origin", "*")  // CORS, allow cross-origin requests
	w.Header().Set("Access-Control-Allow-Headers", "*") // CORS, allow auth via JS // FIXME is this terrible?
	return nil
}

func (s *Server) topicFromPath(path string) (*topic, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, errHTTPBadRequestTopicInvalid
	}
	topics, err := s.topicsFromIDs(parts[1])
	if err != nil {
		return nil, err
	}
	return topics[0], nil
}

func (s *Server) topicsFromPath(path string) ([]*topic, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, "", errHTTPBadRequestTopicInvalid
	}
	topicIDs := util.SplitNoEmpty(parts[1], ",")
	topics, err := s.topicsFromIDs(topicIDs...)
	if err != nil {
		return nil, "", errHTTPBadRequestTopicInvalid
	}
	return topics, parts[1], nil
}

func (s *Server) topicsFromIDs(ids ...string) ([]*topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	topics := make([]*topic, 0)
	for _, id := range ids {
		if util.Contains(disallowedTopics, id) {
			return nil, errHTTPBadRequestTopicDisallowed
		}
		if _, ok := s.topics[id]; !ok {
			if len(s.topics) >= s.config.TotalTopicLimit {
				return nil, errHTTPTooManyRequestsLimitTotalTopics
			}
			s.topics[id] = newTopic(id)
		}
		topics = append(topics, s.topics[id])
	}
	return topics, nil
}

func (s *Server) execManager() {
	log.Debug("Manager: Starting")
	defer log.Debug("Manager: Finished")

	// WARNING: Make sure to only selectively lock with the mutex, and be aware that this
	//          there is no mutex for the entire function.

	// Expire visitors from rate visitors map
	s.mu.Lock()
	staleVisitors := 0
	for ip, v := range s.visitors {
		if v.Stale() {
			log.Trace("Deleting stale visitor %s", v.ip)
			delete(s.visitors, ip)
			staleVisitors++
		}
	}
	s.mu.Unlock()
	log.Debug("Manager: Deleted %d stale visitor(s)", staleVisitors)

	// Delete expired user tokens
	if s.userManager != nil {
		if err := s.userManager.RemoveExpiredTokens(); err != nil {
			log.Warn("Error expiring user tokens: %s", err.Error())
		}
	}

	// Delete expired attachments
	if s.fileCache != nil {
		ids, err := s.messageCache.AttachmentsExpired()
		if err != nil {
			log.Warn("Manager: Error retrieving expired attachments: %s", err.Error())
		} else if len(ids) > 0 {
			if log.IsDebug() {
				log.Debug("Manager: Deleting attachments %s", strings.Join(ids, ", "))
			}
			if err := s.fileCache.Remove(ids...); err != nil {
				log.Warn("Manager: Error deleting attachments: %s", err.Error())
			}
			if err := s.messageCache.MarkAttachmentsDeleted(ids...); err != nil {
				log.Warn("Manager: Error marking attachments deleted: %s", err.Error())
			}
		} else {
			log.Debug("Manager: No expired attachments to delete")
		}
	}

	// DeleteMessages message cache
	log.Debug("Manager: Pruning messages")
	expiredMessages, err := s.messageCache.MessagesExpired()
	if err != nil {
		log.Warn("Manager: Error retrieving expired messages: %s", err.Error())
	} else if len(expiredMessages) > 0 {
		ids := make([]string, 0)
		for _, m := range expiredMessages {
			ids = append(ids, m.ID)
		}
		if err := s.fileCache.Remove(ids...); err != nil {
			log.Warn("Manager: Error deleting attachments for expired messages: %s", err.Error())
		}
		if err := s.messageCache.DeleteMessages(ids...); err != nil {
			log.Warn("Manager: Error marking attachments deleted: %s", err.Error())
		}
	} else {
		log.Debug("Manager: No expired messages to delete")
	}

	// Message count per topic
	var messages int
	messageCounts, err := s.messageCache.MessageCounts()
	if err != nil {
		log.Warn("Manager: Cannot get message counts: %s", err.Error())
		messageCounts = make(map[string]int) // Empty, so we can continue
	}
	for _, count := range messageCounts {
		messages += count
	}

	// Remove subscriptions without subscribers
	s.mu.Lock()
	var subscribers int
	for _, t := range s.topics {
		subs := t.SubscribersCount()
		msgs, exists := messageCounts[t.ID]
		if subs == 0 && (!exists || msgs == 0) {
			log.Trace("Deleting empty topic %s", t.ID)
			delete(s.topics, t.ID)
			continue
		}
		subscribers += subs
	}
	s.mu.Unlock()

	// Mail stats
	var receivedMailTotal, receivedMailSuccess, receivedMailFailure int64
	if s.smtpServerBackend != nil {
		receivedMailTotal, receivedMailSuccess, receivedMailFailure = s.smtpServerBackend.Counts()
	}
	var sentMailTotal, sentMailSuccess, sentMailFailure int64
	if s.smtpSender != nil {
		sentMailTotal, sentMailSuccess, sentMailFailure = s.smtpSender.Counts()
	}

	// Print stats
	s.mu.Lock()
	messagesCount, topicsCount, visitorsCount := s.messages, len(s.topics), len(s.visitors)
	s.mu.Unlock()
	log.Info("Stats: %d messages published, %d in cache, %d topic(s) active, %d subscriber(s), %d visitor(s), %d mails received (%d successful, %d failed), %d mails sent (%d successful, %d failed)",
		messagesCount, messages, topicsCount, subscribers, visitorsCount,
		receivedMailTotal, receivedMailSuccess, receivedMailFailure,
		sentMailTotal, sentMailSuccess, sentMailFailure)
}

func (s *Server) runSMTPServer() error {
	s.smtpServerBackend = newMailBackend(s.config, s.handle)
	s.smtpServer = smtp.NewServer(s.smtpServerBackend)
	s.smtpServer.Addr = s.config.SMTPServerListen
	s.smtpServer.Domain = s.config.SMTPServerDomain
	s.smtpServer.ReadTimeout = 10 * time.Second
	s.smtpServer.WriteTimeout = 10 * time.Second
	s.smtpServer.MaxMessageBytes = 1024 * 1024 // Must be much larger than message size (headers, multipart, etc.)
	s.smtpServer.MaxRecipients = 1
	s.smtpServer.AllowInsecureAuth = true
	return s.smtpServer.ListenAndServe()
}

func (s *Server) runManager() {
	for {
		select {
		case <-time.After(s.config.ManagerInterval):
			s.execManager()
		case <-s.closeChan:
			return
		}
	}
}

func (s *Server) runStatsResetter() {
	for {
		runAt := util.NextOccurrenceUTC(s.config.VisitorStatsResetTime, time.Now())
		timer := time.NewTimer(time.Until(runAt))
		log.Debug("Stats resetter: Waiting until %v to reset visitor stats", runAt)
		select {
		case <-timer.C:
			s.resetStats()
		case <-s.closeChan:
			timer.Stop()
			return
		}
	}
}

func (s *Server) resetStats() {
	log.Info("Resetting all visitor stats (daily task)")
	s.mu.Lock()
	defer s.mu.Unlock() // Includes the database query to avoid races with other processes
	for _, v := range s.visitors {
		v.ResetStats()
	}
	if s.userManager != nil {
		if err := s.userManager.ResetStats(); err != nil {
			log.Warn("Failed to write to database: %s", err.Error())
		}
	}
}

func (s *Server) runFirebaseKeepaliver() {
	if s.firebaseClient == nil {
		return
	}
	v := newVisitor(s.config, s.messageCache, s.userManager, netip.IPv4Unspecified(), nil) // Background process, not a real visitor, uses IP 0.0.0.0
	for {
		select {
		case <-time.After(s.config.FirebaseKeepaliveInterval):
			s.sendToFirebase(v, newKeepaliveMessage(firebaseControlTopic))
		case <-time.After(s.config.FirebasePollInterval):
			s.sendToFirebase(v, newKeepaliveMessage(firebasePollTopic))
		case <-s.closeChan:
			return
		}
	}
}

func (s *Server) runDelayedSender() {
	for {
		select {
		case <-time.After(s.config.DelayedSenderInterval):
			if err := s.sendDelayedMessages(); err != nil {
				log.Warn("Error sending delayed messages: %s", err.Error())
			}
		case <-s.closeChan:
			return
		}
	}
}

func (s *Server) sendDelayedMessages() error {
	messages, err := s.messageCache.MessagesDue()
	if err != nil {
		return err
	}
	for _, m := range messages {
		var v *visitor
		if s.userManager != nil && m.User != "" {
			user, err := s.userManager.User(m.User)
			if err != nil {
				log.Warn("%s Error sending delayed message: %s", logMessagePrefix(v, m), err.Error())
				continue
			}
			v = s.visitorFromUser(user, m.Sender)
		} else {
			v = s.visitorFromIP(m.Sender)
		}
		if err := s.sendDelayedMessage(v, m); err != nil {
			log.Warn("%s Error sending delayed message: %s", logMessagePrefix(v, m), err.Error())
		}
	}
	return nil
}

func (s *Server) sendDelayedMessage(v *visitor, m *message) error {
	log.Debug("%s Sending delayed message", logMessagePrefix(v, m))
	s.mu.Lock()
	t, ok := s.topics[m.Topic] // If no subscribers, just mark message as published
	s.mu.Unlock()
	if ok {
		go func() {
			// We do not rate-limit messages here, since we've rate limited them in the PUT/POST handler
			if err := t.Publish(v, m); err != nil {
				log.Warn("%s Unable to publish message: %v", logMessagePrefix(v, m), err.Error())
			}
		}()
	}
	if s.firebaseClient != nil { // Firebase subscribers may not show up in topics map
		go s.sendToFirebase(v, m)
	}
	if s.config.UpstreamBaseURL != "" {
		go s.forwardPollRequest(v, m)
	}
	if err := s.messageCache.MarkPublished(m); err != nil {
		return err
	}
	return nil
}

func (s *Server) limitRequests(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if util.ContainsIP(s.config.VisitorRequestExemptIPAddrs, v.ip) {
			return next(w, r, v)
		} else if err := v.RequestAllowed(); err != nil {
			return errHTTPTooManyRequestsLimitRequests
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureWebEnabled(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if !s.config.EnableWeb {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureUserManager(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.userManager == nil {
			return errHTTPNotFound
		}
		return next(w, r, v)
	}
}

func (s *Server) ensureUser(next handleFunc) handleFunc {
	return s.ensureUserManager(func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if v.user == nil {
			return errHTTPUnauthorized
		}
		return next(w, r, v)
	})
}

func (s *Server) ensureStripeCustomer(next handleFunc) handleFunc {
	return s.ensureUser(func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if v.user.Billing.StripeCustomerID == "" {
			return errHTTPBadRequestNotAPaidUser
		}
		return next(w, r, v)
	})
}

// transformBodyJSON peeks the request body, reads the JSON, and converts it to headers
// before passing it on to the next handler. This is meant to be used in combination with handlePublish.
func (s *Server) transformBodyJSON(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		m, err := readJSONWithLimit[publishMessage](r.Body, s.config.MessageLimit)
		if err != nil {
			return err
		}
		if !topicRegex.MatchString(m.Topic) {
			return errHTTPBadRequestTopicInvalid
		}
		if m.Message == "" {
			m.Message = emptyMessageBody
		}
		r.URL.Path = "/" + m.Topic
		r.Body = io.NopCloser(strings.NewReader(m.Message))
		if m.Title != "" {
			r.Header.Set("X-Title", m.Title)
		}
		if m.Priority != 0 {
			r.Header.Set("X-Priority", fmt.Sprintf("%d", m.Priority))
		}
		if m.Tags != nil && len(m.Tags) > 0 {
			r.Header.Set("X-Tags", strings.Join(m.Tags, ","))
		}
		if m.Attach != "" {
			r.Header.Set("X-Attach", m.Attach)
		}
		if m.Filename != "" {
			r.Header.Set("X-Filename", m.Filename)
		}
		if m.Click != "" {
			r.Header.Set("X-Click", m.Click)
		}
		if m.Icon != "" {
			r.Header.Set("X-Icon", m.Icon)
		}
		if len(m.Actions) > 0 {
			actionsStr, err := json.Marshal(m.Actions)
			if err != nil {
				return errHTTPBadRequestMessageJSONInvalid
			}
			r.Header.Set("X-Actions", string(actionsStr))
		}
		if m.Email != "" {
			r.Header.Set("X-Email", m.Email)
		}
		if m.Delay != "" {
			r.Header.Set("X-Delay", m.Delay)
		}
		return next(w, r, v)
	}
}

func (s *Server) transformMatrixJSON(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		newRequest, err := newRequestFromMatrixJSON(r, s.config.BaseURL, s.config.MessageLimit)
		if err != nil {
			return err
		}
		if err := next(w, newRequest, v); err != nil {
			return &errMatrix{pushKey: newRequest.Header.Get(matrixPushKeyHeader), err: err}
		}
		return nil
	}
}

func (s *Server) authorizeTopicWrite(next handleFunc) handleFunc {
	return s.autorizeTopic(next, user.PermissionWrite)
}

func (s *Server) authorizeTopicRead(next handleFunc) handleFunc {
	return s.autorizeTopic(next, user.PermissionRead)
}

func (s *Server) autorizeTopic(next handleFunc, perm user.Permission) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, v *visitor) error {
		if s.userManager == nil {
			return next(w, r, v)
		}
		topics, _, err := s.topicsFromPath(r.URL.Path)
		if err != nil {
			return err
		}
		for _, t := range topics {
			if err := s.userManager.Authorize(v.user, t.ID, perm); err != nil {
				log.Info("unauthorized: %s", err.Error())
				return errHTTPForbidden
			}
		}
		return next(w, r, v)
	}
}

// visitor creates or retrieves a rate.Limiter for the given visitor.
// Note that this function will always return a visitor, even if an error occurs.
func (s *Server) visitor(r *http.Request) (v *visitor, err error) {
	ip := extractIPAddress(r, s.config.BehindProxy)
	var u *user.User // may stay nil if no auth header!
	if u, err = s.authenticate(r); err != nil {
		log.Debug("authentication failed: %s", err.Error())
		err = errHTTPUnauthorized // Always return visitor, even when error occurs!
	}
	if u != nil {
		v = s.visitorFromUser(u, ip)
	} else {
		v = s.visitorFromIP(ip)
	}
	v.mu.Lock()
	v.user = u
	v.mu.Unlock()
	return v, err // Always return visitor, even when error occurs!
}

// authenticate a user based on basic auth username/password (Authorization: Basic ...), or token auth (Authorization: Bearer ...).
// The Authorization header can be passed as a header or the ?auth=... query param. The latter is required only to
// support the WebSocket JavaScript class, which does not support passing headers during the initial request. The auth
// query param is effectively double base64 encoded. Its format is base64(Basic base64(user:pass)).
func (s *Server) authenticate(r *http.Request) (user *user.User, err error) {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	queryParam := readQueryParam(r, "authorization", "auth")
	if queryParam != "" {
		a, err := base64.RawURLEncoding.DecodeString(queryParam)
		if err != nil {
			return nil, err
		}
		value = strings.TrimSpace(string(a))
	}
	if value == "" {
		return nil, nil
	} else if s.userManager == nil {
		return nil, errHTTPUnauthorized
	}
	if strings.HasPrefix(value, "Bearer") {
		return s.authenticateBearerAuth(value)
	}
	return s.authenticateBasicAuth(r, value)
}

func (s *Server) authenticateBasicAuth(r *http.Request, value string) (user *user.User, err error) {
	r.Header.Set("Authorization", value)
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, errors.New("invalid basic auth")
	}
	return s.userManager.Authenticate(username, password)
}

func (s *Server) authenticateBearerAuth(value string) (user *user.User, err error) {
	token := strings.TrimSpace(strings.TrimPrefix(value, "Bearer"))
	return s.userManager.AuthenticateToken(token)
}

func (s *Server) visitorFromID(visitorID string, ip netip.Addr, user *user.User) *visitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, exists := s.visitors[visitorID]
	if !exists {
		s.visitors[visitorID] = newVisitor(s.config, s.messageCache, s.userManager, ip, user)
		return s.visitors[visitorID]
	}
	v.Keepalive()
	return v
}

func (s *Server) visitorFromIP(ip netip.Addr) *visitor {
	return s.visitorFromID(fmt.Sprintf("ip:%s", ip.String()), ip, nil)
}

func (s *Server) visitorFromUser(user *user.User, ip netip.Addr) *visitor {
	return s.visitorFromID(fmt.Sprintf("user:%s", user.Name), ip, user)
}
