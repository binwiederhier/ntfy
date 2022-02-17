package client

import (
	"gopkg.in/yaml.v2"
	"os"
)

const (
	// DefaultBaseURL is the base URL used to expand short topic names
	DefaultBaseURL = "https://ntfy.sh"
)

// Config is the config struct for a Client
type Config struct {
	DefaultHost string `yaml:"default-host"`
	Subscribe   []struct {
		Topic    string            `yaml:"topic"`
		User     string            `yaml:"user"`
		Password string            `yaml:"password"`
		Command  string            `yaml:"command"`
		If       map[string]string `yaml:"if"`
	} `yaml:"subscribe"`
}

// NewConfig creates a new Config struct for a Client
func NewConfig() *Config {
	return &Config{
		DefaultHost: DefaultBaseURL,
		Subscribe:   nil,
	}
}

// LoadConfig loads the Client config from a yaml file
func LoadConfig(filename string) (*Config, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	c := NewConfig()
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}
	return c, nil
}
