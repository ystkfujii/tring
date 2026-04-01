package config

import (
	"time"
)

// Config is the root configuration structure.
type Config struct {
	Version  int       `yaml:"version"`
	Defaults *Defaults `yaml:"defaults,omitempty"`
	Groups   []Group   `yaml:"groups"`
}

// Defaults contains default settings applied to all groups.
type Defaults struct {
	Policy *Policy `yaml:"policy,omitempty"`
}

// Group defines a set of dependencies to update together.
type Group struct {
	Name           string                 `yaml:"name"`
	Description    string                 `yaml:"description,omitempty"`
	Resolver       string                 `yaml:"resolver,omitempty"`
	ResolverConfig map[string]interface{} `yaml:"resolver_config,omitempty"`
	Sources        []RawSource            `yaml:"sources"`
	Selectors      *Selectors             `yaml:"selectors,omitempty"`
	Policy         *Policy                `yaml:"policy,omitempty"`
}

// RawSource represents a source with unparsed config.
type RawSource struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// Selectors defines include/exclude patterns for dependencies.
type Selectors struct {
	Include *SelectorPatterns `yaml:"include,omitempty"`
	Exclude *SelectorPatterns `yaml:"exclude,omitempty"`
}

// SelectorPatterns contains module patterns for filtering.
type SelectorPatterns struct {
	ModulePatterns []string `yaml:"module_patterns,omitempty"`
}

// Policy defines the update policy for a group.
type Policy struct {
	Selection   *Selection   `yaml:"selection,omitempty"`
	Constraints []Constraint `yaml:"constraints,omitempty"`
}

// Selection defines version selection strategy.
type Selection struct {
	Strategy      Strategy `yaml:"strategy,omitempty"`
	MinReleaseAge string   `yaml:"min_release_age,omitempty"`
}

// Strategy is the version selection strategy.
type Strategy string

const (
	// StrategyPatch selects the highest patch version within the same major.minor
	StrategyPatch Strategy = "patch"
	// StrategyMinor selects the highest minor.patch version within the same major
	StrategyMinor Strategy = "minor"
	// StrategyMajor allows updates across major versions (no major-based filtering)
	StrategyMajor Strategy = "major"
)

// Constraint defines a version constraint.
type Constraint struct {
	Type     string   `yaml:"type"`
	Name     string   `yaml:"name,omitempty"`
	Anchor   string   `yaml:"anchor,omitempty"`
	Members  []string `yaml:"members,omitempty"`
	Required bool     `yaml:"required,omitempty"`
}

// EffectivePolicy represents the resolved policy for a group after merging defaults.
// Inheritance rules:
//   - strategy: group.policy.selection.strategy > defaults.policy.selection.strategy > "patch"
//   - min_release_age: group.policy.selection.min_release_age > defaults.policy.selection.min_release_age > 0
//   - constraints: group.policy.constraints only (not inherited from defaults)
//   - resolver: group.resolver only (not inherited from defaults)
type EffectivePolicy struct {
	Strategy      Strategy
	MinReleaseAge time.Duration
	Constraints   []Constraint
}

// GetEffectivePolicy returns the merged policy for a group, combining group and default settings.
func (g *Group) GetEffectivePolicy(defaults *Defaults) (EffectivePolicy, error) {
	ep := EffectivePolicy{
		Strategy: g.getStrategy(defaults),
	}

	minAge, err := g.getMinReleaseAge(defaults)
	if err != nil {
		return ep, err
	}
	ep.MinReleaseAge = minAge

	// Constraints are group-specific, not inherited from defaults
	if g.Policy != nil {
		ep.Constraints = g.Policy.Constraints
	}

	return ep, nil
}

// getStrategy returns the effective strategy for a group.
func (g *Group) getStrategy(defaults *Defaults) Strategy {
	if g.Policy != nil && g.Policy.Selection != nil && g.Policy.Selection.Strategy != "" {
		return g.Policy.Selection.Strategy
	}
	if defaults != nil && defaults.Policy != nil && defaults.Policy.Selection != nil && defaults.Policy.Selection.Strategy != "" {
		return defaults.Policy.Selection.Strategy
	}
	return StrategyPatch
}

// getMinReleaseAge returns the effective min_release_age duration for a group.
func (g *Group) getMinReleaseAge(defaults *Defaults) (time.Duration, error) {
	var ageStr string
	if g.Policy != nil && g.Policy.Selection != nil && g.Policy.Selection.MinReleaseAge != "" {
		ageStr = g.Policy.Selection.MinReleaseAge
	} else if defaults != nil && defaults.Policy != nil && defaults.Policy.Selection != nil {
		ageStr = defaults.Policy.Selection.MinReleaseAge
	}
	if ageStr == "" {
		return 0, nil
	}
	return ParseDuration(ageStr)
}

// GetResolver returns the resolver for a group.
// Note: Resolver is group-specific and not inherited from defaults.
func (g *Group) GetResolver() string {
	return g.Resolver
}
