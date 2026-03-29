package planner

import (
	"path"
	"strings"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
)

// Selector filters dependencies based on include/exclude patterns.
type Selector struct {
	includePatterns []string
	excludePatterns []string
}

// NewSelector creates a new Selector from config selectors.
func NewSelector(s *config.Selectors) *Selector {
	sel := &Selector{}
	if s == nil {
		return sel
	}
	if s.Include != nil {
		sel.includePatterns = s.Include.ModulePatterns
	}
	if s.Exclude != nil {
		sel.excludePatterns = s.Exclude.ModulePatterns
	}
	return sel
}

// Match returns true if the dependency matches the selector criteria.
// A dependency is included if:
// - It matches any include pattern (or no include patterns are defined)
// - It does not match any exclude pattern
func (s *Selector) Match(dep model.Dependency) bool {
	// Check exclude patterns first
	for _, pattern := range s.excludePatterns {
		if matchPattern(pattern, dep.Name) {
			return false
		}
	}

	// If no include patterns, include everything not excluded
	if len(s.includePatterns) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range s.includePatterns {
		if matchPattern(pattern, dep.Name) {
			return true
		}
	}

	return false
}

// matchPattern matches a module name against a glob pattern.
// Supports '*' wildcard matching.
// - "*" matches everything
// - "k8s.io/*" matches any module starting with "k8s.io/"
// - Uses path.Match for more complex patterns
func matchPattern(pattern, name string) bool {
	// Special case: "*" matches everything
	if pattern == "*" {
		return true
	}

	// Handle patterns like "k8s.io/*" which should match "k8s.io/api"
	// path.Match doesn't handle "/" as part of * matching
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(name, prefix+"/")
	}

	// Use path.Match for other patterns
	matched, err := path.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}
