package dockerfile

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
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
	{Match: "docker.io/library/golang", DependencyName: "go", VersionScheme: "semver"},
	{Match: "debian", DependencyName: "debian", VersionScheme: "semver"},
	{Match: "docker.io/library/debian", DependencyName: "debian", VersionScheme: "semver"},
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
	lineNum   int
	original  string
	platform  string
	imageName string
	tag       string
	alias     string
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
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var deps []model.Dependency
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		fromInfo := parseFROMLine(line, lineNum)
		if fromInfo == nil {
			continue
		}

		// Skip if no tag or tag is not semver-like
		if fromInfo.tag == "" {
			continue
		}

		// Try to parse the tag as semver
		version, err := containerimage.ParseTag(fromInfo.tag)
		if err != nil {
			// Skip non-semver tags (e.g., "alpine", "latest", "bookworm")
			continue
		}

		// Look up the dependency mapping
		mapping, repository := s.lookupMapping(fromInfo.imageName)

		// Extract registry host from image name
		registryHost := extractRegistryHost(fromInfo.imageName)

		deps = append(deps, model.Dependency{
			Name:       mapping.DependencyName,
			Version:    version,
			SourceKind: sourceKind,
			FilePath:   path,
			Locator:    fmt.Sprintf("%d", fromInfo.lineNum),
			Metadata: map[string]string{
				MetadataImageName:    fromInfo.imageName,
				MetadataRawRef:       fromInfo.original,
				MetadataTag:          fromInfo.tag,
				MetadataAlias:        fromInfo.alias,
				MetadataPlatform:     fromInfo.platform,
				MetadataRepository:   repository,
				MetadataRegistryHost: registryHost,
				MetadataLine:         fmt.Sprintf("%d", fromInfo.lineNum),
			},
		})
	}

	return deps, scanner.Err()
}

// parseFROMLine parses a FROM instruction line.
func parseFROMLine(line string, lineNum int) *fromLine {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}

	matches := fromRegex.FindStringSubmatch(trimmed)
	if matches == nil {
		return nil
	}

	return &fromLine{
		lineNum:   lineNum,
		original:  trimmed,
		platform:  matches[1],
		imageName: matches[2],
		tag:       matches[3],
		alias:     matches[4],
	}
}

// lookupMapping finds the mapping for an image name.
// Returns the mapping and the repository to use for resolution.
func (s *Source) lookupMapping(imageName string) (ImageMapping, string) {
	// Try exact match first
	if mapping, ok := s.mappings[imageName]; ok {
		return mapping, getRepository(imageName)
	}

	// Try with docker.io/library/ prefix for official images
	if !strings.Contains(imageName, "/") {
		fullName := "docker.io/library/" + imageName
		if mapping, ok := s.mappings[fullName]; ok {
			return mapping, getRepository(imageName)
		}
	}

	// No mapping found, use image name as dependency name
	return ImageMapping{
		Match:          imageName,
		DependencyName: imageName,
		VersionScheme:  "semver",
	}, getRepository(imageName)
}

// getRepository returns the repository name for Docker Hub API.
// For official images (no slash), returns "library/<image>".
// For other images, returns the image name as-is.
func getRepository(imageName string) string {
	// Strip docker.io prefix if present
	imageName = strings.TrimPrefix(imageName, "docker.io/")

	// Official images without namespace get "library/" prefix
	if !strings.Contains(imageName, "/") {
		return "library/" + imageName
	}
	return imageName
}

// extractRegistryHost extracts the registry host from an image name.
// Returns the registry host (e.g., "ghcr.io", "docker.io") or "docker.io" for Docker Hub images.
func extractRegistryHost(imageName string) string {
	// Check for known registry prefixes
	if strings.HasPrefix(imageName, "ghcr.io/") {
		return "ghcr.io"
	}
	if strings.HasPrefix(imageName, "docker.io/") {
		return "docker.io"
	}
	if strings.HasPrefix(imageName, "registry-1.docker.io/") {
		return "docker.io"
	}

	// Check if the first part contains a dot or colon (indicating a registry host)
	// e.g., "myregistry.io/myimage" or "localhost:5000/myimage"
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) == 2 {
		firstPart := parts[0]
		if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
			return firstPart
		}
	}

	// Default to Docker Hub for official images (no slash) or user images (user/repo)
	return "docker.io"
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
