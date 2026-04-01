package githubrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver"
)

const (
	defaultAPIURL  = "https://api.github.com"
	defaultTimeout = 30 * time.Second
)

func init() {
	resolver.Register("githubrelease", &Factory{})
}

// Factory creates githubrelease resolvers.
type Factory struct{}

// Kind returns the resolver type.
func (f *Factory) Kind() string {
	return "githubrelease"
}

// Create creates a new githubrelease resolver from configuration map.
func (f *Factory) Create(config map[string]interface{}) (model.Resolver, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode githubrelease config: %w", err)
	}

	opts := Options{
		APIURL: cfg.APIURL,
		Token:  os.Getenv("GITHUB_TOKEN"),
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

// Options configures the githubrelease resolver.
type Options struct {
	// APIURL is the GitHub API URL (defaults to api.github.com)
	APIURL string
	// HTTPClient is the HTTP client to use (defaults to http.DefaultClient with timeout)
	HTTPClient *http.Client
	// Timeout is the request timeout (defaults to 30s)
	Timeout time.Duration
	// Token is an optional GitHub token for higher rate limits
	Token string
}

// Resolver fetches version candidates from GitHub releases/tags.
type Resolver struct {
	apiURL string
	client *http.Client
	token  string
}

// Ensure Resolver implements model.Resolver
var _ model.Resolver = (*Resolver)(nil)

// New creates a new githubrelease resolver.
func New(opts Options) *Resolver {
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
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
		apiURL: strings.TrimSuffix(apiURL, "/"),
		client: client,
		token:  opts.Token,
	}
}

// Kind returns the resolver type.
func (r *Resolver) Kind() string {
	return "githubrelease"
}

// Resolve fetches version candidates for the given GitHub Action dependency.
// The dependency name should be in the format "owner/repo".
func (r *Resolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	parts := strings.SplitN(dep.Name, "/", 2)
	if len(parts) != 2 {
		return model.Candidates{}, fmt.Errorf("invalid dependency name %q: expected owner/repo format", dep.Name)
	}
	owner, repo := parts[0], parts[1]

	tags, err := r.fetchTags(ctx, owner, repo)
	if err != nil {
		return model.Candidates{}, fmt.Errorf("failed to fetch tags for %s/%s: %w", owner, repo, err)
	}

	// Build repo URL for diff links
	repoURL := "https://github.com/" + owner + "/" + repo

	var candidates []model.Candidate
	for _, tag := range tags {
		v, err := semver.NewVersion(tag.Name)
		if err != nil {
			// Skip non-semver tags (e.g., "main", "v2", etc.)
			continue
		}

		sha := tag.Commit.SHA
		releasedAt := tag.Commit.Date
		if releasedAt.IsZero() {
			// If no commit date, try to fetch it
			commitDate, err := r.fetchCommitDate(ctx, owner, repo, sha)
			if err == nil {
				releasedAt = commitDate
			}
		}

		candidates = append(candidates, model.Candidate{
			Version:    v,
			ReleasedAt: releasedAt,
			Metadata: map[string]string{
				"commit_sha": sha,
				"tag":        tag.Name,
				"repo_url":   repoURL,
			},
		})
	}

	return model.Candidates{Items: candidates}, nil
}

// gitTag represents a GitHub tag from the API.
type gitTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA  string    `json:"sha"`
		URL  string    `json:"url"`
		Date time.Time `json:"-"` // Populated separately if needed
	} `json:"commit"`
}

// fetchTags fetches all tags for a repository.
func (r *Resolver) fetchTags(ctx context.Context, owner, repo string) ([]gitTag, error) {
	var allTags []gitTag
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=%d&page=%d",
			r.apiURL, owner, repo, perPage, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		r.addHeaders(req)

		resp, err := r.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			bodySnippet := strings.TrimSpace(string(body))
			if resp.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("repository not found: %s/%s", owner, repo)
			}
			if bodySnippet != "" {
				return nil, fmt.Errorf("GitHub API error: status %d: %s", resp.StatusCode, bodySnippet)
			}
			return nil, fmt.Errorf("GitHub API error: status %d", resp.StatusCode)
		}

		var tags []gitTag
		if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
			return nil, err
		}

		if len(tags) == 0 {
			break
		}

		allTags = append(allTags, tags...)

		// Stop if we got fewer than requested (last page)
		if len(tags) < perPage {
			break
		}

		page++

		// Safety limit to avoid infinite loops
		if page > 50 {
			break
		}
	}

	return allTags, nil
}

// commitInfo represents commit information from GitHub API.
type commitInfo struct {
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

// fetchCommitDate fetches the commit date for a given SHA.
func (r *Resolver) fetchCommitDate(ctx context.Context, owner, repo, sha string) (time.Time, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", r.apiURL, owner, repo, sha)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, err
	}

	r.addHeaders(req)

	resp, err := r.client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		bodySnippet := strings.TrimSpace(string(body))
		if bodySnippet != "" {
			return time.Time{}, fmt.Errorf("GitHub API error: status %d: %s", resp.StatusCode, bodySnippet)
		}
		return time.Time{}, fmt.Errorf("GitHub API error: status %d", resp.StatusCode)
	}

	var info commitInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return time.Time{}, err
	}

	return info.Commit.Committer.Date, nil
}

// addHeaders adds common headers to requests.
func (r *Resolver) addHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
}
