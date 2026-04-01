package goproxy

import (
	"fmt"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"
)

const Kind = "goproxy"

// Config is the configuration for the goproxy resolver.
type Config struct {
	// ProxyURL is the Go module proxy URL (defaults to proxy.golang.org)
	ProxyURL string `yaml:"proxy_url,omitempty"`
	// Timeout is the request timeout as a duration string (e.g., "30s")
	Timeout string `yaml:"timeout,omitempty"`
}

// DecodeConfig decodes a raw config map into a typed Config.
func DecodeConfig(raw map[string]interface{}) (Config, error) {
	var cfg Config
	if raw == nil {
		return cfg, nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return cfg, fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse goproxy config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig validates goproxy resolver configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if c.ProxyURL != "" {
		u, err := url.Parse(c.ProxyURL)
		if err != nil {
			return fmt.Errorf("invalid proxy_url: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("invalid proxy_url: scheme must be http or https, got %q", u.Scheme)
		}
		if u.Host == "" {
			return fmt.Errorf("invalid proxy_url: host is required")
		}
	}
	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}
	return nil
}
