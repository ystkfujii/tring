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
