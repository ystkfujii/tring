package aqua_registry

import (
	"fmt"
	"net/url"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

const resolverKind = "aqua_registry"

func init() {
	config.RegisterResolverConfigValidator(resolverKind, ValidateConfig)
}

// Config is the configuration for the aqua_registry resolver.
type Config struct {
	APIURL          string `yaml:"api_url,omitempty"`
	RegistryBaseURL string `yaml:"registry_base_url,omitempty"`
	GitHubTokenEnv  string `yaml:"github_token_env,omitempty"`
	Timeout         string `yaml:"timeout,omitempty"`
}

// ValidateConfig validates aqua_registry resolver configuration from a raw config map.
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
		return fmt.Errorf("failed to parse aqua_registry config: %w", err)
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if c.APIURL != "" {
		if _, err := url.Parse(c.APIURL); err != nil {
			return fmt.Errorf("invalid api_url: %w", err)
		}
	}
	if c.RegistryBaseURL != "" {
		if _, err := url.Parse(c.RegistryBaseURL); err != nil {
			return fmt.Errorf("invalid registry_base_url: %w", err)
		}
	}
	if c.Timeout != "" {
		if _, err := time.ParseDuration(c.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}
	return nil
}
