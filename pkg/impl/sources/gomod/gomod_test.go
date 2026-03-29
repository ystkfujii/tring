package gomod

import (
	"context"
	"os"
	"path/filepath"
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

func TestGomodExtract(t *testing.T) {
	// Create a temp directory with a test go.mod
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21

require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
)
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{paths: []string{gomodPath}}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should only get direct dependencies (not indirect)
	if len(deps) != 2 {
		t.Errorf("Extract() returned %d deps, want 2", len(deps))
	}

	// Check first dependency
	found := false
	for _, dep := range deps {
		if dep.Name == "github.com/spf13/cobra" {
			found = true
			if dep.Version.Original() != "v1.8.0" {
				t.Errorf("cobra version = %s, want v1.8.0", dep.Version.Original())
			}
			if dep.SourceKind != "gomod" {
				t.Errorf("SourceKind = %s, want gomod", dep.SourceKind)
			}
			if dep.FilePath != gomodPath {
				t.Errorf("FilePath = %s, want %s", dep.FilePath, gomodPath)
			}
		}
	}
	if !found {
		t.Error("cobra dependency not found")
	}
}

func TestGomodApply(t *testing.T) {
	// Create a temp directory with a test go.mod
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21

require (
	github.com/spf13/cobra v1.8.0
	github.com/stretchr/testify v1.9.0
)
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{paths: []string{gomodPath}}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "github.com/spf13/cobra",
				Version:    mustParse("v1.8.0"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    "github.com/spf13/cobra",
			},
			CurrentVersion: mustParse("v1.8.0"),
			TargetVersion:  mustParse("v1.8.1"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	if !contains(string(data), "github.com/spf13/cobra v1.8.1") {
		t.Errorf("go.mod should contain updated version:\n%s", string(data))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
