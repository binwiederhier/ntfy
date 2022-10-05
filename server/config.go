package server

import (
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
	DefaultVisitorEmailLimitBurst               = 16
	DefaultVisitorEmailLimitReplenish           = time.Hour
	DefaultVisitorAttachmentTotalSizeLimit      = 100 * 1024 * 1024 // 100 MB
	DefaultVisitorAttachmentDailyBandwidthLimit = 500 * 1024 * 1024 // 500 MB
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
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
	AuthFile                             string
	AuthDefaultRead                      bool
	AuthDefaultWrite                     bool
	AttachmentCacheDir                   string
	AttachmentTotalSizeLimit             int64
	AttachmentFileSizeLimit              int64
	AttachmentExpiryDuration             time.Duration
	KeepaliveInterval                    time.Duration
	ManagerInterval                      time.Duration
	WebRootIsApp                         bool
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
	MessageLimit                         int
	MinDelay                             time.Duration
	MaxDelay                             time.Duration
	TotalTopicLimit                      int
	TotalAttachmentSizeLimit             int64
	VisitorSubscriptionLimit             int
	VisitorAttachmentTotalSizeLimit      int64
	VisitorAttachmentDailyBandwidthLimit int
	VisitorRequestLimitBurst             int
	VisitorRequestLimitReplenish         time.Duration
	VisitorRequestExemptIPAddrs          []netip.Prefix
	VisitorEmailLimitBurst               int
	VisitorEmailLimitReplenish           time.Duration
	BehindProxy                          bool
	EnableWeb                            bool
	Version                              string // injected by App
}

// NewConfig instantiates a default new server config
func NewConfig() *Config {
	return &Config{
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
		AuthFile:                             "",
		AuthDefaultRead:                      true,
		AuthDefaultWrite:                     true,
		AttachmentCacheDir:                   "",
		AttachmentTotalSizeLimit:             DefaultAttachmentTotalSizeLimit,
		AttachmentFileSizeLimit:              DefaultAttachmentFileSizeLimit,
		AttachmentExpiryDuration:             DefaultAttachmentExpiryDuration,
		KeepaliveInterval:                    DefaultKeepaliveInterval,
		ManagerInterval:                      DefaultManagerInterval,
		MessageLimit:                         DefaultMessageLengthLimit,
		MinDelay:                             DefaultMinDelay,
		MaxDelay:                             DefaultMaxDelay,
		DelayedSenderInterval:                DefaultDelayedSenderInterval,
		FirebaseKeepaliveInterval:            DefaultFirebaseKeepaliveInterval,
		FirebasePollInterval:                 DefaultFirebasePollInterval,
		FirebaseQuotaExceededPenaltyDuration: DefaultFirebaseQuotaExceededPenaltyDuration,
		TotalTopicLimit:                      DefaultTotalTopicLimit,
		VisitorSubscriptionLimit:             DefaultVisitorSubscriptionLimit,
		VisitorAttachmentTotalSizeLimit:      DefaultVisitorAttachmentTotalSizeLimit,
		VisitorAttachmentDailyBandwidthLimit: DefaultVisitorAttachmentDailyBandwidthLimit,
		VisitorRequestLimitBurst:             DefaultVisitorRequestLimitBurst,
		VisitorRequestLimitReplenish:         DefaultVisitorRequestLimitReplenish,
		VisitorRequestExemptIPAddrs:          make([]netip.Prefix, 0),
		VisitorEmailLimitBurst:               DefaultVisitorEmailLimitBurst,
		VisitorEmailLimitReplenish:           DefaultVisitorEmailLimitReplenish,
		BehindProxy:                          false,
		EnableWeb:                            true,
		Version:                              "",
	}
}
