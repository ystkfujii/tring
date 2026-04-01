package githubaction

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

const Kind = "githubaction"

// Config is the configuration for the githubaction source.
type Config struct {
	FilePaths []string `yaml:"file_paths"`
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
		return cfg, fmt.Errorf("failed to parse githubaction config: %w", err)
	}
	return cfg, nil
}

// ValidateConfig validates githubaction source configuration from a raw config map.
func ValidateConfig(raw map[string]interface{}) error {
	cfg, err := DecodeConfig(raw)
	if err != nil {
		return err
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.FilePaths) == 0 {
		return fmt.Errorf("file_paths must be a non-empty list")
	}
	return nil
}
