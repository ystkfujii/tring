package model

import "context"

// Resolver fetches version candidates for dependencies.
type Resolver interface {
	// Kind returns the resolver type identifier (e.g., "goproxy")
	Kind() string

	// Resolve fetches version candidates for the given dependency.
	Resolve(ctx context.Context, dep Dependency) (Candidates, error)
}
