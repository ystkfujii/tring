package githubrelease

import (
	"fmt"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

const resolverKind = "githubrelease"

func init() {
	config.RegisterResolverConfigValidator(resolverKind, ValidateConfig)
}

// Config is the configuration for the githubrelease resolver.
type Config struct {
	// APIURL is the GitHub API URL (defaults to api.github.com)
	APIURL string `yaml:"api_url,omitempty"`
	// Timeout is the request timeout as a duration string (e.g., "30s")
	Timeout string `yaml:"timeout,omitempty"`
	// Token is an optional GitHub token for higher rate limits
	Token string `yaml:"token,omitempty"`
}

// ValidateConfig validates githubrelease resolver configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	var cfg Config
	if raw == nil {
		return cfg.validate()
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse githubrelease config: %w", err)
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
