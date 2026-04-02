package dockerfile

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

func init() {
	config.RegisterSourceConfigValidator(sourceKind, ValidateConfig)
}

// Config is the configuration for the dockerfile source.
type Config struct {
	// FilePaths is a list of Dockerfile paths to scan
	FilePaths []string `yaml:"file_paths"`
	// ImageMappings maps image names to dependency names
	ImageMappings []ImageMapping `yaml:"image_mappings"`
}

// ImageMapping defines how to map an image to a dependency name.
type ImageMapping struct {
	// Match is the image name to match (e.g., "golang", "docker.io/library/golang")
	Match string `yaml:"match"`
	// DependencyName is the dependency name to use (e.g., "go")
	DependencyName string `yaml:"dependency_name"`
	// VersionScheme is the version scheme (e.g., "semver")
	VersionScheme string `yaml:"version_scheme"`
}

// ValidateConfig validates dockerfile source configuration from a raw config map.
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
		return fmt.Errorf("failed to parse dockerfile config: %w", err)
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.FilePaths) == 0 {
		return fmt.Errorf("file_paths must be a non-empty list")
	}

	for i, m := range c.ImageMappings {
		if m.Match == "" {
			return fmt.Errorf("image_mappings[%d].match is required", i)
		}
		if m.DependencyName == "" {
			return fmt.Errorf("image_mappings[%d].dependency_name is required", i)
		}
	}

	return nil
}
