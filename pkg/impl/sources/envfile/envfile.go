package envfile

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

const sourceKind = "envfile"

func init() {
	sources.Register(sourceKind, &Factory{})
}

// Factory creates envfile sources.
type Factory struct{}

// Kind returns the source type.
func (f *Factory) Kind() string {
	return sourceKind
}

// Create creates a new envfile source from configuration map.
func (f *Factory) Create(config map[string]interface{}, basePath string) (model.Source, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse envfile config: %w", err)
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

	// Build variable map
	varMap := make(map[string]Variable)
	for _, v := range cfg.Variables {
		varMap[v.Name] = v
	}

	return &Source{
		paths:     paths,
		variables: cfg.Variables,
		varMap:    varMap,
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

// Source extracts and updates dependencies from envfiles.
type Source struct {
	paths     []string
	variables []Variable
	varMap    map[string]Variable
}

// Ensure Source implements model.Source
var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// variableRegex matches variable assignments:
// FOO = value
// FOO := value
// FOO ?= value
var variableRegex = regexp.MustCompile(`^(\w+)\s*[:?]?=\s*(.+)$`)

// Extract extracts dependencies from all configured envfiles.
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

		// Skip comments and empty lines
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Try to match variable assignment
		matches := variableRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		varName := matches[1]
		varValue := strings.TrimSpace(matches[2])

		// Check if this variable is tracked
		varConfig, ok := s.varMap[varName]
		if !ok {
			continue
		}

		// Parse the value as a version
		v, err := semver.NewVersion(varValue)
		if err != nil {
			// Skip non-semver values
			continue
		}

		deps = append(deps, model.Dependency{
			Name:       varConfig.ResolveWith,
			Version:    v,
			SourceKind: sourceKind,
			FilePath:   path,
			Locator:    varName, // Use variable name as locator
			Metadata: map[string]string{
				"line":     fmt.Sprintf("%d", lineNum),
				"var_name": varName,
			},
		})
	}

	return deps, scanner.Err()
}

// Apply applies the planned changes to the envfiles.
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
	updates := make(map[string]string)
	for _, c := range changes {
		varName := c.Dependency.Locator
		updates[varName] = c.TargetVersion.Original()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var output strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := scanner.Text()

		matches := variableRegex.FindStringSubmatch(line)
		if matches != nil {
			varName := matches[1]
			if newVersion, ok := updates[varName]; ok {
				opIdx := strings.Index(line, "=")
				if opIdx > 0 {
					prefix := line[:opIdx+1]
					output.WriteString(prefix)
					output.WriteString(" ")
					output.WriteString(newVersion)
					output.WriteString("\n")
					continue
				}
			}
		}

		output.WriteString(line)
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
