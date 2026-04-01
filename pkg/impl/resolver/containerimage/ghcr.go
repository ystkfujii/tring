package containerimage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	defaultGHCRURL = "https://ghcr.io"
)

// ghcrTagsResponse represents the GHCR API response for tags.
type ghcrTagsResponse struct {
	Tags []string `json:"tags"`
}

// GHCRProvider implements Provider for GitHub Container Registry.
type GHCRProvider struct {
	baseURL string
	client  *http.Client
	token   string
}

// NewGHCRProvider creates a new GHCR provider.
func NewGHCRProvider(baseURL string, client *http.Client, token string) *GHCRProvider {
	if baseURL == "" {
		baseURL = defaultGHCRURL
	}
	return &GHCRProvider{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  client,
		token:   token,
	}
}

// ListTags fetches all tags for a repository from GHCR.
// GHCR uses the OCI Distribution API which doesn't provide timestamps.
func (p *GHCRProvider) ListTags(ctx context.Context, repository string) ([]TagInfo, error) {
	// First, get an anonymous token if no token is provided
	token := p.token
	if token == "" {
		var err error
		token, err = p.getAnonymousToken(ctx, repository)
		if err != nil {
			return nil, fmt.Errorf("failed to get anonymous token: %w", err)
		}
	}

	url := fmt.Sprintf("%s/v2/%s/tags/list", p.baseURL, repository)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repository not found: %s", repository)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("unauthorized: repository %s may be private or require authentication", repository)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GHCR API error: status %d", resp.StatusCode)
	}

	var response ghcrTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var tags []TagInfo
	for _, tag := range response.Tags {
		tags = append(tags, TagInfo{
			Name: tag,
			// GHCR OCI API doesn't provide timestamps
		})
	}

	return tags, nil
}

// getAnonymousToken retrieves an anonymous token for public repository access.
func (p *GHCRProvider) getAnonymousToken(ctx context.Context, repository string) (string, error) {
	// GHCR uses token-based auth even for public repos
	// First request to /v2/ will return WWW-Authenticate header with token URL
	url := fmt.Sprintf("https://ghcr.io/token?scope=repository:%s:pull", repository)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token: status %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.Token, nil
}
