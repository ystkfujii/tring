package containerimage

import (
	"context"
	"time"
)

// TagInfo represents a container image tag with its metadata.
type TagInfo struct {
	Name        string
	LastUpdated time.Time
}

// Provider abstracts container registry tag listing.
type Provider interface {
	// ListTags returns all available tags for the given repository.
	ListTags(ctx context.Context, repository string) ([]TagInfo, error)
}
