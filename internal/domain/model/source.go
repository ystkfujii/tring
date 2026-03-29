package model

import "context"

// Source represents a source of dependencies that can be extracted and updated.
type Source interface {
	// Kind returns the source type identifier (e.g., "gomod", "envfile")
	Kind() string

	// Extract extracts all dependencies from this source.
	Extract(ctx context.Context) ([]Dependency, error)

	// Apply applies the planned changes to the source files.
	Apply(ctx context.Context, changes []PlannedChange) error
}

// SourceFactory creates Source instances from raw configuration.
type SourceFactory interface {
	// Kind returns the source type this factory handles
	Kind() string

	// Create creates a Source from configuration map.
	// basePath is the directory containing the config file, used to resolve relative paths.
	Create(config map[string]interface{}, basePath string) (Source, error)
}
