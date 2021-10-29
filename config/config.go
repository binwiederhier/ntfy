// Package config provides the main configuration
package config

import (
	"golang.org/x/time/rate"
	"time"
)

// Defines default config settings
const (
	DefaultListenHTTP        = ":80"
	DefaultKeepaliveInterval = 30 * time.Second
	defaultManagerInterval   = time.Minute
)

// Defines the max number of requests, here:
// 50 requests bucket, replenished at a rate of 1 per second
var (
	defaultLimit      = rate.Every(time.Second)
	defaultLimitBurst = 50
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	ListenHTTP        string
	Limit             rate.Limit
	LimitBurst        int
	FirebaseKeyFile   string
	KeepaliveInterval time.Duration
	ManagerInterval   time.Duration
}

// New instantiates a default new config
func New(listenHTTP string) *Config {
	return &Config{
		ListenHTTP:        listenHTTP,
		Limit:             defaultLimit,
		LimitBurst:        defaultLimitBurst,
		FirebaseKeyFile:   "",
		KeepaliveInterval: DefaultKeepaliveInterval,
		ManagerInterval:   defaultManagerInterval,
	}
}
