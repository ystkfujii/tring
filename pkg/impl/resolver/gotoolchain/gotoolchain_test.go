package gotoolchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ystkfujii/tring/internal/domain/model"
)

func TestResolverResolve(t *testing.T) {
	// Create a mock server
	releases := []goRelease{
		{
			Version: "go1.22.2",
			Stable:  true,
			Files:   []goFile{{Filename: "go1.22.2.src.tar.gz", Kind: "source"}},
		},
		{
			Version: "go1.22.1",
			Stable:  true,
			Files:   []goFile{{Filename: "go1.22.1.src.tar.gz", Kind: "source"}},
		},
		{
			Version: "go1.21.9",
			Stable:  true,
			Files:   []goFile{{Filename: "go1.21.9.src.tar.gz", Kind: "source"}},
		},
		{
			// RC versions are now supported with prerelease normalization
			Version: "go1.23rc1",
			Stable:  false,
			Files:   []goFile{{Filename: "go1.23rc1.src.tar.gz", Kind: "source"}},
		},
		{
			Version: "go1.23beta2",
			Stable:  false,
			Files:   []goFile{{Filename: "go1.23beta2.src.tar.gz", Kind: "source"}},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.URL.Query().Get("mode") != "json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(releases); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	// The dependency name doesn't matter for gotoolchain resolver
	dep := model.Dependency{
		Name: "go",
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	// Should get all 5 versions including rc and beta
	if len(candidates.Items) != 5 {
		t.Errorf("Resolve() returned %d candidates, want 5", len(candidates.Items))
	}

	// Check that we have the expected versions
	versions := make(map[string]bool)
	for _, c := range candidates.Items {
		versions[c.Version.String()] = true
	}

	expectedVersions := []string{"1.22.2", "1.22.1", "1.21.9", "1.23.0-rc.1", "1.23.0-beta.2"}
	for _, v := range expectedVersions {
		if !versions[v] {
			t.Errorf("Expected version %s not found in candidates, got: %v", v, versions)
		}
	}
}

func TestResolverKind(t *testing.T) {
	resolver := New(Options{})
	if resolver.Kind() != "gotoolchain" {
		t.Errorf("Kind() = %s, want gotoolchain", resolver.Kind())
	}
}

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"go1.22.0", "1.22.0", false},
		{"go1.21.9", "1.21.9", false},
		{"go1.23rc1", "1.23.0-rc.1", false},
		{"go1.23beta2", "1.23.0-beta.2", false},
		{"go1.23alpha1", "1.23.0-alpha.1", false},
		{"1.22.0", "1.22.0", false},
		{"1.23rc1", "1.23.0-rc.1", false},
		{"1.23", "1.23.0", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := parseGoVersion(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseGoVersion(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseGoVersion(%q) error = %v", tt.input, err)
				return
			}
			if v.String() != tt.expected {
				t.Errorf("parseGoVersion(%q) = %s, want %s", tt.input, v.String(), tt.expected)
			}
		})
	}
}

func TestNewResolver(t *testing.T) {
	resolver, err := NewResolver(nil)
	if err != nil {
		t.Errorf("NewResolver(nil) error = %v", err)
	}
	if resolver == nil {
		t.Error("NewResolver(nil) returned nil resolver")
	}

	// Test with config
	config := map[string]interface{}{
		"base_url": "https://example.com",
		"timeout":  "10s",
	}
	resolver, err = NewResolver(config)
	if err != nil {
		t.Errorf("NewResolver() error = %v", err)
	}
	if resolver == nil {
		t.Error("NewResolver() returned nil resolver")
	}
}
