package aqua

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
	aquahelper "github.com/ystkfujii/tring/pkg/impl/aqua"
)

func init() {
	config.RegisterSourceConfigValidator(sourceKind, ValidateConfig)
}

// Config is the configuration for the aqua source.
type Config struct {
	FilePaths          []string `yaml:"file_paths"`
	Targets            []string `yaml:"targets"`
	UnsupportedVersion string   `yaml:"unsupported_version,omitempty"`
}

// ValidateConfig validates aqua source configuration from a raw config map.
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
		return fmt.Errorf("failed to parse aqua config: %w", err)
	}
	return cfg.validate()
}

func (c *Config) validate() error {
	if len(c.FilePaths) == 0 {
		return fmt.Errorf("file_paths must be a non-empty list")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("targets must be a non-empty list")
	}

	seen := make(map[string]struct{}, len(c.Targets))
	for i, target := range c.Targets {
		switch target {
		case aquahelper.TargetPackages, aquahelper.TargetRegistries:
		default:
			return fmt.Errorf("targets[%d] must be one of %q or %q", i, aquahelper.TargetPackages, aquahelper.TargetRegistries)
		}
		if _, ok := seen[target]; ok {
			return fmt.Errorf("targets[%d] duplicates %q", i, target)
		}
		seen[target] = struct{}{}
	}

	mode := c.UnsupportedVersion
	if mode == "" {
		mode = aquahelper.UnsupportedVersionSkip
	}
	switch mode {
	case aquahelper.UnsupportedVersionSkip, aquahelper.UnsupportedVersionError:
	default:
		return fmt.Errorf("unsupported_version must be %q or %q", aquahelper.UnsupportedVersionSkip, aquahelper.UnsupportedVersionError)
	}

	return nil
}
