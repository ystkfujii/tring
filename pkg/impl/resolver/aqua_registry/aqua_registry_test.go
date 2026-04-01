package aqua_registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	aquahelper "github.com/ystkfujii/tring/pkg/impl/aqua"
)

func mustParse(t *testing.T, s string) *semver.Version {
	t.Helper()
	v, err := semver.NewVersion(s)
	if err != nil {
		t.Fatalf("failed to parse version %q: %v", s, err)
	}
	return v
}

func TestResolverResolveStandardRegistry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/registry/v4.448.0/registry.yaml":
			w.Write([]byte(`packages:
  - name: kubernetes/kubectl
    type: github_release
    repo_owner: kubernetes
    repo_name: kubernetes
    version_filter: not (Version matches "-(alpha|beta|rc)")
`))
		case strings.HasPrefix(r.URL.Path, "/repos/kubernetes/kubernetes/releases"):
			w.Write([]byte(`[
  {"tag_name":"v1.34.3","published_at":"2024-01-01T00:00:00Z"},
  {"tag_name":"v1.34.4","published_at":"2024-02-01T00:00:00Z"},
  {"tag_name":"v1.35.0-beta.1","published_at":"2024-03-01T00:00:00Z"}
]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:          server.URL,
		RegistryBaseURL: server.URL + "/registry",
		HTTPClient:      server.Client(),
	})

	dep := model.Dependency{
		Name:    "kubernetes/kubectl",
		Version: mustParse(t, "v1.34.3"),
		Metadata: map[string]string{
			aquahelper.MetadataObjectKind:   aquahelper.ObjectKindPackage,
			aquahelper.MetadataRegistryType: aquahelper.RegistryTypeStandard,
			aquahelper.MetadataStandardRef:  "v4.448.0",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates.Items) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates.Items))
	}
	if candidates.Items[1].Metadata["repo_url"] != "https://github.com/kubernetes/kubernetes" {
		t.Fatalf("repo_url = %q", candidates.Items[1].Metadata["repo_url"])
	}
}

func TestResolverResolveLocalRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.yaml")
	if err := os.WriteFile(registryPath, []byte(`packages:
  - name: clamoriniere/crd-to-markdown
    type: github_tag
    version_source: github_tag
    repo_owner: clamoriniere
    repo_name: crd-to-markdown
`), 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/clamoriniere/crd-to-markdown/tags"):
			w.Write([]byte(`[
  {"name":"v0.0.3","commit":{"sha":"abc"}},
  {"name":"v0.0.4","commit":{"sha":"def"}}
]`))
		case r.URL.Path == "/repos/clamoriniere/crd-to-markdown/commits/abc":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
		case r.URL.Path == "/repos/clamoriniere/crd-to-markdown/commits/def":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-02-01T00:00:00Z"}}}`))
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
		Name:    "clamoriniere/crd-to-markdown",
		Version: mustParse(t, "v0.0.3"),
		Metadata: map[string]string{
			aquahelper.MetadataObjectKind:        aquahelper.ObjectKindPackage,
			aquahelper.MetadataRegistryType:      aquahelper.RegistryTypeLocal,
			aquahelper.MetadataLocalRegistryPath: registryPath,
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates.Items) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates.Items))
	}
	if candidates.Items[1].Version.Original() != "v0.0.4" {
		t.Fatalf("latest candidate = %q", candidates.Items[1].Version.Original())
	}
}

func TestResolverNormalizePrefixedVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/registry/v4.448.0/registry.yaml":
			w.Write([]byte(`packages:
  - name: kubernetes-sigs/kustomize
    type: github_tag
    version_source: github_tag
    repo_owner: kubernetes-sigs
    repo_name: kustomize
    version_prefix: kustomize/
`))
		case strings.HasPrefix(r.URL.Path, "/repos/kubernetes-sigs/kustomize/tags"):
			w.Write([]byte(`[
  {"name":"kustomize/v5.8.0","commit":{"sha":"abc"}},
  {"name":"kustomize/v5.8.1","commit":{"sha":"def"}}
]`))
		case r.URL.Path == "/repos/kubernetes-sigs/kustomize/commits/abc":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
		case r.URL.Path == "/repos/kubernetes-sigs/kustomize/commits/def":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-02-01T00:00:00Z"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := New(Options{
		APIURL:          server.URL,
		RegistryBaseURL: server.URL + "/registry",
		HTTPClient:      server.Client(),
	})

	dep := model.Dependency{
		Name:    "kubernetes-sigs/kustomize",
		Version: mustParse(t, "v5.8.0"),
		Metadata: map[string]string{
			aquahelper.MetadataObjectKind:   aquahelper.ObjectKindPackage,
			aquahelper.MetadataRegistryType: aquahelper.RegistryTypeStandard,
			aquahelper.MetadataStandardRef:  "v4.448.0",
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates.Items) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates.Items))
	}

	latest := candidates.Items[1]
	if latest.Version.Original() != "v5.8.1" {
		t.Fatalf("normalized version = %q", latest.Version.Original())
	}
	if latest.Metadata[aquahelper.MetadataRawVersion] != "kustomize/v5.8.1" {
		t.Fatalf("raw version = %q", latest.Metadata[aquahelper.MetadataRawVersion])
	}
	if latest.Metadata[aquahelper.MetadataVersionPrefix] != "kustomize/" {
		t.Fatalf("version prefix = %q", latest.Metadata[aquahelper.MetadataVersionPrefix])
	}
}

func TestResolverResolveStandardRegistryRef(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/repos/aquaproj/aqua-registry/tags"):
			w.Write([]byte(`[
  {"name":"v4.448.0","commit":{"sha":"abc"}},
  {"name":"v4.449.0","commit":{"sha":"def"}}
]`))
		case r.URL.Path == "/repos/aquaproj/aqua-registry/commits/abc":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-01-01T00:00:00Z"}}}`))
		case r.URL.Path == "/repos/aquaproj/aqua-registry/commits/def":
			w.Write([]byte(`{"commit":{"committer":{"date":"2024-02-01T00:00:00Z"}}}`))
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
		Name:    "aquaproj/aqua-registry",
		Version: mustParse(t, "v4.448.0"),
		Metadata: map[string]string{
			aquahelper.MetadataObjectKind: aquahelper.ObjectKindRegistry,
		},
	}

	candidates, err := resolver.Resolve(context.Background(), dep)
	if err != nil {
		t.Fatal(err)
	}

	if len(candidates.Items) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates.Items))
	}
	if !candidates.Items[1].ReleasedAt.Equal(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected released at: %v", candidates.Items[1].ReleasedAt)
	}
}
