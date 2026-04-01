package gotoolchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

const (
	defaultBaseURL = "https://go.dev/dl/"
	defaultTimeout = 30 * time.Second
)

// NewResolver creates a gotoolchain resolver from a raw configuration map.
func NewResolver(rawConfig map[string]interface{}) (*Resolver, error) {
	cfg, err := DecodeConfig(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode gotoolchain config: %w", err)
	}

	opts := Options{
		BaseURL: cfg.BaseURL,
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

// Options configures the gotoolchain resolver.
type Options struct {
	// BaseURL is the base URL for Go downloads API (defaults to https://go.dev/dl/)
	BaseURL string
	// HTTPClient is the HTTP client to use (defaults to http.DefaultClient with timeout)
	HTTPClient *http.Client
	// Timeout is the request timeout (defaults to 30s)
	Timeout time.Duration
}

// Resolver fetches Go toolchain version candidates from go.dev.
type Resolver struct {
	baseURL string
	client  *http.Client
}

// Ensure Resolver implements model.Resolver
var _ model.Resolver = (*Resolver)(nil)

// New creates a new gotoolchain resolver.
func New(opts Options) *Resolver {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
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
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  client,
	}
}

// Kind returns the resolver type.
func (r *Resolver) Kind() string {
	return Kind
}

// goRelease represents a Go release from the downloads API.
type goRelease struct {
	Version string   `json:"version"`
	Stable  bool     `json:"stable"`
	Files   []goFile `json:"files"`
}

// goFile represents a file in a Go release.
type goFile struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind"`
}

// Resolve fetches version candidates for Go toolchain.
// The dependency name is ignored - this resolver always returns all Go versions.
func (r *Resolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	releases, err := r.fetchReleases(ctx)
	if err != nil {
		return model.Candidates{}, fmt.Errorf("failed to fetch Go releases: %w", err)
	}

	var candidates []model.Candidate
	for _, rel := range releases {
		v, err := parseGoVersion(rel.Version)
		if err != nil {
			// Skip versions that can't be parsed as semver
			continue
		}

		candidate := model.Candidate{
			Version: v,
			// go.dev API doesn't provide release timestamps,
			// so we leave ReleasedAt as zero value
			ReleasedAt: time.Time{},
		}
		candidates = append(candidates, candidate)
	}

	return model.Candidates{Items: candidates}, nil
}

func (r *Resolver) fetchReleases(ctx context.Context) ([]goRelease, error) {
	url := r.baseURL + "/?mode=json&include=all"

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

	var releases []goRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return releases, nil
}

// parseGoVersion converts a Go version string (e.g., "go1.22.0") to semver.
// Uses ParseGoVersion which handles rc/beta prereleases.
func parseGoVersion(version string) (*semver.Version, error) {
	return ParseGoVersion(version)
}
