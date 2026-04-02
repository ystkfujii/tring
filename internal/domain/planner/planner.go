package planner

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/constraint"
	"github.com/ystkfujii/tring/internal/domain/model"
)

// Planner plans dependency updates.
type Planner struct {
	resolver     model.Resolver
	strategy     config.Strategy
	minAge       time.Duration
	selector     *Selector
	constraints  []constraint.Constraint
	showDiffLink bool
	now          time.Time
}

// Options configures the planner.
type Options struct {
	Resolver     model.Resolver
	Strategy     config.Strategy
	MinAge       time.Duration
	Selectors    *config.Selectors
	Constraints  []config.Constraint
	ShowDiffLink bool
	Now          time.Time // For testing, defaults to time.Now()
}

// New creates a new Planner.
func New(opts Options) (*Planner, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	p := &Planner{
		resolver:     opts.Resolver,
		strategy:     opts.Strategy,
		minAge:       opts.MinAge,
		selector:     NewSelector(opts.Selectors),
		showDiffLink: opts.ShowDiffLink,
		now:          now,
	}

	for _, c := range opts.Constraints {
		switch c.Type {
		case "align":
			ac, err := constraint.NewAlignConstraint(c)
			if err != nil {
				return nil, fmt.Errorf("failed to create align constraint: %w", err)
			}
			p.constraints = append(p.constraints, ac)
		default:
			return nil, fmt.Errorf("unknown constraint type: %q", c.Type)
		}
	}

	return p, nil
}

// Plan creates planned changes for the given dependencies.
func (p *Planner) Plan(ctx context.Context, deps []model.Dependency) ([]model.PlannedChange, error) {
	var changes []model.PlannedChange
	// candidatesMap caches resolver results by dependency name.
	// If a dependency name appears multiple times (e.g., same module in different files),
	// the resolver is called only once and the result is reused.
	candidatesMap := make(map[string]model.Candidates)

	for _, dep := range deps {
		change := p.planDependency(ctx, dep, candidatesMap)
		changes = append(changes, change)
	}

	for _, c := range p.constraints {
		if err := c.Apply(changes, candidatesMap); err != nil {
			// Log but don't fail for non-required constraints
			// The error is only returned for required constraints
			return nil, fmt.Errorf("constraint %q failed: %w", c.Name(), err)
		}
	}

	return changes, nil
}

func (p *Planner) planDependency(ctx context.Context, dep model.Dependency, candidatesMap map[string]model.Candidates) model.PlannedChange {
	change := model.PlannedChange{
		Dependency:     dep,
		CurrentVersion: dep.Version,
		Strategy:       string(p.strategy),
	}

	if !p.selector.Match(dep) {
		change.SkipReason = model.SkipReasonExcluded
		return change
	}

	// Check cache first
	cacheKey := dep.ResolverCacheKey()
	candidates, cached := candidatesMap[cacheKey]
	if !cached {
		var err error
		candidates, err = p.resolver.Resolve(ctx, dep)
		if err != nil {
			change.SkipReason = model.SkipReasonResolveError
			change.ErrorDetail = err.Error()
			return change
		}
		candidatesMap[cacheKey] = candidates
	}

	// Filter by stability based on current version:
	// - If current is stable (or nil), only consider stable candidates
	// - If current is prerelease, consider both stable and prerelease candidates
	candidates = candidates.FilterByStability(dep.Version)

	if candidates.IsEmpty() {
		change.SkipReason = model.SkipReasonNoCandidate
		return change
	}

	candidates = p.applyStrategy(candidates, dep.Version)

	if candidates.IsEmpty() {
		change.SkipReason = model.SkipReasonNoCandidate
		return change
	}

	if p.minAge > 0 {
		candidates = candidates.FilterByAge(p.minAge, p.now)
		if candidates.IsEmpty() {
			change.SkipReason = model.SkipReasonTooNew
			return change
		}
	}

	selected := candidates.Latest()
	if selected == nil {
		change.SkipReason = model.SkipReasonNoCandidate
		return change
	}

	if dep.Version != nil && selected.Version.Compare(dep.Version) <= 0 {
		change.SkipReason = model.SkipReasonAlreadyLatest
		return change
	}

	change.TargetVersion = selected.Version
	change.SelectedCandidate = selected

	if p.showDiffLink {
		change.DiffLink = generateDiffLink(dep.Version, selected)
	}

	return change
}

func (p *Planner) applyStrategy(candidates model.Candidates, current *semver.Version) model.Candidates {
	if current == nil {
		return candidates
	}

	switch p.strategy {
	case config.StrategyPatch:
		return candidates.FilterSameMajorMinor(current)
	case config.StrategyMinor:
		return candidates.FilterSameMajor(current)
	default:
		// StrategyMajor or unknown: no filtering by major version.
		// Unknown strategies are rejected by validation, so this branch
		// is effectively StrategyMajor only.
		return candidates
	}
}

// generateDiffLink generates a diff link using metadata from the selected candidate.
// Returns empty string if repo_url metadata is not available or versions are nil.
func generateDiffLink(currentVersion *semver.Version, selected *model.Candidate) string {
	if currentVersion == nil || selected == nil || selected.Version == nil {
		return ""
	}

	// Get repo_url from candidate metadata (set by resolver)
	repoURL := selected.Metadata["repo_url"]
	if repoURL == "" {
		return ""
	}

	return fmt.Sprintf("%s/compare/%s...%s", repoURL, currentVersion.Original(), selected.Version.Original())
}
