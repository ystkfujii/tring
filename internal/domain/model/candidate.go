package model

import (
	"time"

	"github.com/Masterminds/semver/v3"
)

// Candidate represents a version candidate returned by a resolver.
type Candidate struct {
	// Version is the candidate version
	Version *semver.Version

	// ReleasedAt is the release timestamp of this version
	ReleasedAt time.Time

	// Metadata contains resolver-specific information needed for updates.
	// For GitHub Actions: "commit_sha" contains the SHA for SHA-pinned references.
	// For Go modules: typically nil.
	Metadata map[string]string
}

// Candidates holds a list of version candidates.
type Candidates struct {
	Items []Candidate
}

// IsEmpty returns true if there are no candidates.
func (c Candidates) IsEmpty() bool {
	return len(c.Items) == 0
}

// FilterStable returns only stable (non-prerelease) candidates.
func (c Candidates) FilterStable() Candidates {
	var stable []Candidate
	for _, item := range c.Items {
		if item.Version.Prerelease() == "" {
			stable = append(stable, item)
		}
	}
	return Candidates{Items: stable}
}

// FilterByStability filters candidates based on the current version's stability.
// If current is stable (or nil), only stable candidates are returned.
// If current is a prerelease, both stable and prerelease candidates are returned.
func (c Candidates) FilterByStability(current *semver.Version) Candidates {
	// If current is nil or stable, filter to stable only
	if current == nil || current.Prerelease() == "" {
		return c.FilterStable()
	}
	// Current is prerelease, allow all candidates
	return c
}

// FilterByAge returns candidates older than the given duration.
func (c Candidates) FilterByAge(minAge time.Duration, now time.Time) Candidates {
	var filtered []Candidate
	cutoff := now.Add(-minAge)
	for _, item := range c.Items {
		if !item.ReleasedAt.After(cutoff) {
			filtered = append(filtered, item)
		}
	}
	return Candidates{Items: filtered}
}

// Latest returns the candidate with the highest version, or nil if empty.
func (c Candidates) Latest() *Candidate {
	if c.IsEmpty() {
		return nil
	}
	latest := &c.Items[0]
	for i := 1; i < len(c.Items); i++ {
		if c.Items[i].Version.Compare(latest.Version) > 0 {
			latest = &c.Items[i]
		}
	}
	return latest
}

// FilterSameMajorMinor returns candidates with the same major.minor as the given version.
func (c Candidates) FilterSameMajorMinor(v *semver.Version) Candidates {
	var filtered []Candidate
	for _, item := range c.Items {
		if item.Version.Major() == v.Major() && item.Version.Minor() == v.Minor() {
			filtered = append(filtered, item)
		}
	}
	return Candidates{Items: filtered}
}

// FilterSameMajor returns candidates with the same major as the given version.
func (c Candidates) FilterSameMajor(v *semver.Version) Candidates {
	var filtered []Candidate
	for _, item := range c.Items {
		if item.Version.Major() == v.Major() {
			filtered = append(filtered, item)
		}
	}
	return Candidates{Items: filtered}
}

// FindByVersion returns the candidate matching the given version string, or nil.
func (c Candidates) FindByVersion(versionStr string) *Candidate {
	for i := range c.Items {
		if c.Items[i].Version.Original() == versionStr || c.Items[i].Version.String() == versionStr {
			return &c.Items[i]
		}
	}
	return nil
}
