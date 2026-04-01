package planner

import (
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
)

func mustParse(v string) *semver.Version {
	return semver.MustParse(v)
}

func TestApplyStrategy(t *testing.T) {
	candidates := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("1.0.0"), ReleasedAt: time.Now()},
			{Version: mustParse("1.0.1"), ReleasedAt: time.Now()},
			{Version: mustParse("1.1.0"), ReleasedAt: time.Now()},
			{Version: mustParse("1.1.1"), ReleasedAt: time.Now()},
			{Version: mustParse("2.0.0"), ReleasedAt: time.Now()},
			{Version: mustParse("2.1.0"), ReleasedAt: time.Now()},
		},
	}

	tests := []struct {
		name       string
		strategy   config.Strategy
		current    *semver.Version
		wantCount  int
		wantLatest string
	}{
		{
			name:       "patch strategy - same major.minor only",
			strategy:   config.StrategyPatch,
			current:    mustParse("1.0.0"),
			wantCount:  2, // 1.0.0, 1.0.1
			wantLatest: "1.0.1",
		},
		{
			name:       "minor strategy - same major only",
			strategy:   config.StrategyMinor,
			current:    mustParse("1.0.0"),
			wantCount:  4, // 1.0.0, 1.0.1, 1.1.0, 1.1.1
			wantLatest: "1.1.1",
		},
		{
			name:       "major strategy - all candidates",
			strategy:   config.StrategyMajor,
			current:    mustParse("1.0.0"),
			wantCount:  6, // all
			wantLatest: "2.1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Planner{strategy: tt.strategy}
			result := p.applyStrategy(candidates, tt.current)

			if len(result.Items) != tt.wantCount {
				t.Errorf("applyStrategy() returned %d candidates, want %d", len(result.Items), tt.wantCount)
			}

			latest := result.Latest()
			if latest == nil {
				t.Fatal("applyStrategy() returned no latest candidate")
			}
			if latest.Version.String() != tt.wantLatest {
				t.Errorf("applyStrategy() latest = %s, want %s", latest.Version.String(), tt.wantLatest)
			}
		})
	}
}

func TestApplyStrategy_NilCurrent(t *testing.T) {
	candidates := model.Candidates{
		Items: []model.Candidate{
			{Version: mustParse("1.0.0"), ReleasedAt: time.Now()},
			{Version: mustParse("2.0.0"), ReleasedAt: time.Now()},
		},
	}

	tests := []struct {
		name     string
		strategy config.Strategy
	}{
		{name: "patch with nil current", strategy: config.StrategyPatch},
		{name: "minor with nil current", strategy: config.StrategyMinor},
		{name: "major with nil current", strategy: config.StrategyMajor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Planner{strategy: tt.strategy}
			result := p.applyStrategy(candidates, nil)

			// When current is nil, all candidates should be returned
			if len(result.Items) != len(candidates.Items) {
				t.Errorf("applyStrategy() with nil current returned %d candidates, want %d",
					len(result.Items), len(candidates.Items))
			}
		})
	}
}
