package envfile

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

func init() {
	config.RegisterSourceConfigValidator(sourceKind, ValidateConfig)
}

// Config is the configuration for the envfile source.
type Config struct {
	FilePaths []string   `yaml:"file_paths"`
	Variables []Variable `yaml:"variables"`
}

// Variable defines a variable to track.
type Variable struct {
	Name        string `yaml:"name"`
	ResolveWith string `yaml:"resolve_with"`
}

// ValidateConfig validates envfile source configuration from a raw config map.
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
		return fmt.Errorf("failed to parse envfile config: %w", err)
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.FilePaths) == 0 {
		return fmt.Errorf("file_paths must be a non-empty list")
	}

	if len(c.Variables) == 0 {
		return fmt.Errorf("variables must be a non-empty list")
	}

	for i, v := range c.Variables {
		if v.Name == "" {
			return fmt.Errorf("variables[%d].name is required", i)
		}
		if v.ResolveWith == "" {
			return fmt.Errorf("variables[%d].resolve_with is required", i)
		}
	}

	return nil
}
