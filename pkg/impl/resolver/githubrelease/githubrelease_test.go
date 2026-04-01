package githubrelease

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

//nolint:unparam // test helper intentionally uses same version
func mustParse(s string) *semver.Version {
	v, err := semver.NewVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestFactoryCreateTokenFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envToken  string
		wantToken string
	}{
		{
			name:      "env token is used",
			envToken:  "env-token",
			wantToken: "env-token",
		},
		{
			name:      "no token when env is empty",
			envToken:  "",
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envToken != "" {
				t.Setenv("GITHUB_TOKEN", tt.envToken)
			}

			factory := &Factory{}
			resolver, err := factory.Create(map[string]interface{}{})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			r, ok := resolver.(*Resolver)
			if !ok {
				t.Fatal("Create() did not return *Resolver")
			}

			if r.token != tt.wantToken {
				t.Errorf("token = %q, want %q", r.token, tt.wantToken)
			}
		})
	}
}

func TestResolverAddHeadersWithToken(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		wantAuth  string
		hasHeader bool
	}{
		{
			name:      "with token",
			token:     "test-token",
			wantAuth:  "Bearer test-token",
			hasHeader: true,
		},
		{
			name:      "without token",
			token:     "",
			wantAuth:  "",
			hasHeader: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(Options{Token: tt.token})
			req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/test", nil)
			r.addHeaders(req)

			auth := req.Header.Get("Authorization")
			if tt.hasHeader {
				if auth != tt.wantAuth {
					t.Errorf("Authorization header = %q, want %q", auth, tt.wantAuth)
				}
			} else {
				if auth != "" {
					t.Errorf("Authorization header should be empty, got %q", auth)
				}
			}
		})
	}
}

func TestResolverResolve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/tags":
			w.Write([]byte(`[
				{"name": "v1.0.0", "commit": {"sha": "abc123"}},
				{"name": "v1.1.0", "commit": {"sha": "def456"}},
				{"name": "v2.0.0", "commit": {"sha": "ghi789"}}
			]`))
		case "/repos/owner/repo/commits/abc123":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
		case "/repos/owner/repo/commits/def456":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-02-01T00:00:00Z"}}}`))
		case "/repos/owner/repo/commits/ghi789":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-03-01T00:00:00Z"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "owner/repo",
		Version: mustParse("v1.0.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if len(candidates.Items) != 3 {
		t.Errorf("Resolve() returned %d candidates, want 3", len(candidates.Items))
	}

	// Check v1.1.0 has correct metadata
	for _, c := range candidates.Items {
		if c.Version.Original() == "v1.1.0" {
			if c.Metadata["commit_sha"] != "def456" {
				t.Errorf("v1.1.0 commit_sha = %q, want %q", c.Metadata["commit_sha"], "def456")
			}
			expectedTime := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
			if !c.ReleasedAt.Equal(expectedTime) {
				t.Errorf("v1.1.0 ReleasedAt = %v, want %v", c.ReleasedAt, expectedTime)
			}
		}
	}
}

func TestResolverResolveRepositoryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "owner/nonexistent",
		Version: mustParse("v1.0.0"),
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err == nil {
		t.Fatal("Resolve() expected error for not found repository")
	}

	if !contains(err.Error(), "repository not found") {
		t.Errorf("error should mention 'repository not found', got: %v", err)
	}
}

func TestResolverResolveAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "owner/repo",
		Version: mustParse("v1.0.0"),
	}

	_, err := resolver.Resolve(context.Background(), dep)
	if err == nil {
		t.Fatal("Resolve() expected error for API error")
	}

	// Should include status code and body snippet
	errStr := err.Error()
	if !contains(errStr, "403") {
		t.Errorf("error should mention status code 403, got: %v", err)
	}
	if !contains(errStr, "rate limit") {
		t.Errorf("error should include response body snippet, got: %v", err)
	}
}

func TestResolverSkipsNonSemverTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/owner/repo/tags" {
			w.Write([]byte(`[
				{"name": "v1.0.0", "commit": {"sha": "abc123"}},
				{"name": "main", "commit": {"sha": "def456"}},
				{"name": "latest", "commit": {"sha": "ghi789"}},
				{"name": "release-candidate", "commit": {"sha": "jkl012"}}
			]`))
			return
		}
		// Return empty commit info
		w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})

	dep := model.Dependency{
		Name:    "owner/repo",
		Version: mustParse("v1.0.0"),
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Only v1.0.0 is valid semver
	if len(candidates.Items) != 1 {
		t.Errorf("Resolve() returned %d candidates, want 1 (only valid semver)", len(candidates.Items))
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
			name: "with api_url",
			config: map[string]interface{}{
				"api_url": "https://github.example.com/api/v3",
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

func TestResolverInvalidDependencyName(t *testing.T) {
	resolver := New(Options{})

	tests := []struct {
		name string
	}{
		{"invalid"},
		{""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := model.Dependency{
				Name:    tt.name,
				Version: mustParse("v1.0.0"),
			}

			_, err := resolver.Resolve(context.Background(), dep)
			if err == nil {
				t.Error("Resolve() expected error for invalid dependency name")
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
