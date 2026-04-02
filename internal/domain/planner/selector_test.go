package planner

import (
	"testing"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
)

func TestSelectorMatch(t *testing.T) {
	tests := []struct {
		name      string
		selectors *config.Selectors
		depName   string
		wantMatch bool
	}{
		{
			name:      "nil selectors matches everything",
			selectors: nil,
			depName:   "github.com/foo/bar",
			wantMatch: true,
		},
		{
			name: "wildcard include matches everything",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"*"},
				},
			},
			depName:   "github.com/foo/bar",
			wantMatch: true,
		},
		{
			name: "specific include pattern",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"github.com/foo/*"},
				},
			},
			depName:   "github.com/foo/bar",
			wantMatch: true,
		},
		{
			name: "specific include pattern no match",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"github.com/other/*"},
				},
			},
			depName:   "github.com/foo/bar",
			wantMatch: false,
		},
		{
			name: "exclude pattern",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"*"},
				},
				Exclude: &config.SelectorPatterns{
					ModulePatterns: []string{"k8s.io/*"},
				},
			},
			depName:   "k8s.io/api",
			wantMatch: false,
		},
		{
			name: "exclude does not match",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"*"},
				},
				Exclude: &config.SelectorPatterns{
					ModulePatterns: []string{"k8s.io/*"},
				},
			},
			depName:   "github.com/foo/bar",
			wantMatch: true,
		},
		{
			name: "k8s.io pattern match",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"k8s.io/*"},
				},
			},
			depName:   "k8s.io/client-go",
			wantMatch: true,
		},
		// Dockerfile source normalizes golang -> go, so selector should match "go" not "golang"
		{
			name: "dockerfile normalized name - selector matches go not golang",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"go"},
				},
			},
			depName:   "go", // Dockerfile source normalizes "golang" image to "go" dependency name
			wantMatch: true,
		},
		{
			name: "dockerfile normalized name - selector golang does not match",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"golang"}, // This should NOT match the normalized name
				},
			},
			depName:   "go", // Dockerfile source normalizes "golang" to "go"
			wantMatch: false,
		},
		{
			name: "dockerfile normalized name - exclude works on normalized name",
			selectors: &config.Selectors{
				Include: &config.SelectorPatterns{
					ModulePatterns: []string{"*"},
				},
				Exclude: &config.SelectorPatterns{
					ModulePatterns: []string{"go"}, // Exclude by normalized name
				},
			},
			depName:   "go",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewSelector(tt.selectors)
			dep := model.Dependency{Name: tt.depName}

			got := selector.Match(dep)
			if got != tt.wantMatch {
				t.Errorf("Match(%q) = %v, want %v", tt.depName, got, tt.wantMatch)
			}
		})
	}
}
