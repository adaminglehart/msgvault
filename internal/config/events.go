package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// EventsConfig selects an optional delivery transport for archive events.
type EventsConfig struct {
	Enabled   bool       `toml:"enabled"`
	Transport string     `toml:"transport"`
	Timeout   string     `toml:"timeout"`
	NATS      NATSConfig `toml:"nats"`
}

type NATSConfig struct {
	URL             string `toml:"url"`
	SubjectPrefix   string `toml:"subject_prefix"`
	CredentialsFile string `toml:"credentials_file"`
}

func (e *EventsConfig) ApplyDefaults() {
	if e.Transport == "" {
		e.Transport = "nats"
	}
	if e.Timeout == "" {
		e.Timeout = "2s"
	}
	if e.NATS.SubjectPrefix == "" {
		e.NATS.SubjectPrefix = "msgvault"
	}
}

func (e EventsConfig) Validate() error {
	if !e.Enabled {
		return nil
	}
	if e.Transport != "nats" {
		return fmt.Errorf("events.transport: unsupported transport %q", e.Transport)
	}
	timeout, err := time.ParseDuration(e.Timeout)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("events.timeout: must be a positive duration")
	}
	u, err := url.Parse(e.NATS.URL)
	if err != nil || u.Host == "" || (u.Scheme != "nats" && u.Scheme != "tls") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("events.nats.url: use one nats:// or tls:// URL without credentials, query, or fragment")
	}
	for _, token := range strings.Split(e.NATS.SubjectPrefix, ".") {
		if token == "" || strings.IndexFunc(token, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsControl(r) || r == '*' || r == '>'
		}) >= 0 {
			return fmt.Errorf("events.nats.subject_prefix: must contain literal, nonempty subject tokens")
		}
	}
	return nil
}
