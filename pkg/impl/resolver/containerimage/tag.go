package containerimage

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ParsedTag holds the parsed components of a container image tag.
type ParsedTag struct {
	Version *semver.Version
	Suffix  string // e.g., "alpine" from "1.24-alpine"
	Raw     string // original tag
}

// ParseTag parses a container image tag into version and optional suffix.
// Splits on first '-' only: "1.24-alpine" -> version=1.24.0, suffix="alpine"
// Supports: 1, 1.2, 1.2.3, v1.2.3, 1.24-alpine, 1.24-alpine3.20
// Does not support: latest, bookworm (non-numeric base)
func ParseTag(tag string) (*ParsedTag, error) {
	base, suffix, _ := strings.Cut(tag, "-")

	v, err := semver.NewVersion(base)
	if err != nil {
		return nil, err
	}

	return &ParsedTag{
		Version: v,
		Suffix:  suffix,
		Raw:     tag,
	}, nil
}

// NormalizeTag converts a container image tag to a semver-compatible string.
// Supports: 1 -> 1.0.0, 1.2 -> 1.2.0, 1.2.3 -> 1.2.3, v1.2.3 -> 1.2.3
// Returns empty string if the tag cannot be normalized.
// Note: This returns only the base version, ignoring suffix.
func NormalizeTag(tag string) string {
	parsed, err := ParseTag(tag)
	if err != nil {
		return ""
	}
	return parsed.Version.String()
}
