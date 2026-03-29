package constraint

import (
	"github.com/ystkfujii/tring/internal/domain/model"
)

// Constraint applies version constraints to planned changes.
type Constraint interface {
	// Name returns the constraint name for logging/debugging
	Name() string

	// Apply applies the constraint to a set of planned changes.
	// It may modify the changes in place, including setting SkipReason
	// or adjusting TargetVersion.
	Apply(changes []model.PlannedChange, candidatesMap map[string]model.Candidates) error
}
