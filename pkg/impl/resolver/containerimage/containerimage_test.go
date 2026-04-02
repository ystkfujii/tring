package containerimage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

func TestResolve_DockerHub_BasicTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/repositories/library/debian/tags" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "12.10", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "12.9", LastUpdated: "2024-02-01T00:00:00Z"},
				{Name: "12", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "11.10", LastUpdated: "2024-03-10T00:00:00Z"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	dep := model.Dependency{
		Name:    "debian",
		Version: semver.MustParse("12.9.0"),
		Metadata: map[string]string{
			"repository":    "library/debian",
			"registry_host": "docker.io",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(candidates.Items) != 4 {
		t.Fatalf("expected 4 candidates, got %d", len(candidates.Items))
	}

	// Check versions are properly parsed
	foundVersions := make(map[string]bool)
	for _, c := range candidates.Items {
		foundVersions[c.Version.String()] = true
	}

	expected := []string{"12.10.0", "12.9.0", "12.0.0", "11.10.0"}
	for _, v := range expected {
		if !foundVersions[v] {
			t.Errorf("missing expected version: %s", v)
		}
	}
}

func TestResolve_DockerHub_FilterNonSemverTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "1.24.1", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "1.24-alpine", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "bookworm", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "latest", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "1.24", LastUpdated: "2024-03-10T00:00:00Z"},
				{Name: "1", LastUpdated: "2024-03-01T00:00:00Z"},
				{Name: "v1.25.0", LastUpdated: "2024-03-20T00:00:00Z"},
				{Name: "1.24.1-bookworm-slim", LastUpdated: "2024-03-15T00:00:00Z"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	// No suffix in metadata -> only match tags without suffix
	dep := model.Dependency{
		Name: "golang",
		Metadata: map[string]string{
			"repository":    "library/golang",
			"registry_host": "docker.io",
			"tag_suffix":    "", // explicitly no suffix
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Only semver tags without suffix: 1.24.1, 1.24, 1, v1.25.0
	if len(candidates.Items) != 4 {
		t.Fatalf("expected 4 semver candidates (no suffix), got %d", len(candidates.Items))
	}

	foundVersions := make(map[string]bool)
	for _, c := range candidates.Items {
		foundVersions[c.Version.String()] = true
	}

	expected := []string{"1.24.1", "1.24.0", "1.0.0", "1.25.0"}
	for _, v := range expected {
		if !foundVersions[v] {
			t.Errorf("missing expected version: %s", v)
		}
	}
}

func TestResolve_DockerHub_FilterBySuffix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "1.24-alpine", LastUpdated: "2024-03-10T00:00:00Z"},
				{Name: "1.25-alpine", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "1.25-bookworm", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "1.25", LastUpdated: "2024-03-15T00:00:00Z"},
				{Name: "latest", LastUpdated: "2024-03-15T00:00:00Z"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	// Current tag has suffix "alpine"
	dep := model.Dependency{
		Name: "golang",
		Metadata: map[string]string{
			"repository":    "library/golang",
			"registry_host": "docker.io",
			"tag_suffix":    "alpine",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Only alpine suffix tags: 1.24-alpine, 1.25-alpine
	if len(candidates.Items) != 2 {
		t.Fatalf("expected 2 candidates with alpine suffix, got %d", len(candidates.Items))
	}

	for _, c := range candidates.Items {
		if c.Metadata["tag_suffix"] != "alpine" {
			t.Errorf("expected tag_suffix='alpine', got %q", c.Metadata["tag_suffix"])
		}
		// Check that raw tag ends with "-alpine"
		tag := c.Metadata["tag"]
		if len(tag) < 7 || tag[len(tag)-6:] != "alpine" {
			t.Errorf("expected tag to end with 'alpine', got %q", tag)
		}
	}
}

func TestResolve_UsesMetadataRepository(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the metadata repository is used
		if r.URL.Path != "/v2/repositories/library/golang/tags" {
			t.Errorf("expected repository 'library/golang', got path: %s", r.URL.Path)
		}

		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "1.24.1", LastUpdated: time.Now().Format(time.RFC3339)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	// Dependency.Name is "go" but metadata has the actual repository
	dep := model.Dependency{
		Name: "go",
		Metadata: map[string]string{
			"repository":    "library/golang",
			"registry_host": "docker.io",
		},
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
}

func TestResolve_RepositoryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	dep := model.Dependency{
		Name: "nonexistent",
		Metadata: map[string]string{
			"repository":    "library/nonexistent",
			"registry_host": "docker.io",
		},
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err == nil {
		t.Fatal("expected error for non-existent repository")
	}
}

func TestResolve_IncludesReleasedAt(t *testing.T) {
	expectedTime := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "1.0.0", LastUpdated: expectedTime.Format(time.RFC3339)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	dep := model.Dependency{
		Name: "test",
		Metadata: map[string]string{
			"repository":    "library/test",
			"registry_host": "docker.io",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(candidates.Items) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates.Items))
	}

	if !candidates.Items[0].ReleasedAt.Equal(expectedTime) {
		t.Errorf("expected ReleasedAt=%v, got %v", expectedTime, candidates.Items[0].ReleasedAt)
	}
}

func TestResolve_IncludesTagMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := dockerHubResponse{
			Results: []dockerHubTag{
				{Name: "v1.24.1", LastUpdated: time.Now().Format(time.RFC3339)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		DockerHubURL: server.URL,
	})

	dep := model.Dependency{
		Name: "test",
		Metadata: map[string]string{
			"repository":    "library/test",
			"registry_host": "docker.io",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if len(candidates.Items) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates.Items))
	}

	if candidates.Items[0].Metadata["tag"] != "v1.24.1" {
		t.Errorf("expected tag metadata='v1.24.1', got %q", candidates.Items[0].Metadata["tag"])
	}
}

func TestResolve_UnsupportedRegistry(t *testing.T) {
	resolver := New(Options{})

	dep := model.Dependency{
		Name: "myimage",
		Metadata: map[string]string{
			"repository":    "myuser/myimage",
			"registry_host": "quay.io",
		},
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err == nil {
		t.Fatal("expected error for unsupported registry")
	}
}

func TestGetRegistryHost(t *testing.T) {
	resolver := New(Options{})

	tests := []struct {
		name     string
		dep      model.Dependency
		expected string
	}{
		{
			name: "uses metadata registry_host",
			dep: model.Dependency{
				Name: "myimage",
				Metadata: map[string]string{
					"registry_host": "ghcr.io",
				},
			},
			expected: "ghcr.io",
		},
		{
			name: "defaults to docker.io",
			dep: model.Dependency{
				Name: "nginx",
			},
			expected: "docker.io",
		},
		{
			name: "empty metadata defaults to docker.io",
			dep: model.Dependency{
				Name: "nginx",
				Metadata: map[string]string{
					"registry_host": "",
				},
			},
			expected: "docker.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.getRegistryHost(tt.dep)
			if result != tt.expected {
				t.Errorf("getRegistryHost() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetRepository(t *testing.T) {
	resolver := New(Options{})

	tests := []struct {
		name     string
		dep      model.Dependency
		expected string
	}{
		{
			name: "uses metadata repository",
			dep: model.Dependency{
				Name: "go",
				Metadata: map[string]string{
					"repository":    "library/golang",
					"registry_host": "docker.io",
				},
			},
			expected: "library/golang",
		},
		{
			name: "falls back to name with library prefix for Docker Hub",
			dep: model.Dependency{
				Name: "nginx",
				Metadata: map[string]string{
					"registry_host": "docker.io",
				},
			},
			expected: "nginx",
		},
		{
			name: "user namespaced image",
			dep: model.Dependency{
				Name: "myuser/myimage",
				Metadata: map[string]string{
					"registry_host": "docker.io",
				},
			},
			expected: "myuser/myimage",
		},
		{
			name: "GHCR image without library prefix",
			dep: model.Dependency{
				Name: "myorg/myimage",
				Metadata: map[string]string{
					"registry_host": "ghcr.io",
				},
			},
			expected: "myorg/myimage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.getRepository(tt.dep)
			if result != tt.expected {
				t.Errorf("getRepository() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestParseTag(t *testing.T) {
	tests := []struct {
		input       string
		wantVersion string
		wantSuffix  string
		wantRaw     string
		wantErr     bool
	}{
		// Basic semver
		{"1", "1.0.0", "", "1", false},
		{"1.24", "1.24.0", "", "1.24", false},
		{"1.24.3", "1.24.3", "", "1.24.3", false},
		{"v1.24.3", "1.24.3", "", "v1.24.3", false},
		// With suffix
		{"1.24-alpine", "1.24.0", "alpine", "1.24-alpine", false},
		{"1.24-alpine3.20", "1.24.0", "alpine3.20", "1.24-alpine3.20", false},
		{"1.24.1-bookworm", "1.24.1", "bookworm", "1.24.1-bookworm", false},
		{"v1.2.3-slim", "1.2.3", "slim", "v1.2.3-slim", false},
		// Invalid cases (non-semver base)
		{"latest", "", "", "", true},
		{"bookworm", "", "", "", true},
		{"alpine", "", "", "", true},
		{"", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed, err := ParseTag(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTag(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTag(%q) unexpected error: %v", tt.input, err)
			}
			if parsed.Version.String() != tt.wantVersion {
				t.Errorf("ParseTag(%q).Version = %q, want %q", tt.input, parsed.Version.String(), tt.wantVersion)
			}
			if parsed.Suffix != tt.wantSuffix {
				t.Errorf("ParseTag(%q).Suffix = %q, want %q", tt.input, parsed.Suffix, tt.wantSuffix)
			}
			if parsed.Raw != tt.wantRaw {
				t.Errorf("ParseTag(%q).Raw = %q, want %q", tt.input, parsed.Raw, tt.wantRaw)
			}
		})
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1", "1.0.0"},
		{"1.2", "1.2.0"},
		{"1.2.3", "1.2.3"},
		{"v1.2.3", "1.2.3"},
		{"12", "12.0.0"},
		{"12.10", "12.10.0"},
		{"12.10.0", "12.10.0"},
		// With suffix - returns base version only
		{"1.24-alpine", "1.24.0"},
		{"1.24.1-bookworm", "1.24.1"},
		// Invalid cases
		{"bookworm", ""},
		{"latest", ""},
		{".1.2.3", ""},
		{"1.2.3.", ""},
		{"", ""},
		{"1.2.3.4", ""},
		{"abc.1.2", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeTag(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false, // All fields are optional
		},
		{
			name:    "empty config",
			config:  map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "valid config with registry_url",
			config: map[string]interface{}{
				"registry_url": "https://custom.registry.io",
			},
			wantErr: false,
		},
		{
			name: "valid config with timeout",
			config: map[string]interface{}{
				"timeout": "60s",
			},
			wantErr: false,
		},
		{
			name: "valid config with ghcr_token",
			config: map[string]interface{}{
				"ghcr_token": "ghr_xxxx",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsDockerHubRegistry(t *testing.T) {
	tests := []struct {
		host     string
		expected bool
	}{
		{"docker.io", true},
		{"registry-1.docker.io", true},
		{"registry.hub.docker.com", true},
		{"", true}, // Empty defaults to Docker Hub
		{"ghcr.io", false},
		{"quay.io", false},
		{"gcr.io", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := isDockerHubRegistry(tt.host)
			if result != tt.expected {
				t.Errorf("isDockerHubRegistry(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}
