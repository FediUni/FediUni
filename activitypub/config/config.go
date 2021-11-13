package config

import "flag"

var (
	URL = flag.String("fediuni_url", "", "This is the URL for this FediUni instance.")
)

// Config stores details for the current instance of FediUni to be used when
// creating actors, etc.
type Config struct {
	URL string
}

// New returns an instance of Config based on the provided flags.
func New() *Config {
	return &Config{
		URL: *URL,
	}
}
