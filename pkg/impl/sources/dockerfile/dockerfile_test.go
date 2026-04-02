package dockerfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	dfparser "github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver/containerimage"
)

// --- Extract Tests ---

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
	if dep.Metadata[MetadataImageName] != "golang" {
		t.Errorf("expected image_name='golang', got %q", dep.Metadata[MetadataImageName])
	}
	if dep.Metadata[MetadataRepository] != "library/golang" {
		t.Errorf("expected repository='library/golang', got %q", dep.Metadata[MetadataRepository])
	}
}

func TestExtract_DockerHubWithFullPath(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM docker.io/library/golang:1.24
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
	if dep.Metadata[MetadataRegistryHost] != "docker.io" {
		t.Errorf("expected registry_host='docker.io', got %q", dep.Metadata[MetadataRegistryHost])
	}
	if dep.Metadata[MetadataRepository] != "library/golang" {
		t.Errorf("expected repository='library/golang', got %q", dep.Metadata[MetadataRepository])
	}
}

func TestExtract_GHCRRegistry(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM ghcr.io/org/app:v1.2.3
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
	if dep.Name != "ghcr.io/org/app" {
		t.Errorf("expected Name='ghcr.io/org/app', got %q", dep.Name)
	}
	if dep.Version.String() != "1.2.3" {
		t.Errorf("expected Version='1.2.3', got %q", dep.Version.String())
	}
	if dep.Metadata[MetadataTag] != "v1.2.3" {
		t.Errorf("expected tag='v1.2.3', got %q", dep.Metadata[MetadataTag])
	}
	if dep.Metadata[MetadataRegistryHost] != "ghcr.io" {
		t.Errorf("expected registry_host='ghcr.io', got %q", dep.Metadata[MetadataRegistryHost])
	}
}

func TestExtract_LocalhostRegistryWithPort(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM localhost:5000/app:1.2.3
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
	if dep.Name != "localhost:5000/app" {
		t.Errorf("expected Name='localhost:5000/app', got %q", dep.Name)
	}
	if dep.Version.String() != "1.2.3" {
		t.Errorf("expected Version='1.2.3', got %q", dep.Version.String())
	}
	if dep.Metadata[MetadataRegistryHost] != "localhost:5000" {
		t.Errorf("expected registry_host='localhost:5000', got %q", dep.Metadata[MetadataRegistryHost])
	}
}

func TestExtract_WithPlatformAndAlias(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM --platform=linux/amd64 golang:1.24 AS builder
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
	if dep.Metadata[MetadataPlatform] != "linux/amd64" {
		t.Errorf("expected platform='linux/amd64', got %q", dep.Metadata[MetadataPlatform])
	}
	if dep.Metadata[MetadataAlias] != "builder" {
		t.Errorf("expected alias='builder', got %q", dep.Metadata[MetadataAlias])
	}
}

func TestExtract_WithVariablePlatform(t *testing.T) {
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
			{Match: "myregistry.io/myimage", DependencyName: "my-app"},
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
}

// --- Skip Tests ---

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

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	if deps[0].Name != "debian" {
		t.Errorf("expected Name='debian', got %q", deps[0].Name)
	}
}

func TestExtract_SuffixTags(t *testing.T) {
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

	// Now we extract both suffix tags and pure semver tags
	// 1.24-alpine (suffix=alpine) and 1.24.1 (no suffix)
	// bookworm and latest are still skipped (non-semver base)
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies (suffix + pure semver), got %d", len(deps))
	}

	// First should be 1.24-alpine
	if deps[0].Version.String() != "1.24.0" {
		t.Errorf("expected first Version='1.24.0', got %q", deps[0].Version.String())
	}
	if deps[0].Metadata[MetadataTag] != "1.24-alpine" {
		t.Errorf("expected first tag='1.24-alpine', got %q", deps[0].Metadata[MetadataTag])
	}
	if deps[0].Metadata[MetadataTagSuffix] != "alpine" {
		t.Errorf("expected first tag_suffix='alpine', got %q", deps[0].Metadata[MetadataTagSuffix])
	}

	// Second should be 1.24.1
	if deps[1].Version.String() != "1.24.1" {
		t.Errorf("expected second Version='1.24.1', got %q", deps[1].Version.String())
	}
	if deps[1].Metadata[MetadataTagSuffix] != "" {
		t.Errorf("expected second tag_suffix='', got %q", deps[1].Metadata[MetadataTagSuffix])
	}
}

func TestExtract_SkipsScratch(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24 AS builder
RUN go build -o /app .

FROM scratch
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

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}

	if deps[0].Name != "go" {
		t.Errorf("expected Name='go', got %q", deps[0].Name)
	}
}

func TestExtract_SkipsStageReference(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24 AS builder
RUN go build -o /app .

FROM builder AS test
RUN go test ./...

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
		t.Fatalf("expected 2 dependencies (golang, debian), got %d", len(deps))
	}

	if deps[0].Name != "go" {
		t.Errorf("expected first dep Name='go', got %q", deps[0].Name)
	}
	if deps[1].Name != "debian" {
		t.Errorf("expected second dep Name='debian', got %q", deps[1].Name)
	}
}

func TestExtract_SkipsVariableInImage(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM ${BASE_IMAGE}
RUN echo "hello"
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

	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies (variable image skipped), got %d", len(deps))
	}
}

func TestExtract_SkipsVariableInTag(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:${GO_VERSION}
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

	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies (variable tag skipped), got %d", len(deps))
	}
}

func TestExtract_SkipsMultiLineFROM(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM \
    golang:1.24 AS builder
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

	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency (debian only), got %d", len(deps))
	}

	if deps[0].Name != "debian" {
		t.Errorf("expected Name='debian', got %q", deps[0].Name)
	}
}

func TestExtract_SkipsDigestReference(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang@sha256:abc123def456
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

	// Digest reference should be skipped
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency (debian only), got %d", len(deps))
	}

	if deps[0].Name != "debian" {
		t.Errorf("expected Name='debian', got %q", deps[0].Name)
	}
}

// --- Apply Tests ---

func TestApply_UpdatesTag(t *testing.T) {
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

	expected := `FROM golang:1.24.2 AS builder
RUN go build -o /app .

FROM debian:12.10
COPY --from=builder /app /app
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
			TargetVersion:  semver.MustParse("1.25.5"),
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
			TargetVersion:  semver.MustParse("1.25.0"),
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

func TestApply_LocalhostRegistryWithPort(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM localhost:5000/myapp:1.2.3
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "localhost:5000/myapp",
				Version:    semver.MustParse("1.2.3"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "1.2.3",
				},
			},
			CurrentVersion: semver.MustParse("1.2.3"),
			TargetVersion:  semver.MustParse("1.2.4"),
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM localhost:5000/myapp:1.2.4
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_GhcrRegistry(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM ghcr.io/org/app:v1.2.3
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "ghcr.io/org/app",
				Version:    semver.MustParse("1.2.3"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag: "v1.2.3",
				},
			},
			CurrentVersion: semver.MustParse("1.2.3"),
			TargetVersion:  semver.MustParse("1.2.4"),
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM ghcr.io/org/app:v1.2.4
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_WithPlatformAndAlias(t *testing.T) {
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

	expected := `FROM --platform=$BUILDPLATFORM golang:1.24.2 AS builder
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

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.1"),
				SourceKind: "gomod",
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

	if string(data) != content {
		t.Errorf("file should not have changed, but got:\n%s", string(data))
	}
}

func TestApply_UsesRawTagFromCandidate(t *testing.T) {
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")

	content := `FROM golang:1.24-alpine AS builder
RUN go build -o /app .
`
	if err := os.WriteFile(dockerfile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := &Source{
		paths:    []string{dockerfile},
		mappings: buildMappingLookup(nil),
	}

	changes := []model.PlannedChange{
		{
			Dependency: model.Dependency{
				Name:       "go",
				Version:    semver.MustParse("1.24.0"),
				SourceKind: sourceKind,
				FilePath:   dockerfile,
				Locator:    "1",
				Metadata: map[string]string{
					MetadataTag:       "1.24-alpine",
					MetadataTagSuffix: "alpine",
				},
			},
			CurrentVersion: semver.MustParse("1.24.0"),
			TargetVersion:  semver.MustParse("1.25.0"),
			SelectedCandidate: &model.Candidate{
				Version: semver.MustParse("1.25.0"),
				Metadata: map[string]string{
					"tag":        "1.25-alpine",
					"tag_suffix": "alpine",
				},
			},
		},
	}

	if err := s.Apply(context.Background(), changes); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}

	expected := `FROM golang:1.25-alpine AS builder
RUN go build -o /app .
`
	if string(data) != expected {
		t.Errorf("unexpected output:\n%s\nexpected:\n%s", string(data), expected)
	}
}

func TestApply_FallbackToFormatTag(t *testing.T) {
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

	// No SelectedCandidate - should fall back to formatTag
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
			CurrentVersion:    semver.MustParse("1.24.0"),
			TargetVersion:     semver.MustParse("1.25.0"),
			SelectedCandidate: nil, // No candidate metadata
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

// --- Unit Tests ---

func TestParseStage(t *testing.T) {
	tests := []struct {
		line     string
		expected *fromLine
	}{
		{
			line: "FROM golang:1.24.1",
			expected: &fromLine{
				familiarName: "golang",
				tag:          "1.24.1",
			},
		},
		{
			line: "FROM golang:1.24 AS builder",
			expected: &fromLine{
				familiarName: "golang",
				tag:          "1.24",
				alias:        "builder",
			},
		},
		{
			line: "FROM --platform=linux/amd64 golang:1.24.1 AS builder",
			expected: &fromLine{
				platform:     "linux/amd64",
				familiarName: "golang",
				tag:          "1.24.1",
				alias:        "builder",
			},
		},
		{
			line: "FROM docker.io/library/debian:12.10",
			expected: &fromLine{
				familiarName: "debian",
				tag:          "12.10",
				repository:   "library/debian",
				registryHost: "docker.io",
			},
		},
		{
			line:     "FROM golang",
			expected: nil,
		},
		{
			line:     "FROM ${BASE_IMAGE}",
			expected: nil,
		},
		{
			line:     "FROM golang:${GO_VERSION}",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := parseTestFROMLine(t, tt.line)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected non-nil, got nil")
			}
			if result.familiarName != tt.expected.familiarName {
				t.Errorf("familiarName: got %q, want %q", result.familiarName, tt.expected.familiarName)
			}
			if result.tag != tt.expected.tag {
				t.Errorf("tag: got %q, want %q", result.tag, tt.expected.tag)
			}
			if result.alias != tt.expected.alias {
				t.Errorf("alias: got %q, want %q", result.alias, tt.expected.alias)
			}
			if tt.expected.platform != "" && result.platform != tt.expected.platform {
				t.Errorf("platform: got %q, want %q", result.platform, tt.expected.platform)
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

func parseTestFROMLine(t *testing.T, line string) *fromLine {
	t.Helper()
	result, err := dfparser.Parse(strings.NewReader(line))
	if err != nil {
		t.Fatalf("dfparser.Parse() error = %v", err)
	}
	stages, _, err := instructions.Parse(result.AST, nil)
	if err != nil {
		t.Fatalf("instructions.Parse() error = %v", err)
	}
	if len(stages) == 0 {
		return nil
	}
	return parseStage(stages[0])
}

func TestReplaceTag(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		currentTag string
		newTag     string
		expected   string
	}{
		{
			name:       "simple",
			line:       "FROM golang:1.24.1",
			currentTag: "1.24.1",
			newTag:     "1.24.2",
			expected:   "FROM golang:1.24.2",
		},
		{
			name:       "with alias",
			line:       "FROM golang:1.24.1 AS builder",
			currentTag: "1.24.1",
			newTag:     "1.24.2",
			expected:   "FROM golang:1.24.2 AS builder",
		},
		{
			name:       "registry with port",
			line:       "FROM localhost:5000/app:1.2.3",
			currentTag: "1.2.3",
			newTag:     "1.2.4",
			expected:   "FROM localhost:5000/app:1.2.4",
		},
		{
			name:       "with platform",
			line:       "FROM --platform=linux/amd64 golang:1.24",
			currentTag: "1.24",
			newTag:     "1.25",
			expected:   "FROM --platform=linux/amd64 golang:1.25",
		},
		{
			name:       "invalid line",
			line:       "RUN echo hello",
			currentTag: "1.24",
			newTag:     "1.25",
			expected:   "RUN echo hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceTag(tt.line, tt.currentTag, tt.newTag)
			if result != tt.expected {
				t.Errorf("replaceTag() = %q, want %q", result, tt.expected)
			}
		})
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
		{"12.10", "12.10.0"},
		{"1.24-alpine", "1.24.0"}, // Now returns base version
		{"bookworm", ""},
		{"latest", ""},
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

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
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
