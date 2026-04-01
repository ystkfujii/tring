package githubrelease

import (
	"fmt"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"
)

const Kind = "githubrelease"

// Config is the configuration for the githubrelease resolver.
type Config struct {
	// APIURL is the GitHub API URL (defaults to api.github.com)
	APIURL string `yaml:"api_url,omitempty"`
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
		return cfg, fmt.Errorf("failed to parse githubrelease config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig validates githubrelease resolver configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if c.APIURL != "" {
		if _, err := url.Parse(c.APIURL); err != nil {
			return fmt.Errorf("invalid api_url: %w", err)
		}
	}
	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}
	return nil
}
