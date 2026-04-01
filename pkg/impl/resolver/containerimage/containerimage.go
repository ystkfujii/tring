package containerimage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver"
)

const (
	resolverKind   = "containerimage"
	defaultTimeout = 30 * time.Second
)

// Metadata keys used by dockerfile source
const (
	metadataRegistryHost = "registry_host"
	metadataRepository   = "repository"
)

// Known registry hosts
const (
	RegistryDockerHub    = "docker.io"
	RegistryDockerHubAlt = "registry-1.docker.io"
	RegistryDockerHubAPI = "registry.hub.docker.com"
	RegistryGHCR         = "ghcr.io"
)

func init() {
	resolver.Register(resolverKind, &Factory{})
}

// Factory creates containerimage resolvers.
type Factory struct{}

// Kind returns the resolver type.
func (f *Factory) Kind() string {
	return resolverKind
}

// Create creates a new containerimage resolver from configuration map.
func (f *Factory) Create(config map[string]interface{}) (model.Resolver, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode containerimage config: %w", err)
	}

	opts := Options{
		DockerHubURL: cfg.RegistryURL, // For backwards compatibility
		GHCRToken:    cfg.GHCRToken,
	}

	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		opts.Timeout = timeout
	}

	return New(opts), nil
}

func decodeConfig(raw map[string]interface{}, cfg *Config) error {
	if raw == nil {
		return nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// Options configures the containerimage resolver.
type Options struct {
	// DockerHubURL is the Docker Hub registry URL (defaults to https://registry.hub.docker.com)
	DockerHubURL string
	// GHCRURL is the GHCR registry URL (defaults to https://ghcr.io)
	GHCRURL string
	// GHCRToken is the optional GitHub token for private GHCR repositories
	GHCRToken string
	// HTTPClient is the HTTP client to use (defaults to http.DefaultClient with timeout)
	HTTPClient *http.Client
	// Timeout is the request timeout (defaults to 30s)
	Timeout time.Duration
}

// Resolver fetches container image tag candidates from various registries.
type Resolver struct {
	dockerHubProvider *DockerHubProvider
	ghcrProvider      *GHCRProvider
}

// Ensure Resolver implements model.Resolver
var _ model.Resolver = (*Resolver)(nil)

// New creates a new containerimage resolver.
func New(opts Options) *Resolver {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	return &Resolver{
		dockerHubProvider: NewDockerHubProvider(opts.DockerHubURL, client),
		ghcrProvider:      NewGHCRProvider(opts.GHCRURL, client, opts.GHCRToken),
	}
}

// Kind returns the resolver type.
func (r *Resolver) Kind() string {
	return resolverKind
}

// Resolve fetches version candidates for the given container image dependency.
// It determines the registry from Dependency.Metadata["registry_host"] and dispatches
// to the appropriate provider.
func (r *Resolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	registryHost := r.getRegistryHost(dep)
	repository := r.getRepository(dep)

	if repository == "" {
		return model.Candidates{}, fmt.Errorf("could not determine repository for dependency %q", dep.Name)
	}

	provider := r.getProvider(registryHost)
	if provider == nil {
		return model.Candidates{}, fmt.Errorf("unsupported registry: %s", registryHost)
	}

	tags, err := provider.ListTags(ctx, repository)
	if err != nil {
		return model.Candidates{}, fmt.Errorf("failed to fetch tags for %s: %w", repository, err)
	}

	var candidates []model.Candidate
	for _, tag := range tags {
		v, err := ParseTag(tag.Name)
		if err != nil {
			// Skip non-semver tags
			continue
		}

		candidates = append(candidates, model.Candidate{
			Version:    v,
			ReleasedAt: tag.LastUpdated,
			Metadata: map[string]string{
				"tag": tag.Name,
			},
		})
	}

	return model.Candidates{Items: candidates}, nil
}

// getRegistryHost determines the registry host for the dependency.
// Priority: Metadata["registry_host"] -> default to docker.io
func (r *Resolver) getRegistryHost(dep model.Dependency) string {
	if dep.Metadata != nil {
		if host, ok := dep.Metadata[metadataRegistryHost]; ok && host != "" {
			return host
		}
	}
	// Default to Docker Hub
	return RegistryDockerHub
}

// getRepository determines the container repository to query for tags.
// Priority: Metadata["repository"] -> Name with library/ prefix for Docker Hub official images
// IMPORTANT: Never infer repository from dep.Name (e.g., "go" should NOT become "golang")
func (r *Resolver) getRepository(dep model.Dependency) string {
	// Prefer metadata repository if available (set by dockerfile source)
	if dep.Metadata != nil {
		if repo, ok := dep.Metadata[metadataRepository]; ok && repo != "" {
			return repo
		}
	}

	// Fall back to dependency name
	// For official Docker Hub images (no slash), add library/ prefix
	name := dep.Name
	registryHost := r.getRegistryHost(dep)

	if isDockerHubRegistry(registryHost) && !strings.Contains(name, "/") {
		return "library/" + name
	}
	return name
}

// getProvider returns the appropriate provider for the given registry host.
func (r *Resolver) getProvider(registryHost string) Provider {
	switch {
	case isDockerHubRegistry(registryHost):
		return r.dockerHubProvider
	case registryHost == RegistryGHCR:
		return r.ghcrProvider
	default:
		return nil
	}
}

// isDockerHubRegistry checks if the registry host is Docker Hub.
func isDockerHubRegistry(host string) bool {
	switch host {
	case RegistryDockerHub, RegistryDockerHubAlt, RegistryDockerHubAPI, "":
		return true
	default:
		return false
	}
}
