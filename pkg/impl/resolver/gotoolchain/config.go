package gotoolchain

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const Kind = "gotoolchain"

// Config is the configuration for the gotoolchain resolver.
type Config struct {
	// BaseURL is the base URL for Go downloads API (defaults to https://go.dev/dl/)
	BaseURL string `yaml:"base_url"`
	// Timeout is the HTTP request timeout (e.g., "30s")
	Timeout string `yaml:"timeout"`
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
		return cfg, fmt.Errorf("failed to parse gotoolchain config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig validates gotoolchain resolver configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	_, err := DecodeConfig(raw)
	return err
}
