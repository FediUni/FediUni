package config

import (
	"os"
)

// Config stores details for the current instance of FediUni to be used when
// creating actors, etc.
type Config struct {
	URL string
}

// New returns an instance of Config based on the provided flags.
func New() *Config {
	URL := os.Getenv("FEDIUNI_URL")
	return &Config{
		URL: URL,
	}
}
