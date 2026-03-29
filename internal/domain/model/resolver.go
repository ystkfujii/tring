package model

import "context"

// Resolver fetches version candidates for dependencies.
type Resolver interface {
	// Kind returns the resolver type identifier (e.g., "goproxy")
	Kind() string

	// Resolve fetches version candidates for the given dependency.
	Resolve(ctx context.Context, dep Dependency) (Candidates, error)
}

// ResolverFactory creates Resolver instances.
type ResolverFactory interface {
	// Kind returns the resolver type this factory handles
	Kind() string

	// Create creates a Resolver from configuration map.
	// config may be nil if no resolver_config is specified.
	Create(config map[string]interface{}) (Resolver, error)
}
