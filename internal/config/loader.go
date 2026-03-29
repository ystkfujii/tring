package config

import (
	"fmt"
	"os"

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

// FindGroup finds a group by name in the configuration.
func (c *Config) FindGroup(name string) (*Group, error) {
	for i := range c.Groups {
		if c.Groups[i].Name == name {
			return &c.Groups[i], nil
		}
	}
	return nil, fmt.Errorf("group not found: %q", name)
}

// GroupNames returns a list of all group names.
func (c *Config) GroupNames() []string {
	names := make([]string, len(c.Groups))
	for i, g := range c.Groups {
		names[i] = g.Name
	}
	return names
}
