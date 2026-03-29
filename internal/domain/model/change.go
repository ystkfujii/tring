package model

import "github.com/Masterminds/semver/v3"

// SkipReason indicates why a dependency was not updated.
type SkipReason string

const (
	// SkipReasonNone means no skip (change will be applied)
	SkipReasonNone SkipReason = ""
	// SkipReasonAlreadyLatest means the dependency is already at the latest version
	SkipReasonAlreadyLatest SkipReason = "already_latest"
	// SkipReasonNoCandidate means no suitable candidate was found
	SkipReasonNoCandidate SkipReason = "no_candidate"
	// SkipReasonConstraintFailed means a constraint prevented the update
	SkipReasonConstraintFailed SkipReason = "constraint_failed"
	// SkipReasonTooNew means all candidates are newer than min_release_age
	SkipReasonTooNew SkipReason = "too_new"
	// SkipReasonExcluded means the dependency was excluded by selectors
	SkipReasonExcluded SkipReason = "excluded"
	// SkipReasonResolveError means resolver failed to get candidates
	SkipReasonResolveError SkipReason = "resolve_error"
)

// String returns a human-readable description of the skip reason.
func (s SkipReason) String() string {
	switch s {
	case SkipReasonNone:
		return ""
	case SkipReasonAlreadyLatest:
		return "already at latest version"
	case SkipReasonNoCandidate:
		return "no suitable candidate found"
	case SkipReasonConstraintFailed:
		return "constraint prevented update"
	case SkipReasonTooNew:
		return "all candidates too new (min_release_age)"
	case SkipReasonExcluded:
		return "excluded by selector"
	case SkipReasonResolveError:
		return "failed to resolve versions"
	default:
		return string(s)
	}
}

// PlannedChange represents a planned version change for a dependency.
type PlannedChange struct {
	// Dependency is the dependency to be updated
	Dependency Dependency

	// CurrentVersion is the current version
	CurrentVersion *semver.Version

	// TargetVersion is the version to update to (nil if skipped)
	TargetVersion *semver.Version

	// SelectedCandidate is the candidate that was selected (nil if skipped)
	SelectedCandidate *Candidate

	// SkipReason indicates why the change was skipped (empty if not skipped)
	SkipReason SkipReason

	// ErrorDetail provides additional context when SkipReason indicates an error.
	// This helps users understand why a dependency couldn't be updated.
	ErrorDetail string

	// DiffLink is an optional link to view the diff between versions
	DiffLink string

	// Strategy is the strategy used for selection
	Strategy string
}

// IsSkipped returns true if this change was skipped.
func (p PlannedChange) IsSkipped() bool {
	return p.SkipReason != SkipReasonNone
}

// HasUpdate returns true if there is an actual version change.
func (p PlannedChange) HasUpdate() bool {
	if p.IsSkipped() {
		return false
	}
	if p.TargetVersion == nil || p.CurrentVersion == nil {
		return false
	}
	return p.TargetVersion.Compare(p.CurrentVersion) != 0
}

// ChangeSet represents a collection of planned changes for a group.
type ChangeSet struct {
	GroupName string
	Changes   []PlannedChange
}

// Updates returns only the changes that have actual updates.
func (cs ChangeSet) Updates() []PlannedChange {
	var updates []PlannedChange
	for _, c := range cs.Changes {
		if c.HasUpdate() {
			updates = append(updates, c)
		}
	}
	return updates
}

// Skipped returns only the changes that were skipped.
func (cs ChangeSet) Skipped() []PlannedChange {
	var skipped []PlannedChange
	for _, c := range cs.Changes {
		if c.IsSkipped() {
			skipped = append(skipped, c)
		}
	}
	return skipped
}

// Stats returns statistics about the change set.
func (cs ChangeSet) Stats() (updates, skipped int) {
	for _, c := range cs.Changes {
		if c.HasUpdate() {
			updates++
		} else if c.IsSkipped() {
			skipped++
		}
	}
	return
}
