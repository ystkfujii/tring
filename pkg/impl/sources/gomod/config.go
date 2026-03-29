package gomod

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

func init() {
	config.RegisterSourceConfigValidator(sourceKind, ValidateConfig)
}

// Config is the configuration for the gomod source.
type Config struct {
	ManifestPaths []string `yaml:"manifest_paths"`
}

// ValidateConfig validates gomod source configuration from a raw config map.
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
		return fmt.Errorf("failed to parse gomod config: %w", err)
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.ManifestPaths) == 0 {
		return fmt.Errorf("manifest_paths must be a non-empty list")
	}
	return nil
}
