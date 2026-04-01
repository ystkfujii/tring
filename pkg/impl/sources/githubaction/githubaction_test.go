package githubaction

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

func TestParseLine(t *testing.T) {
	s := &Source{}

	tests := []struct {
		name          string
		line          string
		wantDep       bool
		wantName      string
		wantVersion   string
		wantPinnedSHA bool
	}{
		{
			name:          "simple version ref",
			line:          "      uses: actions/checkout@v4.1.1",
			wantDep:       true,
			wantName:      "actions/checkout",
			wantVersion:   "v4.1.1",
			wantPinnedSHA: false,
		},
		{
			name:          "version ref with comment",
			line:          "      uses: actions/setup-go@v5.0.0 # comment",
			wantDep:       true,
			wantName:      "actions/setup-go",
			wantVersion:   "v5.0.0",
			wantPinnedSHA: false,
		},
		{
			name:          "SHA pin with version comment",
			line:          "      uses: actions/create-github-app-token@29824e69f54612133e76f7eaac726eef6c875baf # v2.2.1",
			wantDep:       true,
			wantName:      "actions/create-github-app-token",
			wantVersion:   "v2.2.1",
			wantPinnedSHA: true,
		},
		{
			name:          "subpath action",
			line:          "      uses: actions/cache/save@v3.2.1",
			wantDep:       true,
			wantName:      "actions/cache",
			wantVersion:   "v3.2.1",
			wantPinnedSHA: false,
		},
		{
			name:          "SHA pin with subpath",
			line:          "      uses: github/codeql-action/init@1234567890123456789012345678901234567890 # v2.1.0",
			wantDep:       true,
			wantName:      "github/codeql-action",
			wantVersion:   "v2.1.0",
			wantPinnedSHA: true,
		},
		{
			name:    "local action - skip",
			line:    "      uses: ./.github/actions/local",
			wantDep: false,
		},
		{
			name:    "floating major tag - skip",
			line:    "      uses: actions/checkout@v4",
			wantDep: false,
		},
		{
			name:    "main branch - skip",
			line:    "      uses: actions/checkout@main",
			wantDep: false,
		},
		{
			name:    "SHA without version comment - skip",
			line:    "      uses: actions/checkout@1234567890123456789012345678901234567890",
			wantDep: false,
		},
		{
			name:    "not a uses line",
			line:    "      run: echo hello",
			wantDep: false,
		},
		{
			name:    "docker reference - skip",
			line:    "      uses: docker://alpine:3.8",
			wantDep: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := s.parseLine(tt.line, 1, "test.yaml")

			if tt.wantDep && dep == nil {
				t.Fatal("expected dependency but got nil")
			}
			if !tt.wantDep && dep != nil {
				t.Fatalf("expected no dependency but got %+v", dep)
			}

			if !tt.wantDep {
				return
			}

			if dep.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", dep.Name, tt.wantName)
			}

			if dep.Version.Original() != tt.wantVersion {
				t.Errorf("Version = %q, want %q", dep.Version.Original(), tt.wantVersion)
			}

			if dep.PinnedBySHA != tt.wantPinnedSHA {
				t.Errorf("PinnedBySHA = %v, want %v", dep.PinnedBySHA, tt.wantPinnedSHA)
			}
		})
	}
}

func TestExtract(t *testing.T) {
	// Create a temporary workflow file
	tmpDir := t.TempDir()
	workflowContent := `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4.1.1
      - uses: actions/setup-go@0aaccfd150d50ccaeb58ebd88d36e91967a5f35b # v5.4.0
      - uses: actions/cache/save@v3.2.1
      - run: go build ./...
`
	workflowPath := filepath.Join(tmpDir, "ci.yaml")
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{paths: []string{workflowPath}}
	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d", len(deps))
	}

	// Check first dep (actions/checkout)
	if deps[0].Name != "actions/checkout" {
		t.Errorf("deps[0].Name = %q, want %q", deps[0].Name, "actions/checkout")
	}
	if deps[0].Version.Original() != "v4.1.1" {
		t.Errorf("deps[0].Version = %q, want %q", deps[0].Version.Original(), "v4.1.1")
	}

	// Check second dep (actions/setup-go with SHA)
	if deps[1].Name != "actions/setup-go" {
		t.Errorf("deps[1].Name = %q, want %q", deps[1].Name, "actions/setup-go")
	}
	if !deps[1].PinnedBySHA {
		t.Errorf("deps[1] should be pinned by SHA")
	}

	// Check third dep (actions/cache with subpath)
	if deps[2].Name != "actions/cache" {
		t.Errorf("deps[2].Name = %q, want %q", deps[2].Name, "actions/cache")
	}
}

func TestApply(t *testing.T) {
	tmpDir := t.TempDir()

	// Test case 1: Simple version update
	t.Run("simple version update", func(t *testing.T) {
		content := `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4.1.1
`
		workflowPath := filepath.Join(tmpDir, "ci1.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		s := &Source{paths: []string{workflowPath}}

		v := mustParse("v4.1.1")
		newV := mustParse("v4.2.0")

		changes := []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:       "actions/checkout",
					Version:    v,
					SourceKind: Kind,
					FilePath:   workflowPath,
					Locator:    "line:7:actions/checkout",
					Line:       7,
				},
				CurrentVersion: v,
				TargetVersion:  newV,
			},
		}

		if err := s.Apply(context.Background(), changes); err != nil {
			t.Fatal(err)
		}

		result, _ := os.ReadFile(workflowPath)
		if !strings.Contains(string(result), "actions/checkout@v4.2.0") {
			t.Errorf("expected v4.2.0 in result:\n%s", result)
		}
	})

	// Test case 2: SHA pin update
	t.Run("SHA pin update", func(t *testing.T) {
		content := `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@1234567890123456789012345678901234567890 # v4.1.1
`
		workflowPath := filepath.Join(tmpDir, "ci2.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		s := &Source{paths: []string{workflowPath}}

		v := mustParse("v4.1.1")
		newV := mustParse("v4.2.0")

		changes := []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:        "actions/checkout",
					Version:     v,
					SourceKind:  Kind,
					FilePath:    workflowPath,
					Locator:     "line:7:actions/checkout",
					Line:        7,
					PinnedBySHA: true,
				},
				CurrentVersion: v,
				TargetVersion:  newV,
				SelectedCandidate: &model.Candidate{
					Version:   newV,
					CommitSHA: "abcdef1234567890abcdef1234567890abcdef12",
				},
			},
		}

		if err := s.Apply(context.Background(), changes); err != nil {
			t.Fatal(err)
		}

		result, _ := os.ReadFile(workflowPath)
		expected := "actions/checkout@abcdef1234567890abcdef1234567890abcdef12 # v4.2.0"
		if !strings.Contains(string(result), expected) {
			t.Errorf("expected %q in result:\n%s", expected, result)
		}
	})

	// Test case 3: Subpath action update
	t.Run("subpath action update", func(t *testing.T) {
		content := `name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/cache/save@v3.2.1
`
		workflowPath := filepath.Join(tmpDir, "ci3.yaml")
		if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		s := &Source{paths: []string{workflowPath}}

		v := mustParse("v3.2.1")
		newV := mustParse("v3.3.0")

		changes := []model.PlannedChange{
			{
				Dependency: model.Dependency{
					Name:       "actions/cache",
					Version:    v,
					SourceKind: Kind,
					FilePath:   workflowPath,
					Locator:    "line:7:actions/cache/save",
					Line:       7,
				},
				CurrentVersion: v,
				TargetVersion:  newV,
			},
		}

		if err := s.Apply(context.Background(), changes); err != nil {
			t.Fatal(err)
		}

		result, _ := os.ReadFile(workflowPath)
		expected := "actions/cache/save@v3.3.0"
		if !strings.Contains(string(result), expected) {
			t.Errorf("expected %q in result:\n%s", expected, result)
		}
	})
}

func TestParseTarget(t *testing.T) {
	tests := []struct {
		target   string
		wantRepo string
	}{
		{"actions/checkout", "actions/checkout"},
		{"actions/cache/save", "actions/cache"},
		{"github/codeql-action/init", "github/codeql-action"},
		{"owner/repo/deep/path", "owner/repo"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if repo := parseTarget(tt.target); repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}
