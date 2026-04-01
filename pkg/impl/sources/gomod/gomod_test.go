package gomod

import (
	"context"
	"fmt"
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

	src := &Source{paths: []string{gomodPath}, includeRequire: true}
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

	src := &Source{paths: []string{gomodPath}, includeRequire: true}

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

func TestGomodExtractGoVersion(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should get go directive + 1 require
	if len(deps) != 2 {
		t.Errorf("Extract() returned %d deps, want 2", len(deps))
	}

	// Check go directive
	found := false
	for _, dep := range deps {
		if dep.Locator == LocatorGoVersion {
			found = true
			if dep.Name != "go" {
				t.Errorf("go directive Name = %s, want go", dep.Name)
			}
			if dep.Version.Original() != "1.22.0" {
				t.Errorf("go directive version = %s, want 1.22.0", dep.Version.Original())
			}
		}
	}
	if !found {
		t.Error("go directive not found")
	}
}

func TestGomodExtractToolchain(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

toolchain go1.22.2

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackToolchain: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should get toolchain + 1 require
	if len(deps) != 2 {
		t.Errorf("Extract() returned %d deps, want 2", len(deps))
	}

	// Check toolchain directive
	found := false
	for _, dep := range deps {
		if dep.Locator == LocatorToolchain {
			found = true
			if dep.Name != "go" {
				t.Errorf("toolchain Name = %s, want go", dep.Name)
			}
			if dep.Version.Original() != "1.22.2" {
				t.Errorf("toolchain version = %s, want 1.22.2", dep.Version.Original())
			}
		}
	}
	if !found {
		t.Error("toolchain directive not found")
	}
}

func TestGomodExtractBothGoAndToolchain(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

toolchain go1.22.2

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
		trackToolchain: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should get go directive + toolchain + 1 require
	if len(deps) != 3 {
		t.Errorf("Extract() returned %d deps, want 3", len(deps))
	}
}

func TestGomodApplyGoVersion(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21.0

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.21.0"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorGoVersion,
			},
			CurrentVersion: mustParse("1.21.0"),
			TargetVersion:  mustParse("1.22.0"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	// FormatGoDirective formats 1.22.0 as "1.22" (patch=0 is omitted)
	if !contains(string(data), "go 1.22") {
		t.Errorf("go.mod should contain updated go version:\n%s", string(data))
	}
}

func TestGomodApplyToolchain(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

toolchain go1.22.1

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackToolchain: true,
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.22.1"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorToolchain,
			},
			CurrentVersion: mustParse("1.22.1"),
			TargetVersion:  mustParse("1.22.2"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	if !contains(string(data), "toolchain go1.22.2") {
		t.Errorf("go.mod should contain updated toolchain:\n%s", string(data))
	}
}

func TestGomodExtractIncludeRequireFalse(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

toolchain go1.22.2

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: false,
		trackGoVersion: true,
		trackToolchain: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Should only get go directive + toolchain (no require)
	if len(deps) != 2 {
		t.Errorf("Extract() returned %d deps, want 2", len(deps))
	}

	// Verify no require dependencies
	for _, dep := range deps {
		if dep.Locator != LocatorGoVersion && dep.Locator != LocatorToolchain {
			t.Errorf("unexpected dependency extracted: %s", dep.Name)
		}
	}
}

func TestGomodApplyMultipleChanges(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21.0

toolchain go1.21.5

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
		trackToolchain: true,
	}

	// Apply go, toolchain, and require changes together
	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.21.0"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorGoVersion,
			},
			CurrentVersion: mustParse("1.21.0"),
			TargetVersion:  mustParse("1.22.0"),
		},
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.21.5"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorToolchain,
			},
			CurrentVersion: mustParse("1.21.5"),
			TargetVersion:  mustParse("1.22.2"),
		},
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

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	content = string(data)
	// FormatGoDirective formats 1.22.0 as "1.22" (patch=0 is omitted)
	if !contains(content, "go 1.22") {
		t.Errorf("go.mod should contain updated go version:\n%s", content)
	}
	if !contains(content, "toolchain go1.22.2") {
		t.Errorf("go.mod should contain updated toolchain:\n%s", content)
	}
	if !contains(content, "github.com/spf13/cobra v1.8.1") {
		t.Errorf("go.mod should contain updated require:\n%s", content)
	}
}

func TestGomodApplyGoVersionOnly(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21.0

toolchain go1.21.5

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
		trackToolchain: true,
	}

	// Only update go directive
	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.21.0"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorGoVersion,
			},
			CurrentVersion: mustParse("1.21.0"),
			TargetVersion:  mustParse("1.22.0"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	resultContent := string(data)
	// go should be updated (FormatGoDirective formats 1.22.0 as "1.22")
	if !contains(resultContent, "go 1.22") {
		t.Errorf("go.mod should contain updated go version:\n%s", resultContent)
	}
	// toolchain should remain unchanged
	if !contains(resultContent, "toolchain go1.21.5") {
		t.Errorf("go.mod should keep original toolchain:\n%s", resultContent)
	}
	// require should remain unchanged
	if !contains(resultContent, "github.com/spf13/cobra v1.8.0") {
		t.Errorf("go.mod should keep original require:\n%s", resultContent)
	}
}

func TestGomodApplyToolchainOnly(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.21.0

toolchain go1.21.5

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: true,
		trackGoVersion: true,
		trackToolchain: true,
	}

	// Only update toolchain directive
	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    mustParse("1.21.5"),
				SourceKind: "gomod",
				FilePath:   gomodPath,
				Locator:    LocatorToolchain,
			},
			CurrentVersion: mustParse("1.21.5"),
			TargetVersion:  mustParse("1.22.2"),
		},
	}

	if err := src.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	data, err := os.ReadFile(gomodPath)
	if err != nil {
		t.Fatalf("failed to read go.mod: %v", err)
	}

	resultContent := string(data)
	// go should remain unchanged
	if !contains(resultContent, "go 1.21.0") {
		t.Errorf("go.mod should keep original go version:\n%s", resultContent)
	}
	// toolchain should be updated
	if !contains(resultContent, "toolchain go1.22.2") {
		t.Errorf("go.mod should contain updated toolchain:\n%s", resultContent)
	}
	// require should remain unchanged
	if !contains(resultContent, "github.com/spf13/cobra v1.8.0") {
		t.Errorf("go.mod should keep original require:\n%s", resultContent)
	}
}

func TestConfigShouldIncludeRequire(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name:     "nil defaults to true",
			config:   Config{ManifestPaths: []string{"go.mod"}},
			expected: true,
		},
		{
			name: "explicit true",
			config: Config{
				ManifestPaths:  []string{"go.mod"},
				IncludeRequire: boolPtr(true),
			},
			expected: true,
		},
		{
			name: "explicit false",
			config: Config{
				ManifestPaths:  []string{"go.mod"},
				IncludeRequire: boolPtr(false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.ShouldIncludeRequire(); got != tt.expected {
				t.Errorf("ShouldIncludeRequire() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}

// TestNewSourceIncludeRequire tests that include_require config option
// is properly propagated through source creation and extraction.
func TestNewSourceIncludeRequire(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22.0

toolchain go1.22.2

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	t.Run("include_require unset defaults to true", func(t *testing.T) {
		rawConfig := map[string]interface{}{
			"manifest_paths": []interface{}{"go.mod"},
		}

		src, err := NewSource(rawConfig, tmpDir)
		if err != nil {
			t.Fatalf("NewSource() error = %v", err)
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}

		// Should extract require dependency
		found := false
		for _, dep := range deps {
			if dep.Name == "github.com/spf13/cobra" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to extract require dependency when include_require is unset")
		}
	})

	t.Run("include_require false excludes require", func(t *testing.T) {
		rawConfig := map[string]interface{}{
			"manifest_paths":   []interface{}{"go.mod"},
			"include_require":  false,
			"track_go_version": true,
		}

		src, err := NewSource(rawConfig, tmpDir)
		if err != nil {
			t.Fatalf("NewSource() error = %v", err)
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}

		// Should NOT extract require dependency
		for _, dep := range deps {
			if dep.Name == "github.com/spf13/cobra" {
				t.Error("expected NOT to extract require dependency when include_require is false")
			}
		}

		// Should extract go directive
		found := false
		for _, dep := range deps {
			if dep.Locator == LocatorGoVersion {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to extract go directive when track_go_version is true")
		}
	})

	t.Run("include_require false with track_go_version only", func(t *testing.T) {
		rawConfig := map[string]interface{}{
			"manifest_paths":   []interface{}{"go.mod"},
			"include_require":  false,
			"track_go_version": true,
			"track_toolchain":  false,
		}

		src, err := NewSource(rawConfig, tmpDir)
		if err != nil {
			t.Fatalf("NewSource() error = %v", err)
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}

		// Should only extract go directive
		if len(deps) != 1 {
			t.Errorf("Extract() returned %d deps, want 1", len(deps))
		}

		if len(deps) > 0 && deps[0].Locator != LocatorGoVersion {
			t.Errorf("expected go directive, got locator %s", deps[0].Locator)
		}
	})

	t.Run("include_require false with track_toolchain only", func(t *testing.T) {
		rawConfig := map[string]interface{}{
			"manifest_paths":   []interface{}{"go.mod"},
			"include_require":  false,
			"track_go_version": false,
			"track_toolchain":  true,
		}

		src, err := NewSource(rawConfig, tmpDir)
		if err != nil {
			t.Fatalf("NewSource() error = %v", err)
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}

		// Should only extract toolchain directive
		if len(deps) != 1 {
			t.Errorf("Extract() returned %d deps, want 1", len(deps))
		}

		if len(deps) > 0 && deps[0].Locator != LocatorToolchain {
			t.Errorf("expected toolchain directive, got locator %s", deps[0].Locator)
		}
	})

	t.Run("include_require true extracts all", func(t *testing.T) {
		rawConfig := map[string]interface{}{
			"manifest_paths":   []interface{}{"go.mod"},
			"include_require":  true,
			"track_go_version": true,
			"track_toolchain":  true,
		}

		src, err := NewSource(rawConfig, tmpDir)
		if err != nil {
			t.Fatalf("NewSource() error = %v", err)
		}

		deps, err := src.Extract(context.Background())
		if err != nil {
			t.Fatalf("Extract() error = %v", err)
		}

		// Should extract all 3: go directive + toolchain + require
		if len(deps) != 3 {
			t.Errorf("Extract() returned %d deps, want 3", len(deps))
		}

		hasGo := false
		hasToolchain := false
		hasRequire := false
		for _, dep := range deps {
			switch dep.Locator {
			case LocatorGoVersion:
				hasGo = true
			case LocatorToolchain:
				hasToolchain = true
			default:
				if dep.Name == "github.com/spf13/cobra" {
					hasRequire = true
				}
			}
		}

		if !hasGo {
			t.Error("expected go directive")
		}
		if !hasToolchain {
			t.Error("expected toolchain directive")
		}
		if !hasRequire {
			t.Error("expected require dependency")
		}
	})
}

// TestExtractPrereleaseGoVersion tests extracting prerelease go directive (e.g., go 1.23rc1)
func TestExtractPrereleaseGoVersion(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.23rc1

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: false,
		trackGoVersion: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("Extract() returned %d deps, want 1", len(deps))
	}

	dep := deps[0]
	if dep.Locator != LocatorGoVersion {
		t.Errorf("expected go directive, got locator %s", dep.Locator)
	}
	// ParseGoVersion normalizes 1.23rc1 -> 1.23.0-rc.1
	if dep.Version.String() != "1.23.0-rc.1" {
		t.Errorf("go version = %s, want 1.23.0-rc.1", dep.Version.String())
	}
}

// TestExtractPrereleaseToolchain tests extracting prerelease toolchain directive (e.g., toolchain go1.23rc1)
func TestExtractPrereleaseToolchain(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22

toolchain go1.23rc1

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: false,
		trackToolchain: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("Extract() returned %d deps, want 1", len(deps))
	}

	dep := deps[0]
	if dep.Locator != LocatorToolchain {
		t.Errorf("expected toolchain directive, got locator %s", dep.Locator)
	}
	// ParseGoVersion normalizes go1.23rc1 -> 1.23.0-rc.1
	if dep.Version.String() != "1.23.0-rc.1" {
		t.Errorf("toolchain version = %s, want 1.23.0-rc.1", dep.Version.String())
	}
}

// TestExtractStableGoVersion tests extracting stable go directive
func TestExtractStableGoVersion(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.23.0

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: false,
		trackGoVersion: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("Extract() returned %d deps, want 1", len(deps))
	}

	dep := deps[0]
	if dep.Version.String() != "1.23.0" {
		t.Errorf("go version = %s, want 1.23.0", dep.Version.String())
	}
}

// TestExtractStableToolchain tests extracting stable toolchain directive
func TestExtractStableToolchain(t *testing.T) {
	tmpDir := t.TempDir()
	gomodPath := filepath.Join(tmpDir, "go.mod")

	content := `module example.com/test

go 1.22

toolchain go1.23.0

require github.com/spf13/cobra v1.8.0
`

	if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test go.mod: %v", err)
	}

	src := &Source{
		paths:          []string{gomodPath},
		includeRequire: false,
		trackToolchain: true,
	}
	deps, err := src.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("Extract() returned %d deps, want 1", len(deps))
	}

	dep := deps[0]
	if dep.Version.String() != "1.23.0" {
		t.Errorf("toolchain version = %s, want 1.23.0", dep.Version.String())
	}
}

// TestRoundTripGoVersion tests that Extract -> Apply -> Extract produces consistent results
func TestRoundTripGoVersion(t *testing.T) {
	tests := []struct {
		name              string
		initialGo         string
		initialToolchain  string
		targetGoVersion   string // semver format (e.g., "1.23.0-rc.1")
		expectedGo        string // what should be in go.mod after apply
		expectedToolchain string // what should be in go.mod after apply
	}{
		{
			name:              "stable version",
			initialGo:         "1.21.0",
			initialToolchain:  "go1.21.5",
			targetGoVersion:   "1.22.0",
			expectedGo:        "go 1.22",
			expectedToolchain: "toolchain go1.22.0",
		},
		{
			name:              "stable with patch",
			initialGo:         "1.21.0",
			initialToolchain:  "go1.21.5",
			targetGoVersion:   "1.22.3",
			expectedGo:        "go 1.22.3",
			expectedToolchain: "toolchain go1.22.3",
		},
		{
			name:              "prerelease rc",
			initialGo:         "1.22.0",
			initialToolchain:  "go1.22.0",
			targetGoVersion:   "1.23.0-rc.1",
			expectedGo:        "go 1.23rc1",
			expectedToolchain: "toolchain go1.23rc1",
		},
		{
			name:              "prerelease beta",
			initialGo:         "1.22.0",
			initialToolchain:  "go1.22.0",
			targetGoVersion:   "1.23.0-beta.2",
			expectedGo:        "go 1.23beta2",
			expectedToolchain: "toolchain go1.23beta2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			gomodPath := filepath.Join(tmpDir, "go.mod")

			content := fmt.Sprintf(`module example.com/test

go %s

toolchain %s
`, tt.initialGo, tt.initialToolchain)

			if err := os.WriteFile(gomodPath, []byte(content), 0644); err != nil {
				t.Fatalf("failed to write test go.mod: %v", err)
			}

			src := &Source{
				paths:          []string{gomodPath},
				trackGoVersion: true,
				trackToolchain: true,
			}

			// Apply changes
			targetVersion := mustParse(tt.targetGoVersion)
			currentGoVersion := mustParse(tt.initialGo)
			currentToolchainVersion := mustParse(tt.initialToolchain[2:]) // strip "go" prefix
			changes := []model.PlannedChange{
				{
					Dependency: model.Dependency{
						Name:       "go",
						SourceKind: "gomod",
						FilePath:   gomodPath,
						Locator:    LocatorGoVersion,
					},
					CurrentVersion: currentGoVersion,
					TargetVersion:  targetVersion,
				},
				{
					Dependency: model.Dependency{
						Name:       "go",
						SourceKind: "gomod",
						FilePath:   gomodPath,
						Locator:    LocatorToolchain,
					},
					CurrentVersion: currentToolchainVersion,
					TargetVersion:  targetVersion,
				},
			}

			if err := src.Apply(context.Background(), changes); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			// Read back the file to verify format
			data, err := os.ReadFile(gomodPath)
			if err != nil {
				t.Fatalf("failed to read go.mod: %v", err)
			}
			content = string(data)

			if !contains(content, tt.expectedGo) {
				t.Errorf("go.mod should contain %q:\n%s", tt.expectedGo, content)
			}
			if !contains(content, tt.expectedToolchain) {
				t.Errorf("go.mod should contain %q:\n%s", tt.expectedToolchain, content)
			}

			// Re-extract to verify round-trip
			deps, err := src.Extract(context.Background())
			if err != nil {
				t.Fatalf("Extract() after Apply error = %v", err)
			}

			if len(deps) != 2 {
				t.Fatalf("Extract() returned %d deps, want 2", len(deps))
			}

			for _, dep := range deps {
				if dep.Version.String() != tt.targetGoVersion {
					t.Errorf("re-extracted version = %s, want %s (locator: %s)",
						dep.Version.String(), tt.targetGoVersion, dep.Locator)
				}
			}
		})
	}
}
