package model

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

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

// ResolveCacheKey returns the cache key for resolver results.
// It includes dependency name and resolver-affecting metadata when present.
func (d Dependency) ResolveCacheKey() string {
	parts := []string{d.Name}
	for _, key := range resolutionCacheMetadataKeys {
		if value := d.Metadata[key]; value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, "|")
}

// DisplayVersion returns the version string that should be used for user-facing output or links.
func (d Dependency) DisplayVersion() string {
	return DisplayVersion(d.Version, d.Metadata)
}
