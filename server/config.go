package server

import (
	"heckel.io/ntfy/user"
	"io/fs"
	"net/netip"
	"time"
)

// Defines default config settings (excluding limits, see below)
const (
	DefaultListenHTTP                           = ":80"
	DefaultCacheDuration                        = 12 * time.Hour
	DefaultKeepaliveInterval                    = 45 * time.Second // Not too frequently to save battery (Android read timeout used to be 77s!)
	DefaultManagerInterval                      = time.Minute
	DefaultDelayedSenderInterval                = 10 * time.Second
	DefaultMinDelay                             = 10 * time.Second
	DefaultMaxDelay                             = 3 * 24 * time.Hour
	DefaultFirebaseKeepaliveInterval            = 3 * time.Hour    // ~control topic (Android), not too frequently to save battery
	DefaultFirebasePollInterval                 = 20 * time.Minute // ~poll topic (iOS), max. 2-3 times per hour (see docs)
	DefaultFirebaseQuotaExceededPenaltyDuration = 10 * time.Minute // Time that over-users are locked out of Firebase if it returns "quota exceeded"
	DefaultStripePriceCacheDuration             = 3 * time.Hour    // Time to keep Stripe prices cached in memory before a refresh is needed
)

// Defines all global and per-visitor limits
// - message size limit: the max number of bytes for a message
// - total topic limit: max number of topics overall
// - various attachment limits
const (
	DefaultMessageLengthLimit       = 4096 // Bytes
	DefaultTotalTopicLimit          = 15000
	DefaultAttachmentTotalSizeLimit = int64(5 * 1024 * 1024 * 1024) // 5 GB
	DefaultAttachmentFileSizeLimit  = int64(15 * 1024 * 1024)       // 15 MB
	DefaultAttachmentExpiryDuration = 3 * time.Hour
)

// Defines all per-visitor limits
// - per visitor subscription limit: max number of subscriptions (active HTTP connections) per per-visitor/IP
// - per visitor request limit: max number of PUT/GET/.. requests (here: 60 requests bucket, replenished at a rate of one per 5 seconds)
// - per visitor email limit: max number of emails (here: 16 email bucket, replenished at a rate of one per hour)
// - per visitor attachment size limit: total per-visitor attachment size in bytes to be stored on the server
// - per visitor attachment daily bandwidth limit: number of bytes that can be transferred to/from the server
const (
	DefaultVisitorSubscriptionLimit             = 30
	DefaultVisitorRequestLimitBurst             = 60
	DefaultVisitorRequestLimitReplenish         = 5 * time.Second
	DefaultVisitorMessageDailyLimit             = 0
	DefaultVisitorEmailLimitBurst               = 16
	DefaultVisitorEmailLimitReplenish           = time.Hour
	DefaultVisitorSMSDailyLimit                 = 10
	DefaultVisitorCallDailyLimit                = 10
	DefaultVisitorAccountCreationLimitBurst     = 3
	DefaultVisitorAccountCreationLimitReplenish = 24 * time.Hour
	DefaultVisitorAuthFailureLimitBurst         = 30
	DefaultVisitorAuthFailureLimitReplenish     = time.Minute
	DefaultVisitorAttachmentTotalSizeLimit      = 100 * 1024 * 1024 // 100 MB
	DefaultVisitorAttachmentDailyBandwidthLimit = 500 * 1024 * 1024 // 500 MB
)

var (
	// DefaultVisitorStatsResetTime defines the time at which visitor stats are reset (wall clock only)
	DefaultVisitorStatsResetTime = time.Date(0, 0, 0, 0, 0, 0, 0, time.UTC)

	// DefaultDisallowedTopics defines the topics that are forbidden, because they are used elsewhere. This array can be
	// extended using the server.yml config. If updated, also update in Android and web app.
	DefaultDisallowedTopics = []string{"docs", "static", "file", "app", "metrics", "account", "settings", "signup", "login", "v1"}
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	File                                 string // Config file, only used for testing
	BaseURL                              string
	ListenHTTP                           string
	ListenHTTPS                          string
	ListenUnix                           string
	ListenUnixMode                       fs.FileMode
	KeyFile                              string
	CertFile                             string
	FirebaseKeyFile                      string
	CacheFile                            string
	CacheDuration                        time.Duration
	CacheStartupQueries                  string
	CacheBatchSize                       int
	CacheBatchTimeout                    time.Duration
	AuthFile                             string
	AuthStartupQueries                   string
	AuthDefault                          user.Permission
	AuthBcryptCost                       int
	AuthStatsQueueWriterInterval         time.Duration
	AttachmentCacheDir                   string
	AttachmentTotalSizeLimit             int64
	AttachmentFileSizeLimit              int64
	AttachmentExpiryDuration             time.Duration
	KeepaliveInterval                    time.Duration
	ManagerInterval                      time.Duration
	DisallowedTopics                     []string
	WebRoot                              string // empty to disable
	DelayedSenderInterval                time.Duration
	FirebaseKeepaliveInterval            time.Duration
	FirebasePollInterval                 time.Duration
	FirebaseQuotaExceededPenaltyDuration time.Duration
	UpstreamBaseURL                      string
	SMTPSenderAddr                       string
	SMTPSenderUser                       string
	SMTPSenderPass                       string
	SMTPSenderFrom                       string
	SMTPServerListen                     string
	SMTPServerDomain                     string
	SMTPServerAddrPrefix                 string
	TwilioMessagingBaseURL               string
	TwilioAccount                        string
	TwilioAuthToken                      string
	TwilioFromNumber                     string
	TwilioVerifyBaseURL                  string
	TwilioVerifyService                  string
	MetricsEnable                        bool
	MetricsListenHTTP                    string
	ProfileListenHTTP                    string
	MessageLimit                         int
	MinDelay                             time.Duration
	MaxDelay                             time.Duration
	TotalTopicLimit                      int
	TotalAttachmentSizeLimit             int64
	VisitorSubscriptionLimit             int
	VisitorAttachmentTotalSizeLimit      int64
	VisitorAttachmentDailyBandwidthLimit int64
	VisitorRequestLimitBurst             int
	VisitorRequestLimitReplenish         time.Duration
	VisitorRequestExemptIPAddrs          []netip.Prefix
	VisitorMessageDailyLimit             int
	VisitorEmailLimitBurst               int
	VisitorEmailLimitReplenish           time.Duration
	VisitorSMSDailyLimit                 int
	VisitorCallDailyLimit                int
	VisitorAccountCreationLimitBurst     int
	VisitorAccountCreationLimitReplenish time.Duration
	VisitorAuthFailureLimitBurst         int
	VisitorAuthFailureLimitReplenish     time.Duration
	VisitorStatsResetTime                time.Time // Time of the day at which to reset visitor stats
	VisitorSubscriberRateLimiting        bool      // Enable subscriber-based rate limiting for UnifiedPush topics
	BehindProxy                          bool
	StripeSecretKey                      string
	StripeWebhookKey                     string
	StripePriceCacheDuration             time.Duration
	BillingContact                       string
	EnableSignup                         bool // Enable creation of accounts via API and UI
	EnableLogin                          bool
	EnableReservations                   bool // Allow users with role "user" to own/reserve topics
	EnableMetrics                        bool
	AccessControlAllowOrigin             string // CORS header field to restrict access from web clients
	Version                              string // injected by App
}

// NewConfig instantiates a default new server config
func NewConfig() *Config {
	return &Config{
		File:                                 "", // Only used for testing
		BaseURL:                              "",
		ListenHTTP:                           DefaultListenHTTP,
		ListenHTTPS:                          "",
		ListenUnix:                           "",
		ListenUnixMode:                       0,
		KeyFile:                              "",
		CertFile:                             "",
		FirebaseKeyFile:                      "",
		CacheFile:                            "",
		CacheDuration:                        DefaultCacheDuration,
		CacheStartupQueries:                  "",
		CacheBatchSize:                       0,
		CacheBatchTimeout:                    0,
		AuthFile:                             "",
		AuthStartupQueries:                   "",
		AuthDefault:                          user.PermissionReadWrite,
		AuthBcryptCost:                       user.DefaultUserPasswordBcryptCost,
		AuthStatsQueueWriterInterval:         user.DefaultUserStatsQueueWriterInterval,
		AttachmentCacheDir:                   "",
		AttachmentTotalSizeLimit:             DefaultAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:              DefaultAttachmentFileSizeLimit,
		AttachmentExpiryDuration:             DefaultAttachmentExpiryDuration,
		KeepaliveInterval:                    DefaultKeepaliveInterval,
		ManagerInterval:                      DefaultManagerInterval,
		DisallowedTopics:                     DefaultDisallowedTopics,
		WebRoot:                              "/",
		DelayedSenderInterval:                DefaultDelayedSenderInterval,
		FirebaseKeepaliveInterval:            DefaultFirebaseKeepaliveInterval,
		FirebasePollInterval:                 DefaultFirebasePollInterval,
		FirebaseQuotaExceededPenaltyDuration: DefaultFirebaseQuotaExceededPenaltyDuration,
		UpstreamBaseURL:                      "",
		SMTPSenderAddr:                       "",
		SMTPSenderUser:                       "",
		SMTPSenderPass:                       "",
		SMTPSenderFrom:                       "",
		SMTPServerListen:                     "",
		SMTPServerDomain:                     "",
		SMTPServerAddrPrefix:                 "",
		TwilioMessagingBaseURL:               "https://api.twilio.com", // Override for tests
		TwilioAccount:                        "",
		TwilioAuthToken:                      "",
		TwilioFromNumber:                     "",
		TwilioVerifyBaseURL:                  "https://verify.twilio.com", // Override for tests
		TwilioVerifyService:                  "",
		MessageLimit:                         DefaultMessageLengthLimit,
		MinDelay:                             DefaultMinDelay,
		MaxDelay:                             DefaultMaxDelay,
		TotalTopicLimit:                      DefaultTotalTopicLimit,
		TotalAttachmentSizeLimit:             0,
		VisitorSubscriptionLimit:             DefaultVisitorSubscriptionLimit,
		VisitorAttachmentTotalSizeLimit:      DefaultVisitorAttachmentTotalSizeLimit,
		VisitorAttachmentDailyBandwidthLimit: DefaultVisitorAttachmentDailyBandwidthLimit,
		VisitorRequestLimitBurst:             DefaultVisitorRequestLimitBurst,
		VisitorRequestLimitReplenish:         DefaultVisitorRequestLimitReplenish,
		VisitorRequestExemptIPAddrs:          make([]netip.Prefix, 0),
		VisitorMessageDailyLimit:             DefaultVisitorMessageDailyLimit,
		VisitorEmailLimitBurst:               DefaultVisitorEmailLimitBurst,
		VisitorEmailLimitReplenish:           DefaultVisitorEmailLimitReplenish,
		VisitorAccountCreationLimitBurst:     DefaultVisitorAccountCreationLimitBurst,
		VisitorAccountCreationLimitReplenish: DefaultVisitorAccountCreationLimitReplenish,
		VisitorAuthFailureLimitBurst:         DefaultVisitorAuthFailureLimitBurst,
		VisitorAuthFailureLimitReplenish:     DefaultVisitorAuthFailureLimitReplenish,
		VisitorStatsResetTime:                DefaultVisitorStatsResetTime,
		VisitorSubscriberRateLimiting:        false,
		BehindProxy:                          false,
		StripeSecretKey:                      "",
		StripeWebhookKey:                     "",
		StripePriceCacheDuration:             DefaultStripePriceCacheDuration,
		BillingContact:                       "",
		EnableSignup:                         false,
		EnableLogin:                          false,
		EnableReservations:                   false,
		AccessControlAllowOrigin:             "*",
		Version:                              "",
	}
}
