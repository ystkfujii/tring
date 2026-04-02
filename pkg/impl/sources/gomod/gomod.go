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
	"github.com/ystkfujii/tring/pkg/impl/resolver/gotoolchain"
	"github.com/ystkfujii/tring/pkg/impl/sources"
)

const sourceKind = "gomod"

// Locator constants for go directive and toolchain
const (
	LocatorGoVersion = "$go"
	LocatorToolchain = "$toolchain"
)

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

	return &Source{
		paths:          paths,
		TrackRequire:   cfg.TrackRequire,
		trackGoVersion: cfg.TrackGoVersion,
		trackToolchain: cfg.TrackToolchain,
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

// Source extracts and updates dependencies from go.mod files.
type Source struct {
	paths          []string
	TrackRequire   bool
	trackGoVersion bool
	trackToolchain bool
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

	// Extract go directive if enabled
	if s.trackGoVersion && f.Go != nil && f.Go.Version != "" {
		// Use ParseGoVersion to handle both stable (1.23.0) and prerelease (1.23rc1) formats
		v, err := gotoolchain.ParseGoVersion(f.Go.Version)
		if err == nil {
			deps = append(deps, model.Dependency{
				Name:       "go",
				Version:    v,
				SourceKind: sourceKind,
				FilePath:   path,
				Locator:    LocatorGoVersion,
				Metadata:   nil,
			})
		}
	}

	// Extract toolchain directive if enabled
	if s.trackToolchain && f.Toolchain != nil && f.Toolchain.Name != "" {
		// Use ParseGoVersion to handle both stable (go1.23.0) and prerelease (go1.23rc1) formats
		// ParseGoVersion handles the "go" prefix automatically
		v, err := gotoolchain.ParseGoVersion(f.Toolchain.Name)
		if err == nil {
			deps = append(deps, model.Dependency{
				Name:       "go",
				Version:    v,
				SourceKind: sourceKind,
				FilePath:   path,
				Locator:    LocatorToolchain,
				Metadata:   nil,
			})
		}
	}

	if s.TrackRequire {
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

	for _, c := range changes {
		switch c.Dependency.Locator {
		case LocatorGoVersion:
			// Update go directive using proper Go version format
			goDirective := gotoolchain.FormatGoDirective(c.TargetVersion)
			if err := f.AddGoStmt(goDirective); err != nil {
				return fmt.Errorf("failed to update go directive: %w", err)
			}
		case LocatorToolchain:
			// Update toolchain directive using proper Go version format
			toolchainName := gotoolchain.FormatToolchainDirective(c.TargetVersion)
			if err := f.AddToolchainStmt(toolchainName); err != nil {
				return fmt.Errorf("failed to update toolchain directive: %w", err)
			}
		default:
			// Update require statement
			if err := f.AddRequire(c.Dependency.Name, c.TargetVersion.Original()); err != nil {
				return fmt.Errorf("failed to update require %s: %w", c.Dependency.Name, err)
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
