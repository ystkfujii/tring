package dockerfile

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	dfparser "github.com/moby/buildkit/frontend/dockerfile/parser"
	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/resolver/containerimage"
	"github.com/ystkfujii/tring/pkg/impl/sources"
)

const sourceKind = "dockerfile"

// Metadata keys
const (
	MetadataImageName    = "image_name"
	MetadataRawRef       = "raw_ref"
	MetadataTag          = "tag"
	MetadataAlias        = "alias"
	MetadataPlatform     = "platform"
	MetadataRepository   = "repository"
	MetadataRegistryHost = "registry_host"
	MetadataLine         = "line"
)

// Default image mappings
var defaultImageMappings = []ImageMapping{
	{Match: "golang", DependencyName: "go", VersionScheme: "semver"},
	{Match: "debian", DependencyName: "debian", VersionScheme: "semver"},
}

func init() {
	sources.Register(sourceKind, &Factory{})
}

// Factory creates dockerfile sources.
type Factory struct{}

// Kind returns the source type.
func (f *Factory) Kind() string {
	return sourceKind
}

// Create creates a new dockerfile source from configuration map.
func (f *Factory) Create(config map[string]interface{}, basePath string) (model.Source, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse dockerfile config: %w", err)
	}

	// Resolve paths relative to basePath
	paths := make([]string, len(cfg.FilePaths))
	for i, p := range cfg.FilePaths {
		if filepath.IsAbs(p) {
			paths[i] = p
		} else {
			paths[i] = filepath.Join(basePath, p)
		}
	}

	// Build image mapping lookup (user mappings + defaults)
	mappings := buildMappingLookup(cfg.ImageMappings)

	return &Source{
		paths:    paths,
		mappings: mappings,
	}, nil
}

func decodeConfig(raw map[string]interface{}, cfg *Config) error {
	if raw == nil {
		return nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// buildMappingLookup creates a mapping lookup with user mappings taking precedence over defaults.
func buildMappingLookup(userMappings []ImageMapping) map[string]ImageMapping {
	result := make(map[string]ImageMapping)

	// Add default mappings first
	for _, m := range defaultImageMappings {
		result[m.Match] = m
	}

	// User mappings override defaults
	for _, m := range userMappings {
		result[m.Match] = m
	}

	return result
}

// Source extracts and updates dependencies from Dockerfiles.
type Source struct {
	paths    []string
	mappings map[string]ImageMapping
}

// Ensure Source implements model.Source
var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// fromLine represents a parsed FROM instruction.
type fromLine struct {
	lineNum             int
	original            string
	platform            string
	imageName           string
	normalizedImageName string
	tag                 string
	alias               string
	repository          string
	registryHost        string
	version             *semver.Version
}

// FROM instruction regex
// Matches: FROM [--platform=...] image[:tag] [AS alias]
var fromRegex = regexp.MustCompile(`(?i)^FROM\s+(?:--platform=(\S+)\s+)?([^:\s]+)(?::([^\s]+))?\s*(?:AS\s+(\S+))?`)

// Extract extracts dependencies from all configured Dockerfiles.
func (s *Source) Extract(ctx context.Context) ([]model.Dependency, error) {
	var deps []model.Dependency

	for _, path := range s.paths {
		fileDeps, err := s.extractFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to extract from %s: %w", path, err)
		}
		deps = append(deps, fileDeps...)
	}

	return deps, nil
}

func (s *Source) extractFromFile(path string) ([]model.Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	result, err := dfparser.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	var deps []model.Dependency
	for _, node := range result.AST.Children {
		fromInfo, err := parseFROMNode(node)
		if err != nil {
			return nil, fmt.Errorf("failed to parse FROM instruction at line %d: %w", node.StartLine, err)
		}
		if fromInfo == nil {
			continue
		}

		mapping := s.lookupMapping(fromInfo.imageName, fromInfo.normalizedImageName)

		deps = append(deps, model.Dependency{
			Name:       mapping.DependencyName,
			Version:    fromInfo.version,
			SourceKind: sourceKind,
			FilePath:   path,
			Locator:    fmt.Sprintf("%d", fromInfo.lineNum),
			Metadata: map[string]string{
				MetadataImageName:    fromInfo.imageName,
				MetadataRawRef:       fromInfo.original,
				MetadataTag:          fromInfo.tag,
				MetadataAlias:        fromInfo.alias,
				MetadataPlatform:     fromInfo.platform,
				MetadataRepository:   fromInfo.repository,
				MetadataRegistryHost: fromInfo.registryHost,
				MetadataLine:         fmt.Sprintf("%d", fromInfo.lineNum),
			},
		})
	}

	return deps, nil
}

// parseFROMNode parses a FROM instruction node from the Dockerfile AST.
func parseFROMNode(node *dfparser.Node) (*fromLine, error) {
	if node == nil || !strings.EqualFold(node.Value, command.From) {
		return nil, nil
	}

	args := collectNodeValues(node.Next)
	if len(args) == 0 {
		return nil, nil
	}

	ref, tag, err := parseTaggedReference(args[0])
	if err != nil {
		return nil, err
	}
	if tag == "" {
		// No explicit tag, skip
		return nil, nil
	}

	version, err := containerimage.ParseTag(tag)
	if err != nil {
		// Skip non-semver tags (e.g., "alpine", "latest", "bookworm")
		return nil, nil
	}

	return &fromLine{
		lineNum:             node.StartLine,
		original:            strings.TrimSpace(node.Original),
		platform:            extractPlatform(node.Flags),
		imageName:           extractFamiliarName(ref),
		normalizedImageName: extractNormalizedName(ref),
		tag:                 tag,
		alias:               extractAlias(args[1:]),
		repository:          extractRepository(ref),
		registryHost:        extractRegistryHost(ref),
		version:             version,
	}, nil
}

func collectNodeValues(node *dfparser.Node) []string {
	var values []string
	for ; node != nil; node = node.Next {
		if node.Value != "" {
			values = append(values, node.Value)
		}
	}
	return values
}

func extractPlatform(flags []string) string {
	for _, flag := range flags {
		if value, ok := strings.CutPrefix(flag, "--platform="); ok {
			return value
		}
		if value, ok := strings.CutPrefix(flag, "platform="); ok {
			return value
		}
	}
	return ""
}

func extractAlias(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if strings.EqualFold(args[i], "AS") {
			return args[i+1]
		}
	}
	return ""
}

// parseTaggedReference parses an image reference and returns the tag if present.
// Returns (ref, tag, nil) if the reference has an explicit tag.
// Returns (ref, "", nil) if the reference has no explicit tag.
// Returns (nil, "", err) if parsing fails.
func parseTaggedReference(imageRef string) (name.Reference, string, error) {
	trimmed := strings.TrimSpace(imageRef)

	// Check if the reference contains a digest (sha256:...)
	if strings.Contains(trimmed, "@sha256:") {
		// Digest references are not supported
		ref, err := name.ParseReference(trimmed)
		if err != nil {
			return nil, "", err
		}
		return ref, "", nil
	}

	// Check if the reference has an explicit tag
	// We need to distinguish "golang:1.24" from "golang" (no tag)
	// The tag part comes after the last ":" but we need to be careful about
	// registry ports like "localhost:5000/image:tag"
	hasExplicitTag := hasTag(trimmed)

	ref, err := name.ParseReference(trimmed)
	if err != nil {
		return nil, "", err
	}

	if !hasExplicitTag {
		return ref, "", nil
	}

	// Extract the tag
	if tag, ok := ref.(name.Tag); ok {
		return ref, tag.TagStr(), nil
	}

	return ref, "", nil
}

// hasTag checks if the image reference string contains an explicit tag.
// Handles cases like:
// - "golang:1.24" -> true
// - "golang" -> false
// - "localhost:5000/image:tag" -> true
// - "localhost:5000/image" -> false
func hasTag(imageRef string) bool {
	// Remove digest if present
	if idx := strings.Index(imageRef, "@"); idx != -1 {
		return false
	}

	// Find the last colon
	lastColon := strings.LastIndex(imageRef, ":")
	if lastColon == -1 {
		return false
	}

	// Check if the colon is part of a port (registry:port/image)
	// If there's a "/" after the colon, it's a port, not a tag
	afterColon := imageRef[lastColon+1:]
	if strings.Contains(afterColon, "/") {
		return false
	}

	return true
}

// extractFamiliarName extracts the "familiar" (short) image name from a reference.
// For Docker Hub official images: "index.docker.io/library/golang" -> "golang"
// For Docker Hub user images: "index.docker.io/user/image" -> "user/image"
// For other registries: "ghcr.io/org/app" -> "ghcr.io/org/app"
func extractFamiliarName(ref name.Reference) string {
	repo := ref.Context()
	registry := repo.RegistryStr()
	repoName := repo.RepositoryStr()

	// For Docker Hub official images, return just the image name
	if isDockerHubRegistry(registry) {
		if strings.HasPrefix(repoName, "library/") {
			return strings.TrimPrefix(repoName, "library/")
		}
		// Docker Hub user images without library/ prefix
		return repoName
	}

	// For other registries, include the registry in the name
	return registry + "/" + repoName
}

// isDockerHubRegistry checks if the registry is Docker Hub.
func isDockerHubRegistry(registry string) bool {
	switch registry {
	case "index.docker.io", "docker.io", "registry-1.docker.io":
		return true
	default:
		return false
	}
}

// extractRepository extracts the repository path from a reference.
// For Docker Hub: "library/golang" or "user/image"
// For other registries: "org/app"
func extractRepository(ref name.Reference) string {
	return ref.Context().RepositoryStr()
}

// extractNormalizedName extracts the normalized (canonical) name from a reference.
// Normalizes Docker Hub registry to "docker.io" for consistency with existing behavior.
func extractNormalizedName(ref name.Reference) string {
	name := ref.Context().Name()
	// Normalize index.docker.io to docker.io for consistency
	if strings.HasPrefix(name, "index.docker.io/") {
		return "docker.io/" + strings.TrimPrefix(name, "index.docker.io/")
	}
	return name
}

// extractRegistryHost extracts the registry host from a reference.
// Normalizes Docker Hub to "docker.io" for consistency with existing behavior.
func extractRegistryHost(ref name.Reference) string {
	registry := ref.Context().RegistryStr()
	// Normalize index.docker.io to docker.io for consistency
	if registry == "index.docker.io" {
		return "docker.io"
	}
	return registry
}

// lookupMapping finds the mapping for an image name.
func (s *Source) lookupMapping(imageNames ...string) ImageMapping {
	for _, imageName := range imageNames {
		if imageName == "" {
			continue
		}
		if mapping, ok := s.mappings[imageName]; ok {
			return mapping
		}
	}

	fallbackName := ""
	for _, imageName := range imageNames {
		if imageName != "" {
			fallbackName = imageName
			break
		}
	}

	return ImageMapping{
		Match:          fallbackName,
		DependencyName: fallbackName,
		VersionScheme:  "semver",
	}
}

// Apply applies the planned changes to the Dockerfiles.
func (s *Source) Apply(ctx context.Context, changes []model.PlannedChange) error {
	// Group changes by file
	changesByFile := make(map[string][]model.PlannedChange)
	for _, c := range changes {
		if c.IsSkipped() || !c.HasUpdate() {
			continue
		}
		if c.Dependency.SourceKind != sourceKind {
			continue
		}
		changesByFile[c.Dependency.FilePath] = append(changesByFile[c.Dependency.FilePath], c)
	}

	// Apply changes to each file
	for path, fileChanges := range changesByFile {
		if err := s.applyToFile(path, fileChanges); err != nil {
			return fmt.Errorf("failed to apply changes to %s: %w", path, err)
		}
	}

	return nil
}

func (s *Source) applyToFile(path string, changes []model.PlannedChange) error {
	// Build a map of line number -> new version
	updatesByLine := make(map[int]string)
	for _, c := range changes {
		lineNum := 0
		if _, err := fmt.Sscanf(c.Dependency.Locator, "%d", &lineNum); err != nil {
			return fmt.Errorf("invalid locator %q: %w", c.Dependency.Locator, err)
		}
		updatesByLine[lineNum] = formatTag(c.TargetVersion, c.Dependency.Metadata[MetadataTag])
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var output strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if newTag, ok := updatesByLine[lineNum]; ok {
			// Update this FROM line
			newLine := updateFROMLineTag(line, newTag)
			output.WriteString(newLine)
		} else {
			output.WriteString(line)
		}
		output.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	result := output.String()
	// Preserve original file's trailing newline behavior
	if !strings.HasSuffix(string(data), "\n") && strings.HasSuffix(result, "\n") {
		result = result[:len(result)-1]
	}

	return os.WriteFile(path, []byte(result), 0644)
}

// formatTag formats the target version as a Docker tag.
// Preserves the original tag format (with or without 'v' prefix, etc.)
func formatTag(version *semver.Version, originalTag string) string {
	// If original had 'v' prefix, keep it
	if strings.HasPrefix(originalTag, "v") {
		return "v" + version.String()
	}

	// For partial versions (1 or 1.2), try to match the format
	parts := strings.Split(strings.TrimPrefix(originalTag, "v"), ".")
	switch len(parts) {
	case 1:
		// Original was just major (e.g., "1")
		return fmt.Sprintf("%d", version.Major())
	case 2:
		// Original was major.minor (e.g., "1.2")
		return fmt.Sprintf("%d.%d", version.Major(), version.Minor())
	default:
		// Use full semver
		return version.String()
	}
}

// updateFROMLineTag replaces the tag in a FROM line while preserving everything else.
func updateFROMLineTag(line string, newTag string) string {
	matches := fromRegex.FindStringSubmatchIndex(line)
	if matches == nil {
		return line
	}

	// matches[6] and matches[7] are the start and end of the tag capture group
	tagStart := matches[6]
	tagEnd := matches[7]

	if tagStart == -1 || tagEnd == -1 {
		// No tag in original, we shouldn't be here but handle gracefully
		return line
	}

	return line[:tagStart] + newTag + line[tagEnd:]
}
