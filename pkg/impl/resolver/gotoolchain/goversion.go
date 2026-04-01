package gotoolchain

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Go version patterns
var (
	// Matches: 1.23, 1.23.1, 1.23rc1, 1.23beta2, go1.23, go1.23.1, go1.23rc1, go1.23beta2
	goVersionRegex = regexp.MustCompile(`^(?:go)?(\d+)\.(\d+)(?:\.(\d+))?(?:(rc|beta|alpha)(\d+))?$`)
)

// ParseGoVersion converts a Go version string to semver.
// Supports formats:
//   - 1.23 -> 1.23.0
//   - 1.23.1 -> 1.23.1
//   - 1.23rc1 -> 1.23.0-rc.1
//   - 1.23beta2 -> 1.23.0-beta.2
//   - go1.23.1 -> 1.23.1
//   - go1.23rc1 -> 1.23.0-rc.1
func ParseGoVersion(version string) (*semver.Version, error) {
	normalized := NormalizeGoVersion(version)
	return semver.NewVersion(normalized)
}

// NormalizeGoVersion converts a Go version string to a semver-compatible string.
func NormalizeGoVersion(version string) string {
	matches := goVersionRegex.FindStringSubmatch(version)
	if matches == nil {
		// Return as-is if it doesn't match the pattern
		return strings.TrimPrefix(version, "go")
	}

	major := matches[1]
	minor := matches[2]
	patch := matches[3]
	preType := matches[4] // rc, beta, alpha
	preNum := matches[5]  // prerelease number

	if patch == "" {
		patch = "0"
	}

	result := major + "." + minor + "." + patch

	if preType != "" && preNum != "" {
		result += "-" + preType + "." + preNum
	}

	return result
}

// FormatGoDirective formats a semver version for use in go directive.
// Stable: 1.23.0 -> 1.23, 1.23.1 -> 1.23.1
// Prerelease: 1.23.0-rc.1 -> 1.23rc1, 1.23.0-beta.2 -> 1.23beta2
func FormatGoDirective(v *semver.Version) string {
	major := v.Major()
	minor := v.Minor()
	patch := v.Patch()
	pre := v.Prerelease()

	if pre != "" {
		// Prerelease: use major.minor + prerelease suffix (no patch)
		// 1.23.0-rc.1 -> 1.23rc1
		return formatInt(major) + "." + formatInt(minor) + formatPrerelease(pre)
	}

	// Stable: include patch only if non-zero
	if patch == 0 {
		// 1.23.0 -> 1.23
		return formatInt(major) + "." + formatInt(minor)
	}
	// 1.23.1 -> 1.23.1
	return formatInt(major) + "." + formatInt(minor) + "." + formatInt(patch)
}

// FormatToolchainDirective formats a semver version for use in toolchain directive.
// Stable: 1.23.0 -> go1.23.0, 1.23.1 -> go1.23.1
// Prerelease: 1.23.0-rc.1 -> go1.23rc1, 1.23.0-beta.2 -> go1.23beta2
func FormatToolchainDirective(v *semver.Version) string {
	major := v.Major()
	minor := v.Minor()
	patch := v.Patch()
	pre := v.Prerelease()

	if pre != "" {
		// Prerelease: use major.minor + prerelease suffix (no patch)
		// 1.23.0-rc.1 -> go1.23rc1
		return "go" + formatInt(major) + "." + formatInt(minor) + formatPrerelease(pre)
	}

	// Stable: always include patch
	return "go" + formatInt(major) + "." + formatInt(minor) + "." + formatInt(patch)
}

func formatInt(n uint64) string {
	return strconv.FormatUint(n, 10)
}

// formatPrerelease converts semver prerelease (rc.1) to Go format (rc1)
func formatPrerelease(pre string) string {
	// Remove dots from prerelease: rc.1 -> rc1, beta.2 -> beta2
	return strings.ReplaceAll(pre, ".", "")
}
