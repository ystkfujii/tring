package githubaction

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
	"github.com/ystkfujii/tring/pkg/impl/sources"
)

const sourceKind = "githubaction"

func init() {
	sources.Register(sourceKind, &Factory{})
}

// Factory creates githubaction sources.
type Factory struct{}

// Kind returns the source type.
func (f *Factory) Kind() string {
	return sourceKind
}

// Create creates a new githubaction source from configuration map.
func (f *Factory) Create(config map[string]interface{}, basePath string) (model.Source, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode githubaction config: %w", err)
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

	return &Source{paths: paths}, nil
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

// Source extracts and updates GitHub Action dependencies from workflow files.
type Source struct {
	paths []string
}

// Ensure Source implements model.Source
var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// usesRegex matches GitHub Action uses directives:
// - uses: owner/repo@ref
// - uses: owner/repo/path@ref
// - uses: owner/repo@sha # version
// - uses: owner/repo/path@sha # version
//
// Groups:
// 1: owner/repo (or owner/repo/path)
// 2: ref (version or SHA)
// 3: optional inline comment (including # and version)
var usesRegex = regexp.MustCompile(`^(\s*-?\s*uses:\s*)([^@\s]+)@([a-zA-Z0-9._-]+)(\s*#\s*v[^\s]+)?(.*)$`)

// shaRegex matches a 40-character hexadecimal SHA.
var shaRegex = regexp.MustCompile(`^[a-f0-9]{40}$`)

// versionCommentRegex extracts the version from inline comment "# v1.2.3"
var versionCommentRegex = regexp.MustCompile(`#\s*(v[^\s]+)`)

// Extract extracts dependencies from all configured workflow files.
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

		dep := s.parseLine(line, lineNum, path)
		if dep != nil {
			deps = append(deps, *dep)
		}
	}

	return deps, scanner.Err()
}

// parseLine attempts to parse a uses directive from a line.
// Returns nil if the line is not a valid uses directive or should be skipped.
func (s *Source) parseLine(line string, lineNum int, filePath string) *model.Dependency {
	matches := usesRegex.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	// matches[1] = prefix (spaces + "uses: ")
	// matches[2] = target (owner/repo or owner/repo/path)
	// matches[3] = ref (version or SHA)
	// matches[4] = optional version comment
	// matches[5] = trailing content

	target := matches[2]
	ref := matches[3]
	versionComment := strings.TrimSpace(matches[4])

	// Skip local actions (./path)
	if strings.HasPrefix(target, "./") || strings.HasPrefix(target, ".\\") {
		return nil
	}

	// Skip docker:// references
	if strings.HasPrefix(target, "docker://") {
		return nil
	}

	// Parse owner/repo and optional subpath
	repo, subpath := parseTarget(target)
	if repo == "" {
		return nil
	}

	// Determine if the ref is a SHA
	isPinnedBySHA := shaRegex.MatchString(ref)

	// Extract version
	var versionStr string
	if isPinnedBySHA {
		// For SHA pins, the version should be in the comment
		if versionComment == "" {
			// Skip SHA pins without version comment
			return nil
		}
		versionMatches := versionCommentRegex.FindStringSubmatch(versionComment)
		if versionMatches == nil {
			return nil
		}
		versionStr = versionMatches[1]
	} else {
		// For version refs, the ref itself is the version
		versionStr = ref

		// Skip floating tags like "main", "master", "v2" (major only)
		if isFloatingRef(ref) {
			return nil
		}
	}

	// Parse as semver
	v, err := semver.NewVersion(versionStr)
	if err != nil {
		// Skip non-semver versions
		return nil
	}

	// Build locator for unique identification
	locator := fmt.Sprintf("line:%d:%s", lineNum, target)

	return &model.Dependency{
		Name:       repo,
		Version:    v,
		SourceKind: sourceKind,
		FilePath:   filePath,
		Locator:    locator,
		Metadata: map[string]string{
			"original_target": target,
			"repo":            repo,
			"subpath":         subpath,
			"current_ref":     ref,
			"pinned_by_sha":   boolToString(isPinnedBySHA),
			"version_comment": versionStr,
			"line":            fmt.Sprintf("%d", lineNum),
		},
	}
}

// parseTarget parses the uses target into owner/repo and optional subpath.
// Example: "actions/checkout" -> ("actions/checkout", "")
// Example: "actions/cache/save" -> ("actions/cache", "save")
func parseTarget(target string) (repo, subpath string) {
	parts := strings.Split(target, "/")
	if len(parts) < 2 {
		return "", ""
	}

	// owner/repo
	repo = parts[0] + "/" + parts[1]

	// Optional subpath (actions/cache/save -> save)
	if len(parts) > 2 {
		subpath = strings.Join(parts[2:], "/")
	}

	return repo, subpath
}

// isFloatingRef returns true if the ref is a floating tag that should be skipped.
func isFloatingRef(ref string) bool {
	// Skip non-version refs
	if ref == "main" || ref == "master" || ref == "latest" {
		return true
	}

	// Skip major-only tags like "v1", "v2"
	if matched, _ := regexp.MatchString(`^v\d+$`, ref); matched {
		return true
	}

	return false
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Apply applies the planned changes to the workflow files.
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
	// Build a map of line number -> change
	changesByLine := make(map[int]model.PlannedChange)
	for _, c := range changes {
		lineNum := 0
		_, _ = fmt.Sscanf(c.Dependency.Metadata["line"], "%d", &lineNum)
		if lineNum > 0 {
			changesByLine[lineNum] = c
		}
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

		change, hasChange := changesByLine[lineNum]
		if hasChange {
			newLine := s.updateLine(line, change)
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

// updateLine updates a uses line with the new version.
func (s *Source) updateLine(line string, change model.PlannedChange) string {
	matches := usesRegex.FindStringSubmatch(line)
	if matches == nil {
		// Should not happen, but return unchanged if it does
		return line
	}

	prefix := matches[1] // "  uses: "
	target := matches[2] // "owner/repo" or "owner/repo/path"
	// oldRef := matches[3]   // old version or SHA
	// oldComment := matches[4]
	trailing := matches[5] // any trailing content

	isPinnedBySHA := change.Dependency.Metadata["pinned_by_sha"] == "true"
	newVersion := change.TargetVersion.Original()

	var newRef string
	var newComment string

	if isPinnedBySHA {
		// For SHA pins, update both SHA and version comment
		newSHA := ""
		if change.SelectedCandidate != nil && change.SelectedCandidate.Metadata != nil {
			newSHA = change.SelectedCandidate.Metadata["commit_sha"]
		}

		if newSHA == "" {
			// Cannot update SHA pin without new SHA, keep version only update
			// This shouldn't happen if resolver is working correctly
			newRef = newVersion
			newComment = ""
		} else {
			newRef = newSHA
			newComment = fmt.Sprintf(" # %s", newVersion)
		}
	} else {
		// For version refs, just update the version
		newRef = newVersion
		newComment = ""
	}

	return fmt.Sprintf("%s%s@%s%s%s", prefix, target, newRef, newComment, trailing)
}
