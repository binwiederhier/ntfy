// Package config provides the main configuration
package config

import (
	"time"
)

// Defines default config settings
const (
	DefaultListenHTTP        = ":80"
	DefaultCacheDuration     = 12 * time.Hour
	DefaultKeepaliveInterval = 30 * time.Second
	DefaultManagerInterval   = time.Minute
)

// Defines all the limits
// - global topic limit: max number of topics overall
// - per visistor request limit: max number of PUT/GET/.. requests (here: 60 requests bucket, replenished at a rate of one per 10 seconds)
// - per visistor subscription limit: max number of subscriptions (active HTTP connections) per per-visitor/IP
const (
	DefaultGlobalTopicLimit             = 5000
	DefaultVisitorRequestLimitBurst     = 60
	DefaultVisitorRequestLimitReplenish = 10 * time.Second
	DefaultVisitorSubscriptionLimit     = 30
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	ListenHTTP                   string
	ListenHTTPS                  string
	KeyFile                      string
	CertFile                     string
	FirebaseKeyFile              string
	CacheFile                    string
	CacheDuration                time.Duration
	KeepaliveInterval            time.Duration
	ManagerInterval              time.Duration
	GlobalTopicLimit             int
	VisitorRequestLimitBurst     int
	VisitorRequestLimitReplenish time.Duration
	VisitorSubscriptionLimit     int
	BehindProxy                  bool
}

// New instantiates a default new config
func New(listenHTTP string) *Config {
	return &Config{
		ListenHTTP:                   listenHTTP,
		ListenHTTPS:                  "",
		KeyFile:                      "",
		CertFile:                     "",
		FirebaseKeyFile:              "",
		CacheFile:                    "",
		CacheDuration:                DefaultCacheDuration,
		KeepaliveInterval:            DefaultKeepaliveInterval,
		ManagerInterval:              DefaultManagerInterval,
		GlobalTopicLimit:             DefaultGlobalTopicLimit,
		VisitorRequestLimitBurst:     DefaultVisitorRequestLimitBurst,
		VisitorRequestLimitReplenish: DefaultVisitorRequestLimitReplenish,
		VisitorSubscriptionLimit:     DefaultVisitorSubscriptionLimit,
		BehindProxy:                  false,
	}
}
