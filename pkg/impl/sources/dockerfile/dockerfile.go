package dockerfile

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/distribution/reference"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
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
	MetadataTagSuffix    = "tag_suffix"
	MetadataAlias        = "alias"
	MetadataPlatform     = "platform"
	MetadataRepository   = "repository"
	MetadataRegistryHost = "registry_host"
	MetadataLine         = "line"
)

// Default image mappings: only non-obvious mappings (e.g., "golang" -> "go").
var defaultImageMappings = []ImageMapping{
	{Match: "golang", DependencyName: "go"},
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

	paths := make([]string, len(cfg.FilePaths))
	for i, p := range cfg.FilePaths {
		if filepath.IsAbs(p) {
			paths[i] = p
		} else {
			paths[i] = filepath.Join(basePath, p)
		}
	}

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

func buildMappingLookup(userMappings []ImageMapping) map[string]ImageMapping {
	result := make(map[string]ImageMapping)
	for _, m := range defaultImageMappings {
		result[m.Match] = m
	}
	for _, m := range userMappings {
		result[m.Match] = m
	}
	return result
}

// Source extracts and updates dependencies from Dockerfiles.
// Supports single-line FROM with semver tags only.
type Source struct {
	paths    []string
	mappings map[string]ImageMapping
}

var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// fromLine represents a parsed FROM instruction.
type fromLine struct {
	lineNum      int
	sourceCode   string
	platform     string
	familiarName string
	tag          string
	tagSuffix    string
	alias        string
	repository   string
	registryHost string
	version      *semver.Version
}

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

	stages, _, err := instructions.Parse(result.AST, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Dockerfile instructions: %w", err)
	}

	var deps []model.Dependency
	for _, stage := range stages {
		fromInfo := parseStage(stage)
		if fromInfo == nil {
			continue
		}

		mapping := s.lookupMapping(fromInfo.familiarName)
		deps = append(deps, model.Dependency{
			Name:       mapping.DependencyName,
			Version:    fromInfo.version,
			SourceKind: sourceKind,
			FilePath:   path,
			Locator:    fmt.Sprintf("%d", fromInfo.lineNum),
			Metadata: map[string]string{
				MetadataImageName:    fromInfo.familiarName,
				MetadataRawRef:       fromInfo.sourceCode,
				MetadataTag:          fromInfo.tag,
				MetadataTagSuffix:    fromInfo.tagSuffix,
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

// parseStage extracts FROM instruction info. Returns nil if skipped.
// Single-line FROM with semver tag only. Skips variable expansion and non-semver.
func parseStage(stage instructions.Stage) *fromLine {
	if stage.BaseName == "" {
		return nil
	}

	if strings.Contains(stage.BaseName, "$") {
		return nil
	}

	// BuildKit normalizes SourceCode (removes backslash continuations),
	// but preserves multiple Location entries for multi-line instructions.
	if len(stage.Location) > 1 {
		return nil
	}

	lineNum := 0
	if len(stage.Location) > 0 {
		lineNum = stage.Location[0].Start.Line
	}

	ref, err := reference.ParseNormalizedNamed(stage.BaseName)
	if err != nil {
		return nil
	}

	// Skip if no explicit tag
	tagged, ok := ref.(reference.NamedTagged)
	if !ok {
		fmt.Printf("Skipping %s: no explicit tag\n", stage.BaseName)
		return nil
	}
	tag := tagged.Tag()

	parsed, err := containerimage.ParseTag(tag)
	if err != nil {
		fmt.Printf("parseTag")
		fmt.Println(err)
		return nil
	}

	return &fromLine{
		lineNum:      lineNum,
		sourceCode:   stage.SourceCode,
		platform:     stage.Platform,
		familiarName: reference.FamiliarName(ref),
		tag:          tag,
		tagSuffix:    parsed.Suffix,
		alias:        stage.Name,
		repository:   reference.Path(ref),
		registryHost: reference.Domain(ref),
		version:      parsed.Version,
	}
}

// lookupMapping finds the mapping for an image name.
func (s *Source) lookupMapping(imageName string) ImageMapping {
	if mapping, ok := s.mappings[imageName]; ok {
		return mapping
	}
	return ImageMapping{
		Match:          imageName,
		DependencyName: imageName,
	}
}

// Apply applies the planned changes to the Dockerfiles.
func (s *Source) Apply(ctx context.Context, changes []model.PlannedChange) error {
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

	for path, fileChanges := range changesByFile {
		if err := s.applyToFile(path, fileChanges); err != nil {
			return fmt.Errorf("failed to apply changes to %s: %w", path, err)
		}
	}

	return nil
}

func (s *Source) applyToFile(path string, changes []model.PlannedChange) error {
	currentTags := make(map[int]string)
	newTags := make(map[int]string)
	for _, c := range changes {
		lineNum := 0
		if _, err := fmt.Sscanf(c.Dependency.Locator, "%d", &lineNum); err != nil {
			return fmt.Errorf("invalid locator %q: %w", c.Dependency.Locator, err)
		}
		currentTag := c.Dependency.Metadata[MetadataTag]
		currentTags[lineNum] = currentTag
		newTags[lineNum] = getNewTag(c)
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

		if currentTag, ok := currentTags[lineNum]; ok {
			line = replaceTag(line, currentTag, newTags[lineNum])
		}
		output.WriteString(line)
		output.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	result := output.String()
	if !strings.HasSuffix(string(data), "\n") && strings.HasSuffix(result, "\n") {
		result = result[:len(result)-1]
	}

	return os.WriteFile(path, []byte(result), 0644)
}

// getNewTag determines the new tag for a planned change.
// Priority: SelectedCandidate.Metadata["tag"] -> formatTag fallback
func getNewTag(c model.PlannedChange) string {
	// Prefer raw tag from selected candidate metadata
	if c.SelectedCandidate != nil && c.SelectedCandidate.Metadata != nil {
		if rawTag := c.SelectedCandidate.Metadata["tag"]; rawTag != "" {
			return rawTag
		}
	}
	// Fallback to formatTag
	return formatTag(c.TargetVersion, c.Dependency.Metadata[MetadataTag])
}

// formatTag formats the target version as a Docker tag.
// Preserves suffix from original tag (e.g., "1.22.0-bookworm" -> "1.22.2-bookworm").
func formatTag(version *semver.Version, originalTag string) string {
	// Split on first '-' to extract base and suffix
	base, suffix, hasSuffix := strings.Cut(originalTag, "-")

	var newBase string
	if strings.HasPrefix(base, "v") {
		newBase = "v" + version.String()
	} else {
		parts := strings.Split(strings.TrimPrefix(base, "v"), ".")
		switch len(parts) {
		case 1:
			newBase = fmt.Sprintf("%d", version.Major())
		case 2:
			newBase = fmt.Sprintf("%d.%d", version.Major(), version.Minor())
		default:
			newBase = version.String()
		}
	}

	if hasSuffix {
		return newBase + "-" + suffix
	}
	return newBase
}

// replaceTag replaces the tag in a single-line FROM instruction.
func replaceTag(line, currentTag, newTag string) string {
	result, err := dfparser.Parse(strings.NewReader(line))
	if err != nil {
		return line
	}

	stages, _, err := instructions.Parse(result.AST, nil)
	if err != nil || len(stages) == 0 {
		return line
	}

	stage := stages[0]
	baseName := stage.BaseName
	if baseName == "" {
		return line
	}

	baseNameIdx := strings.Index(line, baseName)
	if baseNameIdx == -1 {
		return line
	}

	search := ":" + currentTag
	tagIdx := strings.LastIndex(baseName, search)
	if tagIdx == -1 {
		return line
	}

	absoluteTagIdx := baseNameIdx + tagIdx
	return line[:absoluteTagIdx+1] + newTag + line[absoluteTagIdx+len(search):]
}
