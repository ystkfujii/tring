package model

import "github.com/Masterminds/semver/v3"

// Dependency represents a dependency extracted from a source.
type Dependency struct {
	// Name is the dependency name used for resolution (e.g., "github.com/spf13/cobra")
	Name string

	// Version is the current version of the dependency
	Version *semver.Version

	// SourceKind identifies the type of source (e.g., "gomod", "envfile")
	SourceKind string

	// FilePath is the path to the manifest file containing this dependency
	FilePath string

	// Locator is a source-specific identifier for locating the dependency within the file
	// For gomod: module path
	// For envfile: variable name
	Locator string

	// Metadata contains additional source-specific information
	Metadata map[string]string
}

// Key returns a unique identifier for this dependency within a source file.
func (d Dependency) Key() string {
	return d.SourceKind + ":" + d.FilePath + ":" + d.Locator
}

// ResolverCacheKey returns a key for caching resolver results.
// For containerimage dependencies, includes tag_suffix to avoid cache collision
// between different suffix variants (e.g., "go:1.24-alpine" vs "go:1.24-bookworm").
func (d Dependency) ResolverCacheKey() string {
	if d.SourceKind == "dockerfile" && d.Metadata != nil {
		if suffix := d.Metadata["tag_suffix"]; suffix != "" {
			return d.Name + ":" + suffix
		}
	}
	return d.Name
}
