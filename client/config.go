package client

const (
	// DefaultBaseURL is the base URL used to expand short topic names
	DefaultBaseURL = "https://ntfy.sh"
)

// Config is the config struct for a Client
type Config struct {
	DefaultHost string
	Subscribe   []struct {
		Topic   string
		Command string
		// If []map[string]string TODO This would be cool
	}
}

// NewConfig creates a new Config struct for a Client
func NewConfig() *Config {
	return &Config{
		DefaultHost: DefaultBaseURL,
		Subscribe:   nil,
	}
}
