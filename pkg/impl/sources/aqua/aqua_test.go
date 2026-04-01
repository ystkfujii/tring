package aqua

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]interface{}
		wantErr string
	}{
		{
			name: "valid config",
			raw: map[string]interface{}{
				"file_paths":          []interface{}{"aqua.yaml"},
				"targets":             []interface{}{"packages", "registries"},
				"unsupported_version": "skip",
			},
		},
		{
			name: "missing file_paths",
			raw: map[string]interface{}{
				"targets": []interface{}{"packages"},
			},
			wantErr: "file_paths",
		},
		{
			name: "invalid target",
			raw: map[string]interface{}{
				"file_paths": []interface{}{"aqua.yaml"},
				"targets":    []interface{}{"packages", "unknown"},
			},
			wantErr: "targets[1]",
		},
		{
			name: "invalid unsupported version mode",
			raw: map[string]interface{}{
				"file_paths":          []interface{}{"aqua.yaml"},
				"targets":             []interface{}{"packages"},
				"unsupported_version": "warn",
			},
			wantErr: "unsupported_version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.raw)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("ValidateConfig() error = %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("ValidateConfig() expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ValidateConfig() error = %v, want substring %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestExtract(t *testing.T) {
	tmpDir := t.TempDir()
	localRegistryPath := filepath.Join(tmpDir, "registry.yaml")
	if err := os.WriteFile(localRegistryPath, []byte("packages: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	aquaPath := filepath.Join(tmpDir, "aqua.yaml")
	content := `registries:
  - type: standard
    ref: v4.448.0
  - type: local
    name: local
    path: registry.yaml
packages:
  - name: kubernetes/kubectl@v1.34.3
  - name: kubernetes-sigs/kubebuilder
    version: v4.10.1
  - name: kubernetes-sigs/kustomize@kustomize/v5.8.0
  - name: clamoriniere/crd-to-markdown@v0.0.3
    registry: local
  - name: owner/commit@abcdef1234567890
`
	if err := os.WriteFile(aquaPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := &Source{
		paths: []string{aquaPath},
		targets: map[string]bool{
			aquahelper.TargetPackages:   true,
			aquahelper.TargetRegistries: true,
		},
		unsupportedVersion: aquahelper.UnsupportedVersionSkip,
	}

	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(deps) != 5 {
		t.Fatalf("expected 5 dependencies, got %d", len(deps))
	}

	registryDep := deps[0]
	if registryDep.Name != "aquaproj/aqua-registry" {
		t.Fatalf("registry dependency name = %q", registryDep.Name)
	}
	if registryDep.Version.Original() != "v4.448.0" {
		t.Fatalf("registry dependency version = %q", registryDep.Version.Original())
	}

	kubebuilder := deps[2]
	if kubebuilder.Metadata[aquahelper.MetadataVersionField] != aquahelper.VersionFieldVersionField {
		t.Fatalf("kubebuilder version field = %q", kubebuilder.Metadata[aquahelper.MetadataVersionField])
	}

	kustomize := deps[3]
	if kustomize.Version.Original() != "v5.8.0" {
		t.Fatalf("kustomize version = %q", kustomize.Version.Original())
	}
	if kustomize.Metadata[aquahelper.MetadataRawVersion] != "kustomize/v5.8.0" {
		t.Fatalf("kustomize raw version = %q", kustomize.Metadata[aquahelper.MetadataRawVersion])
	}
	if kustomize.Metadata[aquahelper.MetadataVersionPrefix] != "kustomize/" {
		t.Fatalf("kustomize version prefix = %q", kustomize.Metadata[aquahelper.MetadataVersionPrefix])
	}

	localDep := deps[4]
	if localDep.Metadata[aquahelper.MetadataRegistryType] != aquahelper.RegistryTypeLocal {
		t.Fatalf("local dep registry type = %q", localDep.Metadata[aquahelper.MetadataRegistryType])
	}
	if localDep.Metadata[aquahelper.MetadataLocalRegistryPath] != localRegistryPath {
		t.Fatalf("local dep registry path = %q, want %q", localDep.Metadata[aquahelper.MetadataLocalRegistryPath], localRegistryPath)
	}
}

func TestExtractUnsupportedVersion(t *testing.T) {
	tmpDir := t.TempDir()
	aquaPath := filepath.Join(tmpDir, "aqua.yaml")
	content := `registries:
  - type: standard
    ref: v4.448.0
packages:
  - name: owner/repo@deadbeef
`
	if err := os.WriteFile(aquaPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("skip", func(t *testing.T) {
		src := &Source{
			paths: []string{aquaPath},
			targets: map[string]bool{
				aquahelper.TargetPackages: true,
			},
			unsupportedVersion: aquahelper.UnsupportedVersionSkip,
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(deps) != 0 {
			t.Fatalf("expected 0 dependencies, got %d", len(deps))
		}
	})

	t.Run("error", func(t *testing.T) {
		src := &Source{
			paths: []string{aquaPath},
			targets: map[string]bool{
				aquahelper.TargetPackages: true,
			},
			unsupportedVersion: aquahelper.UnsupportedVersionError,
		}

		_, err := src.Extract(context.Background())
		if err == nil {
			t.Fatal("Extract() expected error")
		}
		if !strings.Contains(err.Error(), "unsupported version") {
			t.Fatalf("Extract() error = %v", err)
		}
	})
}

func TestApply(t *testing.T) {
	tmpDir := t.TempDir()
	aquaPath := filepath.Join(tmpDir, "aqua.yaml")
	content := `registries:
  # standard registry comment
  - type: standard
    ref: v4.448.0 # keep inline comment
  - type: local
    name: local
    path: registry.yaml
packages:
  - name: kubernetes/kubectl@v1.34.3 # name embedded
  - name: kubernetes-sigs/kubebuilder
    version: v4.10.1
  - name: kubernetes-sigs/kustomize@kustomize/v5.8.0
  - name: clamoriniere/crd-to-markdown@v0.0.3
    registry: local # keep local comment
`
	if err := os.WriteFile(aquaPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "registry.yaml"), []byte("packages: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	src := &Source{
		paths: []string{aquaPath},
		targets: map[string]bool{
			aquahelper.TargetPackages:   true,
			aquahelper.TargetRegistries: true,
		},
		unsupportedVersion: aquahelper.UnsupportedVersionSkip,
	}

	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	findDep := func(name, objectKind string) model.Dependency {
		t.Helper()
		for _, dep := range deps {
			if dep.Name == name && dep.Metadata[aquahelper.MetadataObjectKind] == objectKind {
				return dep
			}
		}
		t.Fatalf("dependency %q (%s) not found", name, objectKind)
		return model.Dependency{}
	}

	changes := []model.PlannedChange{
		{
			Dependency:     findDep("aquaproj/aqua-registry", aquahelper.ObjectKindRegistry),
			CurrentVersion: mustParse(t, "v4.448.0"),
			TargetVersion:  mustParse(t, "v4.449.0"),
			SelectedCandidate: &model.Candidate{
				Version: mustParse(t, "v4.449.0"),
				Metadata: map[string]string{
					aquahelper.MetadataRawVersion: "v4.449.0",
				},
			},
		},
		{
			Dependency:     findDep("kubernetes/kubectl", aquahelper.ObjectKindPackage),
			CurrentVersion: mustParse(t, "v1.34.3"),
			TargetVersion:  mustParse(t, "v1.34.4"),
			SelectedCandidate: &model.Candidate{
				Version: mustParse(t, "v1.34.4"),
				Metadata: map[string]string{
					aquahelper.MetadataRawVersion: "v1.34.4",
				},
			},
		},
		{
			Dependency:     findDep("kubernetes-sigs/kubebuilder", aquahelper.ObjectKindPackage),
			CurrentVersion: mustParse(t, "v4.10.1"),
			TargetVersion:  mustParse(t, "v4.11.0"),
			SelectedCandidate: &model.Candidate{
				Version: mustParse(t, "v4.11.0"),
				Metadata: map[string]string{
					aquahelper.MetadataRawVersion: "v4.11.0",
				},
			},
		},
		{
			Dependency:     findDep("kubernetes-sigs/kustomize", aquahelper.ObjectKindPackage),
			CurrentVersion: mustParse(t, "v5.8.0"),
			TargetVersion:  mustParse(t, "v5.9.0"),
			SelectedCandidate: &model.Candidate{
				Version: mustParse(t, "v5.9.0"),
				Metadata: map[string]string{
					aquahelper.MetadataRawVersion: "kustomize/v5.9.0",
				},
			},
		},
		{
			Dependency:     findDep("clamoriniere/crd-to-markdown", aquahelper.ObjectKindPackage),
			CurrentVersion: mustParse(t, "v0.0.3"),
			TargetVersion:  mustParse(t, "v0.0.4"),
			SelectedCandidate: &model.Candidate{
				Version: mustParse(t, "v0.0.4"),
				Metadata: map[string]string{
					aquahelper.MetadataRawVersion: "v0.0.4",
				},
			},
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(aquaPath)
	if err != nil {
		t.Fatal(err)
	}

	expected := `registries:
  # standard registry comment
  - type: standard
    ref: v4.449.0 # keep inline comment
  - type: local
    name: local
    path: registry.yaml
packages:
  - name: kubernetes/kubectl@v1.34.4 # name embedded
  - name: kubernetes-sigs/kubebuilder
    version: v4.11.0
  - name: kubernetes-sigs/kustomize@kustomize/v5.9.0
  - name: clamoriniere/crd-to-markdown@v0.0.4
    registry: local # keep local comment
`
	if string(got) != expected {
		t.Fatalf("unexpected aqua.yaml\n=== got ===\n%s\n=== want ===\n%s", got, expected)
	}
}

func TestApplyDuplicateLocator(t *testing.T) {
	tmpDir := t.TempDir()
	aquaPath := filepath.Join(tmpDir, "aqua.yaml")
	content := `registries:
  - type: standard
    ref: v4.448.0
packages:
  - name: kubernetes/kubectl@v1.34.3
`
	if err := os.WriteFile(aquaPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	src := &Source{
		paths: []string{aquaPath},
		targets: map[string]bool{
			aquahelper.TargetPackages: true,
		},
		unsupportedVersion: aquahelper.UnsupportedVersionSkip,
	}

	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	change := model.PlannedChange{
		Dependency:     deps[0],
		CurrentVersion: mustParse(t, "v1.34.3"),
		TargetVersion:  mustParse(t, "v1.34.4"),
		SelectedCandidate: &model.Candidate{
			Version: mustParse(t, "v1.34.4"),
			Metadata: map[string]string{
				aquahelper.MetadataRawVersion: "v1.34.4",
			},
		},
	}

	conflictingChange := model.PlannedChange{
		Dependency:     deps[0],
		CurrentVersion: mustParse(t, "v1.34.3"),
		TargetVersion:  mustParse(t, "v1.34.5"),
		SelectedCandidate: &model.Candidate{
			Version: mustParse(t, "v1.34.5"),
			Metadata: map[string]string{
				aquahelper.MetadataRawVersion: "v1.34.5",
			},
		},
	}

	err = src.Apply(context.Background(), []model.PlannedChange{change, conflictingChange})
	if err == nil {
		t.Fatal("Apply() expected duplicate locator error")
	}
	if !strings.Contains(err.Error(), "duplicate aqua locator") {
		t.Fatalf("Apply() error = %v", err)
	}

	got, readErr := os.ReadFile(aquaPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != content {
		t.Fatalf("aqua.yaml should remain unchanged on duplicate locator error\n=== got ===\n%s\n=== want ===\n%s", got, content)
	}
}
