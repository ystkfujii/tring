package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load loads a configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from YAML bytes.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	return &cfg, nil
}

// LoadResolved loads, validates, and normalizes a configuration file.
func LoadResolved(path string) (*ResolvedConfig, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}

	validator := NewValidator()
	if errs := validator.Validate(cfg); !errs.IsEmpty() {
		return nil, errs
	}

	basePath, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	resolved, err := cfg.Resolve(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config: %w", err)
	}

	return resolved, nil
}
