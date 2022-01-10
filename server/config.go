package server

import (
	"time"
)

// Defines default config settings
const (
	DefaultListenHTTP                = ":80"
	DefaultCacheDuration             = 12 * time.Hour
	DefaultKeepaliveInterval         = 55 * time.Second // Not too frequently to save battery (Android read timeout is 77s!)
	DefaultManagerInterval           = time.Minute
	DefaultAtSenderInterval          = 10 * time.Second
	DefaultMinDelay                  = 10 * time.Second
	DefaultMaxDelay                  = 3 * 24 * time.Hour
	DefaultMessageLimit              = 4096                      // Bytes
	DefaultAttachmentTotalSizeLimit  = int64(1024 * 1024 * 1024) // 1 GB
	DefaultAttachmentFileSizeLimit   = int64(15 * 1024 * 1024)   // 15 MB
	DefaultAttachmentExpiryDuration  = 3 * time.Hour
	DefaultFirebaseKeepaliveInterval = 3 * time.Hour // Not too frequently to save battery
)

// Defines all the limits
// - total topic limit: max number of topics overall
// - per visitor subscription limit: max number of subscriptions (active HTTP connections) per per-visitor/IP
// - per visitor request limit: max number of PUT/GET/.. requests (here: 60 requests bucket, replenished at a rate of one per 10 seconds)
// - per visitor email limit: max number of emails (here: 16 email bucket, replenished at a rate of one per hour)
// - per visitor attachment size limit:
const (
	DefaultTotalTopicLimit                 = 5000
	DefaultVisitorSubscriptionLimit        = 30
	DefaultVisitorRequestLimitBurst        = 60
	DefaultVisitorRequestLimitReplenish    = 10 * time.Second
	DefaultVisitorEmailLimitBurst          = 16
	DefaultVisitorEmailLimitReplenish      = time.Hour
	DefaultVisitorAttachmentTotalSizeLimit = 50 * 1024 * 1024
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	BaseURL                         string
	ListenHTTP                      string
	ListenHTTPS                     string
	KeyFile                         string
	CertFile                        string
	FirebaseKeyFile                 string
	CacheFile                       string
	CacheDuration                   time.Duration
	AttachmentCacheDir              string
	AttachmentTotalSizeLimit        int64
	AttachmentFileSizeLimit         int64
	AttachmentExpiryDuration        time.Duration
	KeepaliveInterval               time.Duration
	ManagerInterval                 time.Duration
	AtSenderInterval                time.Duration
	FirebaseKeepaliveInterval       time.Duration
	SMTPSenderAddr                  string
	SMTPSenderUser                  string
	SMTPSenderPass                  string
	SMTPSenderFrom                  string
	SMTPServerListen                string
	SMTPServerDomain                string
	SMTPServerAddrPrefix            string
	MessageLimit                    int
	MinDelay                        time.Duration
	MaxDelay                        time.Duration
	TotalTopicLimit                 int
	TotalAttachmentSizeLimit        int64
	VisitorSubscriptionLimit        int
	VisitorAttachmentTotalSizeLimit int64
	VisitorRequestLimitBurst        int
	VisitorRequestLimitReplenish    time.Duration
	VisitorEmailLimitBurst          int
	VisitorEmailLimitReplenish      time.Duration
	BehindProxy                     bool
}

// NewConfig instantiates a default new server config
func NewConfig() *Config {
	return &Config{
		BaseURL:                         "",
		ListenHTTP:                      DefaultListenHTTP,
		ListenHTTPS:                     "",
		KeyFile:                         "",
		CertFile:                        "",
		FirebaseKeyFile:                 "",
		CacheFile:                       "",
		CacheDuration:                   DefaultCacheDuration,
		AttachmentCacheDir:              "",
		AttachmentTotalSizeLimit:        DefaultAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:         DefaultAttachmentFileSizeLimit,
		AttachmentExpiryDuration:        DefaultAttachmentExpiryDuration,
		KeepaliveInterval:               DefaultKeepaliveInterval,
		ManagerInterval:                 DefaultManagerInterval,
		MessageLimit:                    DefaultMessageLimit,
		MinDelay:                        DefaultMinDelay,
		MaxDelay:                        DefaultMaxDelay,
		AtSenderInterval:                DefaultAtSenderInterval,
		FirebaseKeepaliveInterval:       DefaultFirebaseKeepaliveInterval,
		TotalTopicLimit:                 DefaultTotalTopicLimit,
		VisitorSubscriptionLimit:        DefaultVisitorSubscriptionLimit,
		VisitorAttachmentTotalSizeLimit: DefaultVisitorAttachmentTotalSizeLimit,
		VisitorRequestLimitBurst:        DefaultVisitorRequestLimitBurst,
		VisitorRequestLimitReplenish:    DefaultVisitorRequestLimitReplenish,
		VisitorEmailLimitBurst:          DefaultVisitorEmailLimitBurst,
		VisitorEmailLimitReplenish:      DefaultVisitorEmailLimitReplenish,
		BehindProxy:                     false,
	}
}
