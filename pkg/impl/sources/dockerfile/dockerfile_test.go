package dockerfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"
	dfparser "github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver/containerimage"
)

func TestExtract_SimpleFromLine(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24.1
RUN go build -o /app .
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	dep := deps[0]
	if dep.Name != "go" {
		t.Errorf("expected Name='go', got %q", dep.Name)
	}
	if dep.Version.String() != "1.24.1" {
		t.Errorf("expected Version='1.24.1', got %q", dep.Version.String())
	}
	if dep.SourceKind != sourceKind {
		t.Errorf("expected SourceKind=%q, got %q", sourceKind, dep.SourceKind)
	}
	if dep.Metadata[MetadataImageName] != "golang" {
		t.Errorf("expected image_name='golang', got %q", dep.Metadata[MetadataImageName])
	}
	if dep.Metadata[MetadataRepository] != "library/golang" {
		t.Errorf("expected repository='library/golang', got %q", dep.Metadata[MetadataRepository])
	}
}

func TestExtract_WithAlias(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24 AS builder
RUN go build -o /app .

FROM debian:12.10
COPY --from=builder /app /app
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}

	// First dependency: golang
	if deps[0].Name != "go" {
		t.Errorf("expected Name='go', got %q", deps[0].Name)
	}
	if deps[0].Version.String() != "1.24.0" {
		t.Errorf("expected Version='1.24.0', got %q", deps[0].Version.String())
	}
	if deps[0].Metadata[MetadataAlias] != "builder" {
		t.Errorf("expected alias='builder', got %q", deps[0].Metadata[MetadataAlias])
	}

	// Second dependency: debian
	if deps[1].Name != "debian" {
		t.Errorf("expected Name='debian', got %q", deps[1].Name)
	}
	if deps[1].Version.String() != "12.10.0" {
		t.Errorf("expected Version='12.10.0', got %q", deps[1].Version.String())
	}
}

func TestExtract_WithPlatform(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM --platform=$BUILDPLATFORM golang:1.24.1 AS builder
RUN go build -o /app .
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	dep := deps[0]
	if dep.Metadata[MetadataPlatform] != "$BUILDPLATFORM" {
		t.Errorf("expected platform='$BUILDPLATFORM', got %q", dep.Metadata[MetadataPlatform])
	}
}

func TestExtract_WithFullImagePath(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM docker.io/library/debian:12.10
RUN apt-get update
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	dep := deps[0]
	if dep.Name != "debian" {
		t.Errorf("expected Name='debian', got %q", dep.Name)
	}
	if dep.Metadata[MetadataRepository] != "library/debian" {
		t.Errorf("expected repository='library/debian', got %q", dep.Metadata[MetadataRepository])
	}
}

func TestExtract_ImageMappingApplied(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM myregistry.io/myimage:1.2.3
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths: []string{dockerfile},
		mappings: buildMappingLookup([]ImageMapping{
			{Match: "myregistry.io/myimage", DependencyName: "my-app", VersionScheme: "semver"},
		}),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	dep := deps[0]
	if dep.Name != "my-app" {
		t.Errorf("expected Name='my-app', got %q", dep.Name)
	}
	if dep.Metadata[MetadataRepository] != "myimage" {
		t.Errorf("expected repository='myimage', got %q", dep.Metadata[MetadataRepository])
	}
	if dep.Metadata[MetadataRegistryHost] != "myregistry.io" {
		t.Errorf("expected registry_host='myregistry.io', got %q", dep.Metadata[MetadataRegistryHost])
	}
}

func TestExtract_SkipsNonSemverTags(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24-alpine
FROM golang:bookworm
FROM golang:latest
FROM golang:1.24.1
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Only 1.24.1 should be extracted (pure semver)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency (only pure semver), got %d", len(deps))
	}

	if deps[0].Version.String() != "1.24.1" {
		t.Errorf("expected Version='1.24.1', got %q", deps[0].Version.String())
	}
}

func TestExtract_SkipsNoTag(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang
FROM debian:12.10
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Only debian:12.10 should be extracted
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	if deps[0].Name != "debian" {
		t.Errorf("expected Name='debian', got %q", deps[0].Name)
	}
}

func TestExtract_VPrefixedTag(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM nginx:v1.25.4
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	deps, err := s.Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	dep := deps[0]
	if dep.Version.String() != "1.25.4" {
		t.Errorf("expected Version='1.25.4', got %q", dep.Version.String())
	}
	if dep.Metadata[MetadataTag] != "v1.25.4" {
		t.Errorf("expected tag='v1.25.4', got %q", dep.Metadata[MetadataTag])
	}
}

func TestApply_UpdatesTagOnly(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24.1 AS builder
RUN go build -o /app .

FROM debian:12.10
COPY --from=builder /app /app
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	targetVersion := semver.MustParse("1.24.2")

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.1"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "1.24.1",
				},
			},
			CurrentVersion: semver.MustParse("1.24.1"),
			TargetVersion:  targetVersion,
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM golang:1.24.2 AS builder
RUN go build -o /app .

FROM debian:12.10
COPY --from=builder /app /app
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_PreservesAlias(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM --platform=$BUILDPLATFORM golang:1.24.1 AS builder
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	targetVersion := semver.MustParse("1.24.2")

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.1"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "1.24.1",
				},
			},
			CurrentVersion: semver.MustParse("1.24.1"),
			TargetVersion:  targetVersion,
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM --platform=$BUILDPLATFORM golang:1.24.2 AS builder
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_PreservesVPrefixTag(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM nginx:v1.25.4
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	targetVersion := semver.MustParse("1.25.5")

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "nginx",
				Version:    semver.MustParse("1.25.4"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "v1.25.4",
				},
			},
			CurrentVersion: semver.MustParse("1.25.4"),
			TargetVersion:  targetVersion,
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM nginx:v1.25.5
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_PreservesPartialVersion(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24 AS builder
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	targetVersion := semver.MustParse("1.25.0")

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.0"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "1.24",
				},
			},
			CurrentVersion: semver.MustParse("1.24.0"),
			TargetVersion:  targetVersion,
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM golang:1.25 AS builder
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_SkipsNonDockerfileChanges(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24.1
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	// Change with different source kind should be skipped
	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.1"),
				SourceKind: "gomod", // Different source kind
				FilePath:   dockerfile,
				Locator:    "1",
			},
			CurrentVersion: semver.MustParse("1.24.1"),
			TargetVersion:  semver.MustParse("1.24.2"),
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	// File should remain unchanged
	if string(data) != content {
		t.Errorf("file should not have changed, but got:\n%s", string(data))
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
		// Invalid cases
		{"1.24-alpine", ""},
		{"bookworm", ""},
		{"latest", ""},
		{"1.24.1-bookworm", ""},
		{".1.2.3", ""},
		{"1.2.3.", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := containerimage.NormalizeTag(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeTag(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseFROMLine(t *testing.T) {
	tests := []struct {
		line     string
		expected *fromLine
	}{
		{
			line: "FROM golang:1.24.1",
			expected: &fromLine{
				lineNum:   1,
				original:  "FROM golang:1.24.1",
				imageName: "golang",
				tag:       "1.24.1",
			},
		},
		{
			line: "FROM golang:1.24 AS builder",
			expected: &fromLine{
				lineNum:   1,
				original:  "FROM golang:1.24 AS builder",
				imageName: "golang",
				tag:       "1.24",
				alias:     "builder",
			},
		},
		{
			line: "FROM --platform=$BUILDPLATFORM golang:1.24.1 AS builder",
			expected: &fromLine{
				lineNum:   1,
				original:  "FROM --platform=$BUILDPLATFORM golang:1.24.1 AS builder",
				platform:  "$BUILDPLATFORM",
				imageName: "golang",
				tag:       "1.24.1",
				alias:     "builder",
			},
		},
		{
			line: "FROM docker.io/library/debian:12.10",
			expected: &fromLine{
				lineNum:             1,
				original:            "FROM docker.io/library/debian:12.10",
				imageName:           "debian",
				normalizedImageName: "docker.io/library/debian",
				tag:                 "12.10",
				repository:          "library/debian",
				registryHost:        "docker.io",
			},
		},
		{
			line:     "FROM golang",
			expected: nil,
		},
		{
			line:     "RUN apt-get update",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result, err := parseTestFROMLine(tt.line)
			if err != nil {
				t.Fatalf("parseTestFROMLine() error = %v", err)
			}
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected %+v, got nil", tt.expected)
			}
			if result.imageName != tt.expected.imageName {
				t.Errorf("imageName: got %q, want %q", result.imageName, tt.expected.imageName)
			}
			if result.tag != tt.expected.tag {
				t.Errorf("tag: got %q, want %q", result.tag, tt.expected.tag)
			}
			if result.alias != tt.expected.alias {
				t.Errorf("alias: got %q, want %q", result.alias, tt.expected.alias)
			}
			if result.platform != tt.expected.platform {
				t.Errorf("platform: got %q, want %q", result.platform, tt.expected.platform)
			}
			if tt.expected.normalizedImageName != "" && result.normalizedImageName != tt.expected.normalizedImageName {
				t.Errorf("normalizedImageName: got %q, want %q", result.normalizedImageName, tt.expected.normalizedImageName)
			}
			if tt.expected.repository != "" && result.repository != tt.expected.repository {
				t.Errorf("repository: got %q, want %q", result.repository, tt.expected.repository)
			}
			if tt.expected.registryHost != "" && result.registryHost != tt.expected.registryHost {
				t.Errorf("registryHost: got %q, want %q", result.registryHost, tt.expected.registryHost)
			}
		})
	}
}

func parseTestFROMLine(line string) (*fromLine, error) {
	result, err := dfparser.Parse(strings.NewReader(line))
	if err != nil {
		return nil, err
	}
	if len(result.AST.Children) == 0 {
		return nil, nil
	}
	return parseFROMNode(result.AST.Children[0])
}

func TestParseFROMLine_NormalizesReferenceMetadata(t *testing.T) {
	tests := []struct {
		line               string
		wantImageName      string
		wantNormalizedName string
		wantRegistryHost   string
		wantRepository     string
	}{
		{
			line:               "FROM golang:1.24",
			wantImageName:      "golang",
			wantNormalizedName: "docker.io/library/golang",
			wantRegistryHost:   "docker.io",
			wantRepository:     "library/golang",
		},
		{
			line:               "FROM docker.io/library/golang:1.24",
			wantImageName:      "golang",
			wantNormalizedName: "docker.io/library/golang",
			wantRegistryHost:   "docker.io",
			wantRepository:     "library/golang",
		},
		{
			line:               "FROM ghcr.io/org/app:v1.2.3",
			wantImageName:      "ghcr.io/org/app",
			wantNormalizedName: "ghcr.io/org/app",
			wantRegistryHost:   "ghcr.io",
			wantRepository:     "org/app",
		},
		{
			line:               "FROM localhost:5000/foo/bar:1.0.0",
			wantImageName:      "localhost:5000/foo/bar",
			wantNormalizedName: "localhost:5000/foo/bar",
			wantRegistryHost:   "localhost:5000",
			wantRepository:     "foo/bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result, err := parseTestFROMLine(tt.line)
			if err != nil {
				t.Fatalf("parseTestFROMLine() error = %v", err)
			}
			if result == nil {
				t.Fatal("expected parsed FROM line, got nil")
			}
			if result.imageName != tt.wantImageName {
				t.Errorf("imageName = %q, want %q", result.imageName, tt.wantImageName)
			}
			if result.normalizedImageName != tt.wantNormalizedName {
				t.Errorf("normalizedImageName = %q, want %q", result.normalizedImageName, tt.wantNormalizedName)
			}
			if result.registryHost != tt.wantRegistryHost {
				t.Errorf("registryHost = %q, want %q", result.registryHost, tt.wantRegistryHost)
			}
			if result.repository != tt.wantRepository {
				t.Errorf("repository = %q, want %q", result.repository, tt.wantRepository)
			}
		})
	}
}

func TestLookupMapping_PrefersFamiliarThenCanonicalName(t *testing.T) {
	s := &Source{
		mappings: buildMappingLookup(nil),
	}

	ref, err := name.ParseReference("docker.io/library/golang:1.24")
	if err != nil {
		t.Fatalf("ParseReference() error = %v", err)
	}

	familiarName := extractFamiliarName(ref)
	canonicalName := ref.Context().Name()

	mapping := s.lookupMapping(familiarName, canonicalName)
	if mapping.DependencyName != "go" {
		t.Errorf("DependencyName = %q, want go", mapping.DependencyName)
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
			wantErr: true, // file_paths required
		},
		{
			name: "valid config",
			config: map[string]interface{}{
				"file_paths": []interface{}{"Dockerfile"},
			},
			wantErr: false,
		},
		{
			name: "empty file_paths",
			config: map[string]interface{}{
				"file_paths": []interface{}{},
			},
			wantErr: true,
		},
		{
			name: "valid config with mappings",
			config: map[string]interface{}{
				"file_paths": []interface{}{"Dockerfile"},
				"image_mappings": []interface{}{
					map[string]interface{}{
						"match":           "myimage",
						"dependency_name": "my-dep",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "mapping missing match",
			config: map[string]interface{}{
				"file_paths": []interface{}{"Dockerfile"},
				"image_mappings": []interface{}{
					map[string]interface{}{
						"dependency_name": "my-dep",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "mapping missing dependency_name",
			config: map[string]interface{}{
				"file_paths": []interface{}{"Dockerfile"},
				"image_mappings": []interface{}{
					map[string]interface{}{
						"match": "myimage",
					},
				},
			},
			wantErr: true,
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
