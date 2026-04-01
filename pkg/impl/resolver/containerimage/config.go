package containerimage

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/config"
)

func init() {
	config.RegisterResolverConfigValidator(resolverKind, ValidateConfig)
}

// Config is the configuration for the containerimage resolver.
type Config struct {
	// RegistryURL is the Docker Hub registry URL (defaults to https://registry.hub.docker.com)
	RegistryURL string `yaml:"registry_url"`
	// GHCRToken is the optional GitHub token for private GHCR repositories
	GHCRToken string `yaml:"ghcr_token"`
	// Timeout is the HTTP request timeout (e.g., "30s")
	Timeout string `yaml:"timeout"`
}

// ValidateConfig validates containerimage resolver configuration from a raw config map.
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
		return fmt.Errorf("failed to parse containerimage config: %w", err)
	}
	return nil
}
