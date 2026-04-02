package containerimage

import (
	"regexp"

	"github.com/Masterminds/semver/v3"
)

var simpleTagPattern = regexp.MustCompile(`^v?\d+(?:\.\d+){0,2}$`)

// ParseTag attempts to parse a container image tag as semver.
// Supports: 1, 1.2, 1.2.3, v1.2.3
// Does not support: 1.24-alpine, bookworm, latest
func ParseTag(tag string) (*semver.Version, error) {
	if !simpleTagPattern.MatchString(tag) {
		return nil, semver.ErrInvalidSemVer
	}
	return semver.NewVersion(tag)
}

// NormalizeTag converts a container image tag to a semver-compatible string.
// Supports: 1 -> 1.0.0, 1.2 -> 1.2.0, 1.2.3 -> 1.2.3, v1.2.3 -> 1.2.3
// Returns empty string if the tag cannot be normalized.
func NormalizeTag(tag string) string {
	v, err := ParseTag(tag)
	if err != nil {
		return ""
	}
	return v.String()
}
