package gomod

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const Kind = "gomod"

// Config is the configuration for the gomod source.
type Config struct {
	ManifestPaths  []string `yaml:"manifest_paths"`
	IncludeRequire *bool    `yaml:"include_require"`
	TrackGoVersion bool     `yaml:"track_go_version"`
	TrackToolchain bool     `yaml:"track_toolchain"`
}

// ShouldIncludeRequire returns whether require dependencies should be extracted.
// Defaults to true if not explicitly set.
func (c *Config) ShouldIncludeRequire() bool {
	if c.IncludeRequire == nil {
		return true
	}
	return *c.IncludeRequire
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
		return cfg, fmt.Errorf("failed to parse gomod config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig validates gomod source configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.ManifestPaths) == 0 {
		return fmt.Errorf("manifest_paths must be a non-empty list")
	}
	return nil
}
