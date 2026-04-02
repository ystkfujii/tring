package containerimage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultDockerHubURL = "https://registry.hub.docker.com"
)

// dockerHubResponse represents the Docker Hub API response for tags.
type dockerHubResponse struct {
	Results []dockerHubTag `json:"results"`
	Next    string         `json:"next"`
}

// dockerHubTag represents a single tag entry from Docker Hub.
type dockerHubTag struct {
	Name        string `json:"name"`
	LastUpdated string `json:"last_updated"`
}

// DockerHubProvider implements Provider for Docker Hub registry.
type DockerHubProvider struct {
	baseURL string
	client  *http.Client
}

// NewDockerHubProvider creates a new Docker Hub provider.
func NewDockerHubProvider(baseURL string, client *http.Client) *DockerHubProvider {
	if baseURL == "" {
		baseURL = defaultDockerHubURL
	}
	return &DockerHubProvider{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		client:  client,
	}
}

// ListTags fetches all tags for a repository from Docker Hub.
func (p *DockerHubProvider) ListTags(ctx context.Context, repository string) ([]TagInfo, error) {
	var allTags []TagInfo
	url := fmt.Sprintf("%s/v2/repositories/%s/tags?page_size=100", p.baseURL, repository)

	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("repository not found: %s", repository)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("docker hub api error: status %d", resp.StatusCode)
		}

		var response dockerHubResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		for _, tag := range response.Results {
			info := TagInfo{
				Name: tag.Name,
			}
			// Parse last_updated if present
			if tag.LastUpdated != "" {
				if t, err := time.Parse(time.RFC3339, tag.LastUpdated); err == nil {
					info.LastUpdated = t
				}
			}
			allTags = append(allTags, info)
		}

		// Follow pagination
		url = response.Next

		// Safety limit to avoid infinite loops
		if len(allTags) > 5000 {
			break
		}
	}

	return allTags, nil
}
