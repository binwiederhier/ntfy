package client

const (
	DefaultBaseURL = "https://ntfy.sh"
)

type Config struct {
	DefaultHost string
	Subscribe   []struct {
		Topic   string
		Command string
	}
}

func NewConfig() *Config {
	return &Config{
		DefaultHost: DefaultBaseURL,
		Subscribe:   nil,
	}
}
