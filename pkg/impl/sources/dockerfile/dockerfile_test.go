package dockerfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"

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
	if dep.Metadata[MetadataRepository] != "myregistry.io/myimage" {
		t.Errorf("expected repository='myregistry.io/myimage', got %q", dep.Metadata[MetadataRepository])
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
				lineNum:   1,
				original:  "FROM docker.io/library/debian:12.10",
				imageName: "docker.io/library/debian",
				tag:       "12.10",
			},
		},
		{
			line: "FROM golang",
			expected: &fromLine{
				lineNum:   1,
				original:  "FROM golang",
				imageName: "golang",
				tag:       "",
			},
		},
		{
			line:     "RUN apt-get update",
			expected: nil,
		},
		{
			line:     "# FROM golang:1.24",
			expected: nil,
		},
		{
			line:     "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := parseFROMLine(tt.line, 1)
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
		})
	}
}

func TestGetRepository(t *testing.T) {
	tests := []struct {
		imageName string
		expected  string
	}{
		{"golang", "library/golang"},
		{"debian", "library/debian"},
		{"nginx", "library/nginx"},
		{"myuser/myimage", "myuser/myimage"},
		{"docker.io/library/golang", "library/golang"},
		{"docker.io/myuser/myimage", "myuser/myimage"},
		{"gcr.io/project/image", "gcr.io/project/image"},
	}

	for _, tt := range tests {
		t.Run(tt.imageName, func(t *testing.T) {
			result := getRepository(tt.imageName)
			if result != tt.expected {
				t.Errorf("getRepository(%q) = %q, want %q", tt.imageName, result, tt.expected)
			}
		})
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

func TestExtractRegistryHost(t *testing.T) {
	tests := []struct {
		imageName string
		expected  string
	}{
		// GHCR
		{"ghcr.io/owner/repo", "ghcr.io"},
		// Docker Hub explicit
		{"docker.io/library/debian", "docker.io"},
		{"docker.io/myuser/myimage", "docker.io"},
		{"registry-1.docker.io/library/debian", "docker.io"},
		// Official images (no registry prefix) default to Docker Hub
		{"debian", "docker.io"},
		{"golang", "docker.io"},
		{"nginx", "docker.io"},
		// User images (no registry prefix) default to Docker Hub
		{"myuser/myimage", "docker.io"},
		// Custom registries with dots
		{"gcr.io/project/image", "gcr.io"},
		{"quay.io/myorg/myimage", "quay.io"},
		{"myregistry.example.com/myimage", "myregistry.example.com"},
		// Custom registries with port
		{"localhost:5000/myimage", "localhost:5000"},
		{"myregistry.io:8080/org/image", "myregistry.io:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.imageName, func(t *testing.T) {
			result := extractRegistryHost(tt.imageName)
			if result != tt.expected {
				t.Errorf("extractRegistryHost(%q) = %q, want %q", tt.imageName, result, tt.expected)
			}
		})
	}
}
