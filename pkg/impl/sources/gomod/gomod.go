package gomod

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/mod/modfile"
	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/pkg/impl/sources"
)

const sourceKind = "gomod"

func init() {
	sources.Register(sourceKind, &Factory{})
}

// Factory creates gomod sources.
type Factory struct{}

// Kind returns the source type.
func (f *Factory) Kind() string {
	return sourceKind
}

// Create creates a new gomod source from configuration map.
func (f *Factory) Create(config map[string]interface{}, basePath string) (model.Source, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode gomod config: %w", err)
	}

	// Resolve paths relative to basePath
	paths := make([]string, len(cfg.ManifestPaths))
	for i, p := range cfg.ManifestPaths {
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

// Source extracts and updates dependencies from go.mod files.
type Source struct {
	paths []string
}

// Ensure Source implements model.Source
var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// Extract extracts dependencies from all configured go.mod files.
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

	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to parse go.mod: %w", err)
	}

	var deps []model.Dependency

	for _, req := range f.Require {
		if req.Indirect {
			// Skip indirect dependencies for now
			continue
		}

		v, err := semver.NewVersion(req.Mod.Version)
		if err != nil {
			// Skip non-semver versions
			continue
		}

		deps = append(deps, model.Dependency{
			Name:       req.Mod.Path,
			Version:    v,
			SourceKind: sourceKind,
			FilePath:   path,
			Locator:    req.Mod.Path, // Use module path as locator
			Metadata:   nil,
		})
	}

	return deps, nil
}

// Apply applies the planned changes to the go.mod files.
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
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	f, err := modfile.Parse(path, data, nil)
	if err != nil {
		return fmt.Errorf("failed to parse go.mod: %w", err)
	}

	updates := make(map[string]string)
	for _, c := range changes {
		updates[c.Dependency.Name] = c.TargetVersion.Original()
	}

	for _, req := range f.Require {
		if newVersion, ok := updates[req.Mod.Path]; ok {
			if err := f.AddRequire(req.Mod.Path, newVersion); err != nil {
				return fmt.Errorf("failed to update require %s: %w", req.Mod.Path, err)
			}
		}
	}

	// Format and write back
	newData, err := f.Format()
	if err != nil {
		return fmt.Errorf("failed to format go.mod: %w", err)
	}

	if err := os.WriteFile(path, newData, 0644); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	return nil
}
