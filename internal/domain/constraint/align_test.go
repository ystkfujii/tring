package constraint_test

import (
	"testing"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/constraint"
	"github.com/ystkfujii/tring/internal/domain/model"
)

func mustParse(s string) *semver.Version {
	v, err := semver.NewVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestAlignConstraint_AnchorAlreadyLatest(t *testing.T) {
	// This test verifies that when the anchor is already at latest version,
	// members are still aligned to the anchor's current version.
	//
	// Scenario:
	// - anchor (k8s.io/apimachinery) is already at v0.28.2 (latest)
	// - member (k8s.io/api) is at v0.28.0 (outdated)
	// - Expected: member should be updated to v0.28.2 (anchor's current version)

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: true,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	// anchor is already_latest (TargetVersion is nil, CurrentVersion is v0.28.2)
	anchorCurrent := mustParse("v0.28.2")
	memberCurrent := mustParse("v0.28.0")

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: anchorCurrent,
			TargetVersion:  nil, // already_latest means no target version
			SkipReason:     model.SkipReasonAlreadyLatest,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: memberCurrent,
			TargetVersion:  mustParse("v0.28.1"), // planner suggested v0.28.1
			SkipReason:     model.SkipReasonNone,
		},
	}

	candidatesMap := map[string]model.Candidates{
		"k8s.io/apimachinery": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
				{Version: mustParse("v0.28.2")},
			},
		},
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
				{Version: mustParse("v0.28.2")},
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}

	// Check member was aligned to anchor's current version
	memberChange := changes[1]
	if memberChange.TargetVersion == nil {
		t.Fatal("member TargetVersion should not be nil")
	}
	if memberChange.TargetVersion.Original() != "v0.28.2" {
		t.Errorf("member TargetVersion = %s, want v0.28.2", memberChange.TargetVersion.Original())
	}
	if memberChange.SkipReason != model.SkipReasonNone {
		t.Errorf("member SkipReason = %s, want none", memberChange.SkipReason)
	}
}

func TestAlignConstraint_AnchorHasTargetVersion(t *testing.T) {
	// When anchor has a target version, members should align to that target version.

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: true,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	anchorCurrent := mustParse("v0.28.0")
	anchorTarget := mustParse("v0.28.2")
	memberCurrent := mustParse("v0.28.0")

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: anchorCurrent,
			TargetVersion:  anchorTarget,
			SkipReason:     model.SkipReasonNone,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: memberCurrent,
			TargetVersion:  mustParse("v0.28.1"),
			SkipReason:     model.SkipReasonNone,
		},
	}

	candidatesMap := map[string]model.Candidates{
		"k8s.io/apimachinery": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
				{Version: mustParse("v0.28.2")},
			},
		},
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
				{Version: mustParse("v0.28.2")},
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}

	// Member should be aligned to anchor's target version (v0.28.2)
	memberChange := changes[1]
	if memberChange.TargetVersion == nil {
		t.Fatal("member TargetVersion should not be nil")
	}
	if memberChange.TargetVersion.Original() != "v0.28.2" {
		t.Errorf("member TargetVersion = %s, want v0.28.2", memberChange.TargetVersion.Original())
	}
}

func TestAlignConstraint_MemberAlreadyAtAnchorVersion(t *testing.T) {
	// When member is already at anchor version, it should be marked as already_latest.

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: true,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	anchorCurrent := mustParse("v0.28.2")
	memberCurrent := mustParse("v0.28.2")

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: anchorCurrent,
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonAlreadyLatest,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: memberCurrent,
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonAlreadyLatest,
		},
	}

	candidatesMap := map[string]model.Candidates{
		"k8s.io/apimachinery": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.2")},
			},
		},
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.2")},
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err != nil {
		t.Fatalf("Apply() returned error: %v", err)
	}

	// Member should remain as already_latest
	memberChange := changes[1]
	if memberChange.SkipReason != model.SkipReasonAlreadyLatest {
		t.Errorf("member SkipReason = %s, want already_latest", memberChange.SkipReason)
	}
}

func TestAlignConstraint_RequiredNoAnchorVersion(t *testing.T) {
	// When anchor has no current version and no target version, required constraint should fail.
	// With pattern A design: error only, changes are not modified.

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: true,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: nil, // No current version
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonResolveError,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: mustParse("v0.28.0"),
			TargetVersion:  mustParse("v0.28.1"),
			SkipReason:     model.SkipReasonNone,
		},
	}

	candidatesMap := map[string]model.Candidates{
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err == nil {
		t.Fatal("Apply() should return error for required constraint with no anchor version")
	}

	// With pattern A: changes should NOT be modified on error
	memberChange := changes[1]
	if memberChange.SkipReason != model.SkipReasonNone {
		t.Errorf("member SkipReason = %s, want none (unchanged)", memberChange.SkipReason)
	}
	if memberChange.TargetVersion == nil || memberChange.TargetVersion.Original() != "v0.28.1" {
		t.Errorf("member TargetVersion should remain v0.28.1 (unchanged)")
	}
}

func TestAlignConstraint_NonRequiredNoAnchorVersion(t *testing.T) {
	// When anchor has no version and constraint is not required, no error should be returned.

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: false,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: nil,
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonResolveError,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: mustParse("v0.28.0"),
			TargetVersion:  mustParse("v0.28.1"),
			SkipReason:     model.SkipReasonNone,
		},
	}

	candidatesMap := map[string]model.Candidates{}

	err = ac.Apply(changes, candidatesMap)
	if err != nil {
		t.Fatalf("Apply() returned unexpected error: %v", err)
	}

	// Member should keep its original target version
	memberChange := changes[1]
	if memberChange.TargetVersion.Original() != "v0.28.1" {
		t.Errorf("member TargetVersion = %s, want v0.28.1", memberChange.TargetVersion.Original())
	}
}

func TestAlignConstraint_RequiredMemberNoCandidateForAnchorVersion(t *testing.T) {
	// When required constraint and member has no candidate for anchor version,
	// it should return an error immediately without modifying changes.

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: true,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	anchorCurrent := mustParse("v0.28.2")

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: anchorCurrent,
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonAlreadyLatest,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: mustParse("v0.28.0"),
			TargetVersion:  mustParse("v0.28.1"),
			SkipReason:     model.SkipReasonNone,
		},
	}

	// k8s.io/api does NOT have v0.28.2 - only has v0.28.0 and v0.28.1
	candidatesMap := map[string]model.Candidates{
		"k8s.io/apimachinery": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.2")},
			},
		},
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
				// v0.28.2 is missing!
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err == nil {
		t.Fatal("Apply() should return error for required constraint when member has no candidate for anchor version")
	}

	// With pattern A: changes should NOT be modified on error
	memberChange := changes[1]
	if memberChange.SkipReason != model.SkipReasonNone {
		t.Errorf("member SkipReason = %s, want none (unchanged)", memberChange.SkipReason)
	}
	if memberChange.TargetVersion == nil || memberChange.TargetVersion.Original() != "v0.28.1" {
		t.Errorf("member TargetVersion should remain v0.28.1 (unchanged)")
	}
}

func TestAlignConstraint_NonRequiredMemberNoCandidateForAnchorVersion(t *testing.T) {
	// When non-required constraint and member has no candidate for anchor version,
	// it should NOT return an error (best effort).

	cfg := config.Constraint{
		Type:     "align",
		Name:     "k8s-align",
		Anchor:   "k8s.io/apimachinery",
		Members:  []string{"k8s.io/apimachinery", "k8s.io/api"},
		Required: false,
	}

	ac, err := constraint.NewAlignConstraint(cfg)
	if err != nil {
		t.Fatalf("failed to create align constraint: %v", err)
	}

	anchorCurrent := mustParse("v0.28.2")

	changes := []model.PlannedChange{
		{
			Dependency:     model.Dependency{Name: "k8s.io/apimachinery"},
			CurrentVersion: anchorCurrent,
			TargetVersion:  nil,
			SkipReason:     model.SkipReasonAlreadyLatest,
		},
		{
			Dependency:     model.Dependency{Name: "k8s.io/api"},
			CurrentVersion: mustParse("v0.28.0"),
			TargetVersion:  mustParse("v0.28.1"),
			SkipReason:     model.SkipReasonNone,
		},
	}

	// k8s.io/api does NOT have v0.28.2
	candidatesMap := map[string]model.Candidates{
		"k8s.io/apimachinery": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.2")},
			},
		},
		"k8s.io/api": {
			Items: []model.Candidate{
				{Version: mustParse("v0.28.0")},
				{Version: mustParse("v0.28.1")},
			},
		},
	}

	err = ac.Apply(changes, candidatesMap)
	if err != nil {
		t.Fatalf("Apply() returned unexpected error: %v", err)
	}

	// Member should keep its original target version (best effort)
	memberChange := changes[1]
	if memberChange.SkipReason != model.SkipReasonNone {
		t.Errorf("member SkipReason = %s, want none", memberChange.SkipReason)
	}
	if memberChange.TargetVersion.Original() != "v0.28.1" {
		t.Errorf("member TargetVersion = %s, want v0.28.1", memberChange.TargetVersion.Original())
	}
}
