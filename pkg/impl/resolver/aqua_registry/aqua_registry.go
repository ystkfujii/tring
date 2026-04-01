package aqua_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	aquahelper "github.com/ystkfujii/tring/pkg/impl/aqua"
	"github.com/ystkfujii/tring/pkg/impl/resolver"
)

const (
	defaultAPIURL          = "https://api.github.com"
	defaultRegistryBaseURL = "https://raw.githubusercontent.com/aquaproj/aqua-registry"
	defaultTimeout         = 30 * time.Second
)

func init() {
	resolver.Register(resolverKind, &Factory{})
}

// Factory creates aqua_registry resolvers.
type Factory struct{}

// Kind returns the resolver type.
func (f *Factory) Kind() string {
	return resolverKind
}

// Create creates a new aqua_registry resolver from configuration map.
func (f *Factory) Create(config map[string]interface{}) (model.Resolver, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode aqua_registry config: %w", err)
	}

	opts := Options{
		APIURL:          cfg.APIURL,
		RegistryBaseURL: cfg.RegistryBaseURL,
		TokenEnv:        cfg.GitHubTokenEnv,
	}
	if opts.TokenEnv == "" {
		opts.TokenEnv = "GITHUB_TOKEN"
	}
	opts.Token = os.Getenv(opts.TokenEnv)

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

// Options configures the aqua_registry resolver.
type Options struct {
	APIURL          string
	RegistryBaseURL string
	HTTPClient      *http.Client
	Timeout         time.Duration
	Token           string
	TokenEnv        string
}

// Resolver fetches version candidates from aqua registries.
type Resolver struct {
	apiURL          string
	registryBaseURL string
	client          *http.Client
	token           string
}

var _ model.Resolver = (*Resolver)(nil)

// New creates a new aqua_registry resolver.
func New(opts Options) *Resolver {
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	registryBaseURL := opts.RegistryBaseURL
	if registryBaseURL == "" {
		registryBaseURL = defaultRegistryBaseURL
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
		apiURL:          strings.TrimSuffix(apiURL, "/"),
		registryBaseURL: strings.TrimSuffix(registryBaseURL, "/"),
		client:          client,
		token:           opts.Token,
	}
}

// Kind returns the resolver type.
func (r *Resolver) Kind() string {
	return resolverKind
}

// Resolve fetches version candidates for the given aqua dependency.
func (r *Resolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	if dep.Metadata[aquahelper.MetadataObjectKind] == aquahelper.ObjectKindRegistry {
		return r.resolveStandardRegistryRef(ctx)
	}

	registryPackage, err := r.resolveRegistryPackage(ctx, dep)
	if err != nil {
		return model.Candidates{}, err
	}

	versionSource := registryPackage.VersionSource
	if versionSource == "" {
		versionSource = "github_release"
	}

	var rawCandidates []rawVersionCandidate
	switch versionSource {
	case "github_release":
		rawCandidates, err = r.fetchReleaseVersions(ctx, registryPackage.RepoOwner, registryPackage.RepoName)
	case "github_tag":
		rawCandidates, err = r.fetchTagVersions(ctx, registryPackage.RepoOwner, registryPackage.RepoName)
	default:
		return model.Candidates{}, fmt.Errorf("package %q uses unsupported version_source %q", dep.Name, versionSource)
	}
	if err != nil {
		return model.Candidates{}, err
	}

	return r.buildCandidates(dep, registryPackage, rawCandidates)
}

type rawVersionCandidate struct {
	RawVersion string
	ReleasedAt time.Time
}

func (r *Resolver) resolveStandardRegistryRef(ctx context.Context) (model.Candidates, error) {
	rawCandidates, err := r.fetchTagVersions(ctx, "aquaproj", "aqua-registry")
	if err != nil {
		return model.Candidates{}, fmt.Errorf("failed to fetch aqua-registry refs: %w", err)
	}

	var candidates []model.Candidate
	for _, raw := range rawCandidates {
		version, prefix, err := aquahelper.NormalizeVersion(raw.RawVersion)
		if err != nil {
			continue
		}
		candidates = append(candidates, model.Candidate{
			Version:    version,
			ReleasedAt: raw.ReleasedAt,
			Metadata: map[string]string{
				aquahelper.MetadataRawVersion:    raw.RawVersion,
				aquahelper.MetadataVersionPrefix: prefix,
				"repo_url":                       "https://github.com/aquaproj/aqua-registry",
			},
		})
	}

	return model.Candidates{Items: candidates}, nil
}

func (r *Resolver) resolveRegistryPackage(ctx context.Context, dep model.Dependency) (aquahelper.RegistryPackage, error) {
	var registry aquahelper.RegistryFile
	var err error

	switch dep.Metadata[aquahelper.MetadataRegistryType] {
	case aquahelper.RegistryTypeStandard:
		ref := dep.Metadata[aquahelper.MetadataStandardRef]
		if ref == "" {
			return aquahelper.RegistryPackage{}, fmt.Errorf("dependency %q is missing aqua standard registry ref", dep.Name)
		}
		registry, err = r.fetchStandardRegistry(ctx, ref)
		if err != nil {
			return aquahelper.RegistryPackage{}, err
		}
	case aquahelper.RegistryTypeLocal:
		path := dep.Metadata[aquahelper.MetadataLocalRegistryPath]
		if path == "" {
			return aquahelper.RegistryPackage{}, fmt.Errorf("dependency %q is missing aqua local registry path", dep.Name)
		}
		registry, err = r.fetchLocalRegistry(path)
		if err != nil {
			return aquahelper.RegistryPackage{}, err
		}
	default:
		return aquahelper.RegistryPackage{}, fmt.Errorf("dependency %q has unsupported aqua registry type %q", dep.Name, dep.Metadata[aquahelper.MetadataRegistryType])
	}
	if err != nil {
		return aquahelper.RegistryPackage{}, err
	}

	for _, pkg := range registry.Packages {
		if aquahelper.MatchesPackageName(pkg, dep.Name) {
			if pkg.RepoOwner == "" || pkg.RepoName == "" {
				return aquahelper.RegistryPackage{}, fmt.Errorf("package %q in registry is missing repo_owner/repo_name", dep.Name)
			}
			return pkg, nil
		}
	}

	return aquahelper.RegistryPackage{}, fmt.Errorf("package %q not found in aqua registry", dep.Name)
}

func (r *Resolver) fetchStandardRegistry(ctx context.Context, ref string) (aquahelper.RegistryFile, error) {
	url := fmt.Sprintf("%s/%s/registry.yaml", r.registryBaseURL, ref)
	body, err := r.fetch(ctx, url, false)
	if err != nil {
		return aquahelper.RegistryFile{}, fmt.Errorf("failed to fetch standard registry ref %q: %w", ref, err)
	}

	var registry aquahelper.RegistryFile
	if err := yaml.Unmarshal(body, &registry); err != nil {
		return aquahelper.RegistryFile{}, fmt.Errorf("failed to parse standard registry ref %q: %w", ref, err)
	}
	return registry, nil
}

func (r *Resolver) fetchLocalRegistry(path string) (aquahelper.RegistryFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return aquahelper.RegistryFile{}, fmt.Errorf("failed to read local registry %q: %w", path, err)
	}

	var registry aquahelper.RegistryFile
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return aquahelper.RegistryFile{}, fmt.Errorf("failed to parse local registry %q: %w", path, err)
	}
	return registry, nil
}

func (r *Resolver) buildCandidates(dep model.Dependency, pkg aquahelper.RegistryPackage, rawCandidates []rawVersionCandidate) (model.Candidates, error) {
	var candidates []model.Candidate

	for _, raw := range rawCandidates {
		allowed, err := allowsVersion(pkg.VersionFilter, raw.RawVersion)
		if err != nil {
			return model.Candidates{}, fmt.Errorf("package %q has unsupported version_filter: %w", dep.Name, err)
		}
		if !allowed {
			continue
		}

		version, prefix, err := aquahelper.NormalizeVersionWithPrefix(raw.RawVersion, pkg.VersionPrefix)
		if err != nil {
			continue
		}

		candidates = append(candidates, model.Candidate{
			Version:    version,
			ReleasedAt: raw.ReleasedAt,
			Metadata: map[string]string{
				aquahelper.MetadataRawVersion:    raw.RawVersion,
				aquahelper.MetadataVersionPrefix: prefix,
				"repo_url":                       fmt.Sprintf("https://github.com/%s/%s", pkg.RepoOwner, pkg.RepoName),
			},
		})
	}

	return model.Candidates{Items: candidates}, nil
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
	Draft       bool      `json:"draft"`
}

func (r *Resolver) fetchReleaseVersions(ctx context.Context, owner, repo string) ([]rawVersionCandidate, error) {
	var all []rawVersionCandidate
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d&page=%d", r.apiURL, owner, repo, perPage, page)
		body, err := r.fetch(ctx, url, true)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch releases for %s/%s: %w", owner, repo, err)
		}

		var releases []githubRelease
		if err := json.Unmarshal(body, &releases); err != nil {
			return nil, fmt.Errorf("failed to decode releases for %s/%s: %w", owner, repo, err)
		}
		if len(releases) == 0 {
			break
		}

		for _, release := range releases {
			if release.Draft || release.TagName == "" {
				continue
			}
			releasedAt := release.PublishedAt
			if releasedAt.IsZero() {
				releasedAt = release.CreatedAt
			}
			all = append(all, rawVersionCandidate{
				RawVersion: release.TagName,
				ReleasedAt: releasedAt,
			})
		}

		if len(releases) < perPage || page >= 50 {
			break
		}
		page++
	}

	return all, nil
}

type gitTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

type commitInfo struct {
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

func (r *Resolver) fetchTagVersions(ctx context.Context, owner, repo string) ([]rawVersionCandidate, error) {
	var all []rawVersionCandidate
	page := 1
	perPage := 100

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=%d&page=%d", r.apiURL, owner, repo, perPage, page)
		body, err := r.fetch(ctx, url, true)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tags for %s/%s: %w", owner, repo, err)
		}

		var tags []gitTag
		if err := json.Unmarshal(body, &tags); err != nil {
			return nil, fmt.Errorf("failed to decode tags for %s/%s: %w", owner, repo, err)
		}
		if len(tags) == 0 {
			break
		}

		for _, tag := range tags {
			releasedAt, err := r.fetchCommitDate(ctx, owner, repo, tag.Commit.SHA)
			if err != nil {
				releasedAt = time.Time{}
			}
			all = append(all, rawVersionCandidate{
				RawVersion: tag.Name,
				ReleasedAt: releasedAt,
			})
		}

		if len(tags) < perPage || page >= 50 {
			break
		}
		page++
	}

	return all, nil
}

func (r *Resolver) fetchCommitDate(ctx context.Context, owner, repo, sha string) (time.Time, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s", r.apiURL, owner, repo, sha)
	body, err := r.fetch(ctx, url, true)
	if err != nil {
		return time.Time{}, err
	}

	var info commitInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return time.Time{}, err
	}

	return info.Commit.Committer.Date, nil
}

func (r *Resolver) fetch(ctx context.Context, url string, addAuth bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if addAuth && r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodySnippet := strings.TrimSpace(string(body))
		if bodySnippet != "" {
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, bodySnippet)
		}
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

var (
	versionMatchesPattern    = regexp.MustCompile(`^Version matches "(.+)"$`)
	versionNotMatchesPattern = regexp.MustCompile(`^not\s*\(\s*Version matches "(.+)"\s*\)$`)
)

func allowsVersion(filter, version string) (bool, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true, nil
	}

	if matches := versionMatchesPattern.FindStringSubmatch(filter); len(matches) == 2 {
		re, err := regexp.Compile(matches[1])
		if err != nil {
			return false, err
		}
		return re.MatchString(version), nil
	}

	if matches := versionNotMatchesPattern.FindStringSubmatch(filter); len(matches) == 2 {
		re, err := regexp.Compile(matches[1])
		if err != nil {
			return false, err
		}
		return !re.MatchString(version), nil
	}

	return false, fmt.Errorf("unsupported expression %q", filter)
}
