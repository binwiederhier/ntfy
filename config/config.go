// Package config provides the main configuration
package config

const (
	DefaultListenHTTP = ":80"
)

// Config is the main config struct for the application. Use New to instantiate a default config struct.
type Config struct {
	ListenHTTP string
}

// New instantiates a default new config
func New(listenHTTP string) *Config {
	return &Config{
		ListenHTTP: listenHTTP,
	}
}
