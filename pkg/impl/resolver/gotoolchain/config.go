package gotoolchain

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

func init() {
	config.RegisterResolverConfigValidator(resolverKind, ValidateConfig)
}

// Config is the configuration for the gotoolchain resolver.
type Config struct {
	// BaseURL is the base URL for Go downloads API (defaults to https://go.dev/dl/)
	BaseURL string `yaml:"base_url"`
	// Timeout is the HTTP request timeout (e.g., "30s")
	Timeout string `yaml:"timeout"`
}

// ValidateConfig validates gotoolchain resolver configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	var cfg Config
	if raw == nil {
		return nil // All fields are optional
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("failed to parse gotoolchain config: %w", err)
	}
	return nil
}
