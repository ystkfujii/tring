package containerimage

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ParseTag attempts to parse a container image tag as semver.
// Supports: 1, 1.2, 1.2.3, v1.2.3
// Does not support: 1.24-alpine, bookworm, latest
func ParseTag(tag string) (*semver.Version, error) {
	normalized := NormalizeTag(tag)
	if normalized == "" {
		return nil, &TagNormalizationError{Tag: tag}
	}
	return semver.NewVersion(normalized)
}

// NormalizeTag converts a container image tag to a semver-compatible string.
// Supports: 1 -> 1.0.0, 1.2 -> 1.2.0, 1.2.3 -> 1.2.3, v1.2.3 -> 1.2.3
// Returns empty string if the tag cannot be normalized.
func NormalizeTag(tag string) string {
	// Remove 'v' prefix if present
	tag = strings.TrimPrefix(tag, "v")

	// Check if it's a pure semver pattern (only digits and dots)
	if !IsSimpleSemverTag(tag) {
		return ""
	}

	parts := strings.Split(tag, ".")
	switch len(parts) {
	case 1:
		// 1 -> 1.0.0
		return parts[0] + ".0.0"
	case 2:
		// 1.2 -> 1.2.0
		return parts[0] + "." + parts[1] + ".0"
	case 3:
		// 1.2.3 -> 1.2.3
		return tag
	default:
		return ""
	}
}

// IsSimpleSemverTag checks if a tag contains only digits and dots.
func IsSimpleSemverTag(tag string) bool {
	for _, c := range tag {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return tag != "" && !strings.HasPrefix(tag, ".") && !strings.HasSuffix(tag, ".")
}

// TagNormalizationError is returned when a tag cannot be normalized to semver.
type TagNormalizationError struct {
	Tag string
}

func (e *TagNormalizationError) Error() string {
	return "cannot normalize tag: " + e.Tag
}
