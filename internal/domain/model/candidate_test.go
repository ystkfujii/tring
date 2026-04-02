package model_test

import (
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

func mustParse(s string) *semver.Version {
	v, err := semver.NewVersion(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestCandidatesFilterStable(t *testing.T) {
	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0")},
			{Version: mustParse("v1.1.0-alpha")},
			{Version: mustParse("v1.1.0")},
			{Version: mustParse("v1.2.0-beta.1")},
		},
	}

	stable := c.FilterStable()
	if len(stable.Items) != 2 {
		t.Errorf("FilterStable() returned %d items, want 2", len(stable.Items))
	}

	for _, item := range stable.Items {
		if item.Version.Prerelease() != "" {
			t.Errorf("FilterStable() included prerelease %s", item.Version.Original())
		}
	}
}

func TestCandidatesFilterByAge(t *testing.T) {
	now := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0"), ReleasedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},  // 14 days old
			{Version: mustParse("v1.1.0"), ReleasedAt: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)}, // 5 days old
			{Version: mustParse("v1.2.0"), ReleasedAt: time.Date(2024, 1, 14, 0, 0, 0, 0, time.UTC)}, // 1 day old
		},
	}

	// Filter to only versions at least 7 days old
	filtered := c.FilterByAge(7*24*time.Hour, now)
	if len(filtered.Items) != 1 {
		t.Errorf("FilterByAge(7d) returned %d items, want 1", len(filtered.Items))
	}
	if filtered.Items[0].Version.String() != "1.0.0" {
		t.Errorf("FilterByAge(7d) returned %s, want v1.0.0", filtered.Items[0].Version.Original())
	}

	// Filter to only versions at least 3 days old
	filtered = c.FilterByAge(3*24*time.Hour, now)
	if len(filtered.Items) != 2 {
		t.Errorf("FilterByAge(3d) returned %d items, want 2", len(filtered.Items))
	}
}

func TestCandidatesFilterByAge_ExcludesUnknownTimestamp(t *testing.T) {
	now := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0"), ReleasedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},  // 14 days old
			{Version: mustParse("v1.1.0"), ReleasedAt: time.Time{}},                                  // Unknown timestamp (zero value)
			{Version: mustParse("v1.2.0"), ReleasedAt: time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)}, // 5 days old
		},
	}

	// Filter to versions at least 3 days old
	// v1.1.0 has unknown timestamp (zero time.Time), so it should be excluded
	filtered := c.FilterByAge(3*24*time.Hour, now)

	if len(filtered.Items) != 2 {
		t.Errorf("FilterByAge(3d) returned %d items, want 2 (unknown timestamp should be excluded)", len(filtered.Items))
	}

	// Verify v1.1.0 is not in the result
	for _, item := range filtered.Items {
		if item.Version.String() == "1.1.0" {
			t.Errorf("FilterByAge should exclude v1.1.0 with unknown timestamp")
		}
	}
}

func TestCandidatesFilterByAge_AllUnknownTimestamps(t *testing.T) {
	now := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0"), ReleasedAt: time.Time{}}, // Unknown timestamp
			{Version: mustParse("v1.1.0"), ReleasedAt: time.Time{}}, // Unknown timestamp
		},
	}

	// When all candidates have unknown timestamps, all should be filtered out
	filtered := c.FilterByAge(1*24*time.Hour, now)

	if len(filtered.Items) != 0 {
		t.Errorf("FilterByAge() returned %d items, want 0 (all unknown timestamps should be excluded)", len(filtered.Items))
	}
}

func TestCandidatesLatest(t *testing.T) {
	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0")},
			{Version: mustParse("v1.2.0")},
			{Version: mustParse("v1.1.0")},
		},
	}

	latest := c.Latest()
	if latest == nil {
		t.Fatal("Latest() returned nil")
	}
	if latest.Version.String() != "1.2.0" {
		t.Errorf("Latest() = %s, want v1.2.0", latest.Version.Original())
	}
}

func TestCandidatesFilterSameMajorMinor(t *testing.T) {
	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0")},
			{Version: mustParse("v1.0.1")},
			{Version: mustParse("v1.0.2")},
			{Version: mustParse("v1.1.0")},
			{Version: mustParse("v2.0.0")},
		},
	}

	current := mustParse("v1.0.0")
	filtered := c.FilterSameMajorMinor(current)

	if len(filtered.Items) != 3 {
		t.Errorf("FilterSameMajorMinor() returned %d items, want 3", len(filtered.Items))
	}

	for _, item := range filtered.Items {
		if item.Version.Major() != 1 || item.Version.Minor() != 0 {
			t.Errorf("FilterSameMajorMinor() included %s", item.Version.Original())
		}
	}
}

func TestCandidatesFilterSameMajor(t *testing.T) {
	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0")},
			{Version: mustParse("v1.1.0")},
			{Version: mustParse("v1.2.0")},
			{Version: mustParse("v2.0.0")},
		},
	}

	current := mustParse("v1.0.0")
	filtered := c.FilterSameMajor(current)

	if len(filtered.Items) != 3 {
		t.Errorf("FilterSameMajor() returned %d items, want 3", len(filtered.Items))
	}

	for _, item := range filtered.Items {
		if item.Version.Major() != 1 {
			t.Errorf("FilterSameMajor() included %s", item.Version.Original())
		}
	}
}

func TestCandidatesFilterByStability(t *testing.T) {
	c := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("v1.0.0")},
			{Version: mustParse("v1.1.0-alpha")},
			{Version: mustParse("v1.1.0-beta")},
			{Version: mustParse("v1.1.0")},
			{Version: mustParse("v1.2.0-rc.1")},
		},
	}

	t.Run("stable_current_filters_to_stable_only", func(t *testing.T) {
		// When current is stable, only stable candidates are returned
		current := mustParse("v1.0.0")
		filtered := c.FilterByStability(current)
		if len(filtered.Items) != 2 {
			t.Errorf("FilterByStability(stable) returned %d items, want 2", len(filtered.Items))
		}
		for _, item := range filtered.Items {
			if item.Version.Prerelease() != "" {
				t.Errorf("FilterByStability(stable) included prerelease %s", item.Version.Original())
			}
		}
	})

	t.Run("prerelease_current_includes_prereleases", func(t *testing.T) {
		// When current is prerelease, prereleases are also included
		current := mustParse("v1.1.0-alpha")
		filtered := c.FilterByStability(current)
		if len(filtered.Items) != 5 {
			t.Errorf("FilterByStability(prerelease) returned %d items, want 5", len(filtered.Items))
		}
	})

	t.Run("nil_current_filters_to_stable_only", func(t *testing.T) {
		// When current is nil, default to stable-only behavior
		filtered := c.FilterByStability(nil)
		if len(filtered.Items) != 2 {
			t.Errorf("FilterByStability(nil) returned %d items, want 2", len(filtered.Items))
		}
	})
}
