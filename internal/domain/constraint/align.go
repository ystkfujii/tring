package constraint

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
)

// AlignConstraint ensures multiple modules share the same version.
type AlignConstraint struct {
	name     string
	anchor   string
	members  map[string]bool
	required bool
}

// NewAlignConstraint creates an AlignConstraint from config.
func NewAlignConstraint(c config.Constraint) (*AlignConstraint, error) {
	if c.Type != "align" {
		return nil, fmt.Errorf("expected constraint type 'align', got %q", c.Type)
	}

	members := make(map[string]bool)
	for _, m := range c.Members {
		members[m] = true
	}

	return &AlignConstraint{
		name:     c.Name,
		anchor:   c.Anchor,
		members:  members,
		required: c.Required,
	}, nil
}

// Name returns the constraint name.
func (a *AlignConstraint) Name() string {
	if a.name != "" {
		return a.name
	}
	return fmt.Sprintf("align(%s)", a.anchor)
}

// Apply aligns all member modules to the anchor's version.
// For required constraints, returns an error immediately if alignment fails (changes are not modified).
// For non-required constraints, silently skips members that cannot be aligned.
func (a *AlignConstraint) Apply(changes []model.PlannedChange, candidatesMap map[string]model.Candidates) error {
	anchorVersion := a.resolveAnchorVersion(changes)

	if anchorVersion == nil {
		if a.required {
			return fmt.Errorf("align constraint %q: anchor %q has no version to align to", a.Name(), a.anchor)
		}
		return nil
	}

	anchorVersionStr := anchorVersion.Original()

	// For required constraints, validate all members first before making any changes
	if a.required {
		if err := a.validateMembers(changes, candidatesMap, anchorVersionStr); err != nil {
			return err
		}
	}

	// Apply alignment to members
	for i := range changes {
		if !a.members[changes[i].Dependency.Name] {
			continue
		}

		if changes[i].Dependency.Name == a.anchor {
			continue
		}

		candidates, ok := candidatesMap[changes[i].Dependency.Name]
		if !ok {
			// Non-required: skip silently (required case already validated above)
			continue
		}

		candidate := candidates.FindByVersion(anchorVersionStr)
		if candidate == nil {
			// Non-required: skip silently (required case already validated above)
			continue
		}

		changes[i].TargetVersion = candidate.Version
		changes[i].SelectedCandidate = candidate
		changes[i].SkipReason = model.SkipReasonNone

		if changes[i].CurrentVersion != nil && candidate.Version.Compare(changes[i].CurrentVersion) == 0 {
			changes[i].SkipReason = model.SkipReasonAlreadyLatest
		}
	}

	return nil
}

// validateMembers checks that all members can be aligned to the anchor version.
// Returns an error if any member cannot be aligned.
func (a *AlignConstraint) validateMembers(changes []model.PlannedChange, candidatesMap map[string]model.Candidates, anchorVersionStr string) error {
	failedMembers := make(map[string]struct{})

	for i := range changes {
		if !a.members[changes[i].Dependency.Name] {
			continue
		}

		if changes[i].Dependency.Name == a.anchor {
			continue
		}

		candidates, ok := candidatesMap[changes[i].Dependency.Name]
		if !ok {
			failedMembers[changes[i].Dependency.Name] = struct{}{}
			continue
		}

		candidate := candidates.FindByVersion(anchorVersionStr)
		if candidate == nil {
			failedMembers[changes[i].Dependency.Name] = struct{}{}
		}
	}

	if len(failedMembers) > 0 {
		names := make([]string, 0, len(failedMembers))
		for name := range failedMembers {
			names = append(names, name)
		}
		return fmt.Errorf("align constraint %q: members %v have no candidate for version %s", a.Name(), names, anchorVersionStr)
	}

	return nil
}

// resolveAnchorVersion determines the version to align members to.
// Priority: TargetVersion (if anchor will be updated) > CurrentVersion (if anchor is already_latest)
func (a *AlignConstraint) resolveAnchorVersion(changes []model.PlannedChange) *semver.Version {
	for i := range changes {
		if changes[i].Dependency.Name != a.anchor {
			continue
		}

		// If anchor has a target version, use it
		if changes[i].TargetVersion != nil {
			return changes[i].TargetVersion
		}

		// If anchor is already_latest, use current version
		if changes[i].SkipReason == model.SkipReasonAlreadyLatest && changes[i].CurrentVersion != nil {
			return changes[i].CurrentVersion
		}

		// Anchor has no usable version (e.g., resolve error)
		return nil
	}

	// Anchor not found in changes
	return nil
}

// AffectedModules returns a list of modules affected by this constraint.
func (a *AlignConstraint) AffectedModules() []string {
	modules := make([]string, 0, len(a.members))
	for m := range a.members {
		modules = append(modules, m)
	}
	return modules
}
