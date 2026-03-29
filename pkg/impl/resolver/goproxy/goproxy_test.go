package goproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

func mustParse(s string) *semver.Version {
	v, err := semver.NewVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestResolverResolve(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/spf13/cobra/@v/list":
			w.Write([]byte("v1.7.0\nv1.8.0\nv1.8.1\n"))
		case "/github.com/spf13/cobra/@v/v1.7.0.info":
			w.Write([]byte(`{"Version":"v1.7.0","Time":"2023-01-01T00:00:00Z"}`))
		case "/github.com/spf13/cobra/@v/v1.8.0.info":
			w.Write([]byte(`{"Version":"v1.8.0","Time":"2024-01-01T00:00:00Z"}`))
		case "/github.com/spf13/cobra/@v/v1.8.1.info":
			w.Write([]byte(`{"Version":"v1.8.1","Time":"2024-06-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "github.com/spf13/cobra",
		Version: mustParse("v1.8.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(candidates.Items) != 3 {
		t.Errorf("Resolve() returned %d candidates, want 3", len(candidates.Items))
	}

	// Check that versions are parsed correctly
	found := false
	for _, c := range candidates.Items {
		if c.Version.Original() == "v1.8.1" {
			found = true
			expectedTime := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
			if !c.ReleasedAt.Equal(expectedTime) {
				t.Errorf("v1.8.1 ReleasedAt = %v, want %v", c.ReleasedAt, expectedTime)
			}
		}
	}
	if !found {
		t.Error("v1.8.1 not found in candidates")
	}
}

func TestResolverResolveModuleNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := New(Options{
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "github.com/nonexistent/module",
		Version: mustParse("v1.0.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(candidates.Items) != 0 {
		t.Errorf("Resolve() returned %d candidates, want 0", len(candidates.Items))
	}
}

func TestEscapePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/foo/bar", "github.com/foo/bar"},
		{"github.com/Foo/Bar", "github.com/!foo/!bar"},
		{"k8s.io/API", "k8s.io/!a!p!i"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := escapePath(tt.input); got != tt.want {
				t.Errorf("escapePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFactoryCreate(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "empty config uses defaults",
			config:  map[string]interface{}{},
			wantErr: false,
		},
		{
			name: "with proxy_url http",
			config: map[string]interface{}{
				"proxy_url": "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "with proxy_url https",
			config: map[string]interface{}{
				"proxy_url": "https://goproxy.io",
			},
			wantErr: false,
		},
		{
			name: "with timeout",
			config: map[string]interface{}{
				"timeout": "60s",
			},
			wantErr: false,
		},
		{
			name: "with proxy_url and timeout",
			config: map[string]interface{}{
				"proxy_url": "http://localhost:8080",
				"timeout":   "60s",
			},
			wantErr: false,
		},
		{
			name: "invalid timeout",
			config: map[string]interface{}{
				"timeout": "invalid",
			},
			wantErr: true,
		},
	}

	factory := &Factory{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, err := factory.Create(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("Create() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if resolver == nil {
				t.Error("Create() returned nil resolver")
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty config is valid",
			config:  Config{},
			wantErr: false,
		},
		{
			name:    "valid http proxy_url",
			config:  Config{ProxyURL: "http://localhost:8080"},
			wantErr: false,
		},
		{
			name:    "valid https proxy_url",
			config:  Config{ProxyURL: "https://goproxy.io"},
			wantErr: false,
		},
		{
			name:    "proxy_url without scheme",
			config:  Config{ProxyURL: "localhost:8080"},
			wantErr: true,
			errMsg:  "scheme must be http or https",
		},
		{
			name:    "proxy_url with invalid scheme",
			config:  Config{ProxyURL: "ftp://example.com"},
			wantErr: true,
			errMsg:  "scheme must be http or https",
		},
		{
			name:    "proxy_url without host",
			config:  Config{ProxyURL: "http://"},
			wantErr: true,
			errMsg:  "host is required",
		},
		{
			name:    "valid timeout",
			config:  Config{Timeout: "30s"},
			wantErr: false,
		},
		{
			name:    "invalid timeout",
			config:  Config{Timeout: "invalid"},
			wantErr: true,
			errMsg:  "invalid timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if tt.wantErr {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestResolverPartialInfoFetchFailure(t *testing.T) {
	// Test that partial failures are tolerated (1 out of 3 fails)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/test/module/@v/list":
			w.Write([]byte("v1.0.0\nv1.1.0\nv1.2.0\n"))
		case "/github.com/test/module/@v/v1.0.0.info":
			w.Write([]byte(`{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`))
		case "/github.com/test/module/@v/v1.1.0.info":
			// This one fails
			http.Error(w, "internal error", http.StatusInternalServerError)
		case "/github.com/test/module/@v/v1.2.0.info":
			w.Write([]byte(`{"Version":"v1.2.0","Time":"2024-03-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "github.com/test/module",
		Version: mustParse("v1.0.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() should succeed with partial failures, got error: %v", err)
	}

	// Should have 2 candidates (v1.0.0 and v1.2.0), v1.1.0 failed
	if len(candidates.Items) != 2 {
		t.Errorf("Resolve() returned %d candidates, want 2", len(candidates.Items))
	}
}

func TestResolverMajorityInfoFetchFailure(t *testing.T) {
	// Test that majority failures result in an error (2 out of 3 fail)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/test/module/@v/list":
			w.Write([]byte("v1.0.0\nv1.1.0\nv1.2.0\n"))
		case "/github.com/test/module/@v/v1.0.0.info":
			http.Error(w, "internal error", http.StatusInternalServerError)
		case "/github.com/test/module/@v/v1.1.0.info":
			http.Error(w, "internal error", http.StatusInternalServerError)
		case "/github.com/test/module/@v/v1.2.0.info":
			w.Write([]byte(`{"Version":"v1.2.0","Time":"2024-03-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "github.com/test/module",
		Version: mustParse("v1.0.0"),
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err == nil {
		t.Fatal("Resolve() should return error when majority of info fetches fail")
	}

	// Error should mention the failure count
	if !contains(err.Error(), "2/3") {
		t.Errorf("Error should mention failure count, got: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFactoryCreateWithProxyURL(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/test/module/@v/list":
			w.Write([]byte("v1.0.0\n"))
		case "/github.com/test/module/@v/v1.0.0.info":
			w.Write([]byte(`{"Version":"v1.0.0","Time":"2024-01-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	factory := &Factory{}
	config := map[string]interface{}{
		"proxy_url": server.URL,
	}

	resolver, err := factory.Create(config)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	dep := model.Dependency{
		Name:    "github.com/test/module",
		Version: mustParse("v1.0.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(candidates.Items) != 1 {
		t.Errorf("Resolve() returned %d candidates, want 1", len(candidates.Items))
	}
}

func TestExtractGitHubRepoURL(t *testing.T) {
	tests := []struct {
		name       string
		modulePath string
		want       string
	}{
		{
			name:       "simple github module",
			modulePath: "github.com/owner/repo",
			want:       "https://github.com/owner/repo",
		},
		{
			name:       "github module with subdir",
			modulePath: "github.com/owner/repo/subdir",
			want:       "https://github.com/owner/repo",
		},
		{
			name:       "github module with v2",
			modulePath: "github.com/owner/repo/v2",
			want:       "https://github.com/owner/repo",
		},
		{
			name:       "github module with deep subdir",
			modulePath: "github.com/owner/repo/pkg/api/v1",
			want:       "https://github.com/owner/repo",
		},
		{
			name:       "non-github module",
			modulePath: "k8s.io/api",
			want:       "",
		},
		{
			name:       "golang.org module",
			modulePath: "golang.org/x/sync",
			want:       "",
		},
		{
			name:       "github.com only",
			modulePath: "github.com/",
			want:       "",
		},
		{
			name:       "github.com owner only",
			modulePath: "github.com/owner",
			want:       "",
		},
		{
			name:       "empty string",
			modulePath: "",
			want:       "",
		},
		{
			name:       "github actions style",
			modulePath: "github.com/actions/checkout",
			want:       "https://github.com/actions/checkout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractGitHubRepoURL(tt.modulePath)
			if got != tt.want {
				t.Errorf("extractGitHubRepoURL(%q) = %q, want %q", tt.modulePath, got, tt.want)
			}
		})
	}
}

func TestResolverRepoURLMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/spf13/cobra/@v/list":
			w.Write([]byte("v1.8.0\n"))
		case "/github.com/spf13/cobra/@v/v1.8.0.info":
			w.Write([]byte(`{"Version":"v1.8.0","Time":"2024-01-01T00:00:00Z"}`))
		case "/k8s.io/api/@v/list":
			w.Write([]byte("v0.28.0\n"))
		case "/k8s.io/api/@v/v0.28.0.info":
			w.Write([]byte(`{"Version":"v0.28.0","Time":"2024-01-01T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		ProxyURL:   server.URL,
		HTTPClient: server.Client(),
	})

	// Test GitHub module - should have repo_url metadata
	t.Run("github module has repo_url", func(t *testing.T) {
		dep := model.Dependency{
			Name:    "github.com/spf13/cobra",
			Version: mustParse("v1.8.0"),
		}

		candidates, err := resolver.Resolve(context.Background(), dep)
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		if len(candidates.Items) != 1 {
			t.Fatalf("Resolve() returned %d candidates, want 1", len(candidates.Items))
		}

		repoURL := candidates.Items[0].Metadata["repo_url"]
		if repoURL != "https://github.com/spf13/cobra" {
			t.Errorf("repo_url = %q, want %q", repoURL, "https://github.com/spf13/cobra")
		}
	})

	// Test non-GitHub module - should not have repo_url metadata
	t.Run("non-github module has no repo_url", func(t *testing.T) {
		dep := model.Dependency{
			Name:    "k8s.io/api",
			Version: mustParse("v0.28.0"),
		}

		candidates, err := resolver.Resolve(context.Background(), dep)
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}

		if len(candidates.Items) != 1 {
			t.Fatalf("Resolve() returned %d candidates, want 1", len(candidates.Items))
		}

		if candidates.Items[0].Metadata != nil {
			repoURL := candidates.Items[0].Metadata["repo_url"]
			if repoURL != "" {
				t.Errorf("non-github module should not have repo_url, got %q", repoURL)
			}
		}
	})
}
