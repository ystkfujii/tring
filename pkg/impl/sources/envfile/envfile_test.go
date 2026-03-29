package envfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestEnvfileExtract(t *testing.T) {
	// Create a temp directory with a test envfile
	tmpDir := t.TempDir()
	envfilePath := filepath.Join(tmpDir, "versions.env")

	content := `# Comment line
FOO = v1.2.3
BAR := v2.0.0
BAZ ?= v0.1.0
UNTRACKED = v9.9.9

.PHONY: build
build:
	go build ./...
`

	if err := os.WriteFile(envfilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test envfile: %v", err)
	}

	src := &Source{
		paths: []string{envfilePath},
		variables: []Variable{
			{Name: "FOO", ResolveWith: "github.com/foo/foo"},
			{Name: "BAR", ResolveWith: "github.com/bar/bar"},
		},
		varMap: map[string]Variable{
			"FOO": {Name: "FOO", ResolveWith: "github.com/foo/foo"},
			"BAR": {Name: "BAR", ResolveWith: "github.com/bar/bar"},
		},
	}

	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(deps) != 2 {
		t.Errorf("Extract() returned %d deps, want 2", len(deps))
	}

	// Check FOO dependency
	found := false
	for _, dep := range deps {
		if dep.Name == "github.com/foo/foo" {
			found = true
			if dep.Version.Original() != "v1.2.3" {
				t.Errorf("FOO version = %s, want v1.2.3", dep.Version.Original())
			}
			if dep.SourceKind != "envfile" {
				t.Errorf("SourceKind = %s, want envfile", dep.SourceKind)
			}
			if dep.Locator != "FOO" {
				t.Errorf("Locator = %s, want FOO", dep.Locator)
			}
		}
	}
	if !found {
		t.Error("FOO dependency not found")
	}
}

func TestEnvfileApply(t *testing.T) {
	// Create a temp directory with a test envfile
	tmpDir := t.TempDir()
	envfilePath := filepath.Join(tmpDir, "versions.env")

	content := `# Comment line
FOO = v1.2.3
BAR := v2.0.0
`

	if err := os.WriteFile(envfilePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test envfile: %v", err)
	}

	src := &Source{
		paths: []string{envfilePath},
		varMap: map[string]Variable{
			"FOO": {Name: "FOO", ResolveWith: "github.com/foo/foo"},
		},
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "github.com/foo/foo",
				Version:    mustParse("v1.2.3"),
				SourceKind: "envfile",
				FilePath:   envfilePath,
				Locator:    "FOO",
			},
			CurrentVersion: mustParse("v1.2.3"),
			TargetVersion:  mustParse("v1.2.4"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(envfilePath)
	if err != nil {
		t.Fatalf("failed to read envfile: %v", err)
	}

	if !strings.Contains(string(data), "v1.2.4") {
		t.Errorf("envfile should contain updated version:\n%s", string(data))
	}

	// BAR should be unchanged
	if !strings.Contains(string(data), "BAR := v2.0.0") {
		t.Errorf("envfile should preserve unchanged variables:\n%s", string(data))
	}

	// Comments should be preserved
	if !strings.Contains(string(data), "# Comment line") {
		t.Errorf("envfile should preserve comments:\n%s", string(data))
	}
}
