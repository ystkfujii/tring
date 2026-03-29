package goproxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver"
)

const (
	defaultProxyURL = "https://proxy.golang.org"
	defaultTimeout  = 30 * time.Second
)

func init() {
	resolver.Register("goproxy", &Factory{})
}

// Factory creates goproxy resolvers.
type Factory struct{}

// Kind returns the resolver type.
func (f *Factory) Kind() string {
	return "goproxy"
}

// Create creates a new goproxy resolver from configuration map.
func (f *Factory) Create(config map[string]interface{}) (model.Resolver, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode goproxy config: %w", err)
	}

	opts := Options{
		ProxyURL: cfg.ProxyURL,
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

// Options configures the goproxy resolver.
type Options struct {
	// ProxyURL is the Go module proxy URL (defaults to proxy.golang.org)
	ProxyURL string
	// HTTPClient is the HTTP client to use (defaults to http.DefaultClient with timeout)
	HTTPClient *http.Client
	// Timeout is the request timeout (defaults to 30s)
	Timeout time.Duration
}

// Resolver fetches version candidates from a Go module proxy.
type Resolver struct {
	proxyURL string
	client   *http.Client
}

// Ensure Resolver implements model.Resolver
var _ model.Resolver = (*Resolver)(nil)

// New creates a new goproxy resolver.
func New(opts Options) *Resolver {
	proxyURL := opts.ProxyURL
	if proxyURL == "" {
		proxyURL = defaultProxyURL
	}

	client := opts.HTTPClient
	if client == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = defaultTimeout
		}
		client = &http.Client{Timeout: timeout}
	}

	return &Resolver{
		proxyURL: strings.TrimSuffix(proxyURL, "/"),
		client:   client,
	}
}

// Kind returns the resolver type.
func (r *Resolver) Kind() string {
	return "goproxy"
}

// Resolve fetches version candidates for the given dependency.
func (r *Resolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	escaped := escapePath(dep.Name)

	listURL := fmt.Sprintf("%s/%s/@v/list", r.proxyURL, escaped)
	versions, err := r.fetchVersionList(ctx, listURL)
	if err != nil {
		return model.Candidates{}, fmt.Errorf("failed to fetch version list for %s: %w", dep.Name, err)
	}

	if len(versions) == 0 {
		return model.Candidates{}, nil
	}

	// Extract repo URL for GitHub modules (best-effort for diff links)
	repoURL := extractGitHubRepoURL(dep.Name)

	var candidates []model.Candidate
	var infoFetchErrors int
	var lastInfoErr error

	for _, vStr := range versions {
		info, err := r.fetchVersionInfo(ctx, escaped, vStr)
		if err != nil {
			infoFetchErrors++
			lastInfoErr = err
			continue
		}

		v, err := semver.NewVersion(vStr)
		if err != nil {
			// Skip invalid semver versions (not counted as fetch error)
			continue
		}

		candidate := model.Candidate{
			Version:    v,
			ReleasedAt: info.Time,
		}
		if repoURL != "" {
			candidate.Metadata = map[string]string{
				"repo_url": repoURL,
			}
		}
		candidates = append(candidates, candidate)
	}

	// If more than half of the versions failed to fetch info, treat it as an error
	// This indicates a systemic problem with the proxy rather than just a few bad versions
	if infoFetchErrors > 0 && infoFetchErrors > len(versions)/2 {
		return model.Candidates{}, fmt.Errorf(
			"failed to fetch version info for %d/%d versions of %s: %w",
			infoFetchErrors, len(versions), dep.Name, lastInfoErr,
		)
	}

	return model.Candidates{Items: candidates}, nil
}

func (r *Resolver) fetchVersionList(ctx context.Context, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// Module not found, return empty list
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var versions []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			versions = append(versions, line)
		}
	}

	return versions, scanner.Err()
}

type versionInfo struct {
	Version string    `json:"Version"`
	Time    time.Time `json:"Time"`
}

func (r *Resolver) fetchVersionInfo(ctx context.Context, escapedPath, version string) (*versionInfo, error) {
	url := fmt.Sprintf("%s/%s/@v/%s.info", r.proxyURL, escapedPath, version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var info versionInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

// escapePath escapes a module path for use in proxy URLs.
// Uppercase letters are replaced with '!' followed by the lowercase letter.
func escapePath(path string) string {
	var b strings.Builder
	for _, r := range path {
		if 'A' <= r && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// extractGitHubRepoURL extracts the GitHub repository URL from a module path.
// It handles various Go module patterns:
//   - github.com/owner/repo -> https://github.com/owner/repo
//   - github.com/owner/repo/subdir -> https://github.com/owner/repo
//   - github.com/owner/repo/v2 -> https://github.com/owner/repo
//
// Returns empty string if not a GitHub module or cannot extract repo URL.
func extractGitHubRepoURL(modulePath string) string {
	path, found := strings.CutPrefix(modulePath, "github.com/")
	if !found || path == "" {
		return ""
	}

	// Split by "/" - we need owner/repo (first two parts)
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}

	return "https://github.com/" + parts[0] + "/" + parts[1]
}
