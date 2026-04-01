package config

import (
	"fmt"
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

// ResolvedConfig is the normalized configuration used by the application layer.
type ResolvedConfig struct {
	BasePath string
	Groups   []ResolvedGroup

	groupIndex map[string]int
}

// ResolvedGroup contains a group after defaults and policy have been resolved.
type ResolvedGroup struct {
	Name           string
	Description    string
	Resolver       string
	ResolverConfig map[string]interface{}
	Sources        []RawSource
	Selectors      *Selectors
	Strategy       Strategy
	MinReleaseAge  time.Duration
	Constraints    []Constraint
}

// Group returns the named resolved group.
func (c *ResolvedConfig) Group(name string) (*ResolvedGroup, error) {
	index, ok := c.groupIndex[name]
	if !ok {
		return nil, fmt.Errorf("group %q not found", name)
	}
	return &c.Groups[index], nil
}

func (c *Config) Resolve(basePath string) (*ResolvedConfig, error) {
	resolved := &ResolvedConfig{
		BasePath:   basePath,
		Groups:     make([]ResolvedGroup, 0, len(c.Groups)),
		groupIndex: make(map[string]int, len(c.Groups)),
	}

	for _, group := range c.Groups {
		resolvedGroup, err := resolveGroup(group, c.Defaults)
		if err != nil {
			return nil, err
		}
		resolved.groupIndex[resolvedGroup.Name] = len(resolved.Groups)
		resolved.Groups = append(resolved.Groups, resolvedGroup)
	}

	return resolved, nil
}

func resolveGroup(group Group, defaults *Defaults) (ResolvedGroup, error) {
	minAge, err := resolveMinReleaseAge(group, defaults)
	if err != nil {
		return ResolvedGroup{}, err
	}

	constraints := []Constraint(nil)
	if group.Policy != nil {
		constraints = group.Policy.Constraints
	}

	return ResolvedGroup{
		Name:           group.Name,
		Description:    group.Description,
		Resolver:       group.Resolver,
		ResolverConfig: group.ResolverConfig,
		Sources:        group.Sources,
		Selectors:      group.Selectors,
		Strategy:       resolveStrategy(group, defaults),
		MinReleaseAge:  minAge,
		Constraints:    constraints,
	}, nil
}

func resolveStrategy(group Group, defaults *Defaults) Strategy {
	if group.Policy != nil && group.Policy.Selection != nil && group.Policy.Selection.Strategy != "" {
		return group.Policy.Selection.Strategy
	}
	if defaults != nil && defaults.Policy != nil && defaults.Policy.Selection != nil && defaults.Policy.Selection.Strategy != "" {
		return defaults.Policy.Selection.Strategy
	}
	return StrategyPatch
}

func resolveMinReleaseAge(group Group, defaults *Defaults) (time.Duration, error) {
	var ageStr string
	if group.Policy != nil && group.Policy.Selection != nil && group.Policy.Selection.MinReleaseAge != "" {
		ageStr = group.Policy.Selection.MinReleaseAge
	} else if defaults != nil && defaults.Policy != nil && defaults.Policy.Selection != nil {
		ageStr = defaults.Policy.Selection.MinReleaseAge
	}
	if ageStr == "" {
		return 0, nil
	}
	return ParseDuration(ageStr)
}
