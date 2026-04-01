package config

import (
	"fmt"
	"strings"

	"github.com/ystkfujii/tring/pkg/impl/resolver/githubrelease"
	"github.com/ystkfujii/tring/pkg/impl/resolver/goproxy"
	"github.com/ystkfujii/tring/pkg/impl/resolver/gotoolchain"
	"github.com/ystkfujii/tring/pkg/impl/sources/envfile"
	"github.com/ystkfujii/tring/pkg/impl/sources/githubaction"
	"github.com/ystkfujii/tring/pkg/impl/sources/gomod"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// IsEmpty returns true if there are no errors.
func (e ValidationErrors) IsEmpty() bool {
	return len(e) == 0
}

// Validator validates configuration.
type Validator struct{}

// NewValidator creates a Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates the entire configuration.
func (v *Validator) Validate(cfg *Config) ValidationErrors {
	var errs ValidationErrors

	if cfg.Version != 1 {
		errs = append(errs, ValidationError{
			Field:   "version",
			Message: fmt.Sprintf("unsupported config version: %d (supported: 1)", cfg.Version),
		})
	}

	if len(cfg.Groups) == 0 {
		errs = append(errs, ValidationError{
			Field:   "groups",
			Message: "at least one group must be defined",
		})
	}

	groupNames := make(map[string]bool)
	for i, g := range cfg.Groups {
		if groupNames[g.Name] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("groups[%d].name", i),
				Message: fmt.Sprintf("duplicate group name: %q", g.Name),
			})
		}
		groupNames[g.Name] = true

		groupErrs := v.validateGroup(&g, i)
		errs = append(errs, groupErrs...)
	}

	if cfg.Defaults != nil && cfg.Defaults.Policy != nil {
		errs = append(errs, v.validatePolicy(cfg.Defaults.Policy, "defaults.policy")...)
	}

	return errs
}

func (v *Validator) validateGroup(g *Group, index int) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("groups[%d]", index)

	if g.Name == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".name",
			Message: "group name is required",
		})
	}

	if len(g.Sources) == 0 {
		errs = append(errs, ValidationError{
			Field:   prefix + ".sources",
			Message: "at least one source must be defined",
		})
	}

	for i, src := range g.Sources {
		errs = append(errs, v.validateSource(&src, fmt.Sprintf("%s.sources[%d]", prefix, i))...)
	}

	resolverType := g.Resolver
	if resolverType != "" && !isKnownResolverType(resolverType) {
		errs = append(errs, ValidationError{
			Field:   prefix + ".resolver",
			Message: fmt.Sprintf("unknown resolver type: %q", resolverType),
		})
	}

	if g.ResolverConfig != nil && resolverType != "" {
		if err := validateResolverConfig(resolverType, g.ResolverConfig); err != nil {
			errs = append(errs, ValidationError{
				Field:   prefix + ".resolver_config",
				Message: err.Error(),
			})
		}
	}

	if g.Policy != nil {
		errs = append(errs, v.validatePolicy(g.Policy, prefix+".policy")...)
		errs = append(errs, v.validatePolicyResolverCompatibility(g.Policy, resolverType, prefix)...)
	}

	if g.Selectors != nil {
		errs = append(errs, v.validateSelectors(g.Selectors, prefix+".selectors")...)
	}

	return errs
}

func (v *Validator) validateSelectors(s *Selectors, prefix string) ValidationErrors {
	var errs ValidationErrors

	if s.Include != nil {
		for i, pattern := range s.Include.ModulePatterns {
			if err := validateSelectorPattern(pattern); err != nil {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.include.module_patterns[%d]", prefix, i),
					Message: err.Error(),
				})
			}
		}
	}

	if s.Exclude != nil {
		for i, pattern := range s.Exclude.ModulePatterns {
			if err := validateSelectorPattern(pattern); err != nil {
				errs = append(errs, ValidationError{
					Field:   fmt.Sprintf("%s.exclude.module_patterns[%d]", prefix, i),
					Message: err.Error(),
				})
			}
		}
	}

	return errs
}

// validateSelectorPattern validates a selector pattern.
// Supported patterns:
//   - "*" (matches everything)
//   - "prefix/*" (matches modules starting with prefix/)
//   - "exact/match" (exact match, no wildcards)
//
// Unsupported patterns:
//   - "**" (recursive glob)
//   - "**/suffix" (recursive glob with suffix)
//   - "prefix/**/suffix" (recursive glob in the middle)
func validateSelectorPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("empty pattern is not allowed")
	}

	// Reject "**" patterns (recursive glob)
	if strings.Contains(pattern, "**") {
		return fmt.Errorf("recursive glob '**' is not supported; use 'prefix/*' instead")
	}

	// Check for invalid glob characters (allow only single *)
	// Patterns like "*foo" or "foo*bar" are not supported
	starCount := strings.Count(pattern, "*")
	if starCount > 1 {
		return fmt.Errorf("multiple wildcards are not supported")
	}

	if starCount == 1 {
		// Only allow "*" alone or "prefix/*" patterns
		if pattern == "*" {
			return nil
		}
		if strings.HasSuffix(pattern, "/*") {
			prefix := strings.TrimSuffix(pattern, "/*")
			if prefix == "" {
				return fmt.Errorf("invalid pattern %q: prefix required before '/*'", pattern)
			}
			if strings.Contains(prefix, "*") {
				return fmt.Errorf("invalid pattern %q: wildcard only allowed at the end", pattern)
			}
			return nil
		}
		return fmt.Errorf("invalid pattern %q: wildcard only supported as '*' or 'prefix/*'", pattern)
	}

	// No wildcard - exact match pattern
	return nil
}

func (v *Validator) validateSource(src *RawSource, prefix string) ValidationErrors {
	var errs ValidationErrors

	if src.Type == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".type",
			Message: "source type is required",
		})
		return errs
	}

	if !isKnownSourceType(src.Type) {
		errs = append(errs, ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("unknown source type: %q", src.Type),
		})
		return errs
	}

	if err := validateSourceConfig(src.Type, src.Config); err != nil {
		errs = append(errs, ValidationError{
			Field:   prefix + ".config",
			Message: err.Error(),
		})
	}

	return errs
}

func (v *Validator) validatePolicy(p *Policy, prefix string) ValidationErrors {
	var errs ValidationErrors

	if p.Selection != nil {
		strategy := p.Selection.Strategy
		if strategy != "" && strategy != StrategyPatch && strategy != StrategyMinor && strategy != StrategyMajor {
			errs = append(errs, ValidationError{
				Field:   prefix + ".selection.strategy",
				Message: fmt.Sprintf("unknown strategy: %q (supported: patch, minor, major)", strategy),
			})
		}

		if p.Selection.MinReleaseAge != "" {
			if _, err := ParseDuration(p.Selection.MinReleaseAge); err != nil {
				errs = append(errs, ValidationError{
					Field:   prefix + ".selection.min_release_age",
					Message: err.Error(),
				})
			}
		}
	}

	for i, c := range p.Constraints {
		errs = append(errs, v.validateConstraint(&c, fmt.Sprintf("%s.constraints[%d]", prefix, i))...)
	}

	return errs
}

func (v *Validator) validateConstraint(c *Constraint, prefix string) ValidationErrors {
	var errs ValidationErrors

	if c.Type != "align" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("unknown constraint type: %q (supported: align)", c.Type),
		})
		return errs
	}

	if c.Anchor == "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".anchor",
			Message: "anchor is required for align constraint",
		})
	}

	if len(c.Members) == 0 {
		errs = append(errs, ValidationError{
			Field:   prefix + ".members",
			Message: "members is required for align constraint",
		})
	}

	return errs
}

// validatePolicyResolverCompatibility validates that policy settings are compatible
// with the specified resolver type.
func (v *Validator) validatePolicyResolverCompatibility(p *Policy, resolverType, prefix string) ValidationErrors {
	var errs ValidationErrors

	// gotoolchain resolver doesn't support min_release_age because go.dev API
	// doesn't provide release timestamps
	if resolverType == "gotoolchain" && p.Selection != nil && p.Selection.MinReleaseAge != "" {
		errs = append(errs, ValidationError{
			Field:   prefix + ".policy.selection.min_release_age",
			Message: fmt.Sprintf("min_release_age is not supported with resolver %q", resolverType),
		})
	}

	return errs
}

func isKnownSourceType(sourceType string) bool {
	switch sourceType {
	case gomod.Kind, envfile.Kind, githubaction.Kind:
		return true
	default:
		return false
	}
}

func isKnownResolverType(resolverType string) bool {
	switch resolverType {
	case goproxy.Kind, githubrelease.Kind, gotoolchain.Kind:
		return true
	default:
		return false
	}
}

func validateSourceConfig(sourceType string, raw map[string]interface{}) error {
	switch sourceType {
	case gomod.Kind:
		return gomod.ValidateConfig(raw)
	case envfile.Kind:
		return envfile.ValidateConfig(raw)
	case githubaction.Kind:
		return githubaction.ValidateConfig(raw)
	default:
		return fmt.Errorf("unknown source type: %q", sourceType)
	}
}

func validateResolverConfig(resolverType string, raw map[string]interface{}) error {
	switch resolverType {
	case goproxy.Kind:
		return goproxy.ValidateConfig(raw)
	case githubrelease.Kind:
		return githubrelease.ValidateConfig(raw)
	case gotoolchain.Kind:
		return gotoolchain.ValidateConfig(raw)
	default:
		return fmt.Errorf("unknown resolver type: %q", resolverType)
	}
}
