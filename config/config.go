// Package config provides the main configuration
package config

import (
	"golang.org/x/time/rate"
	"time"
)

// Defines default config settings
const (
	DefaultListenHTTP            = ":80"
	DefaultMessageBufferDuration = 12 * time.Hour
	DefaultKeepaliveInterval     = 30 * time.Second
	DefaultManagerInterval       = time.Minute
)

// Defines all the limits
// - request limit: max number of PUT/GET/.. requests (here: 50 requests bucket, replenished at a rate of 1 per second)
// - global topic limit: max number of topics overall
// - subscription limit: max number of subscriptions (active HTTP connections) per per-visitor/IP
var (
	defaultGlobalTopicLimit         = 5000
	defaultVisitorRequestLimit      = rate.Every(time.Second)
	defaultVisitorRequestLimitBurst = 50
	defaultVisitorSubscriptionLimit = 30
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	ListenHTTP               string
	FirebaseKeyFile          string
	MessageBufferDuration    time.Duration
	KeepaliveInterval        time.Duration
	ManagerInterval          time.Duration
	GlobalTopicLimit         int
	VisitorRequestLimit      rate.Limit
	VisitorRequestLimitBurst int
	VisitorSubscriptionLimit int
}

// New instantiates a default new config
func New(listenHTTP string) *Config {
	return &Config{
		ListenHTTP:               listenHTTP,
		FirebaseKeyFile:          "",
		MessageBufferDuration:    DefaultMessageBufferDuration,
		KeepaliveInterval:        DefaultKeepaliveInterval,
		ManagerInterval:          DefaultManagerInterval,
		GlobalTopicLimit:         defaultGlobalTopicLimit,
		VisitorRequestLimit:      defaultVisitorRequestLimit,
		VisitorRequestLimitBurst: defaultVisitorRequestLimitBurst,
		VisitorSubscriptionLimit: defaultVisitorSubscriptionLimit,
	}
}
