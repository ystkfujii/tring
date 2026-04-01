package planner

import (
	"context"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
)

type stubResolver struct {
	resolveFunc func(context.Context, model.Dependency) (model.Candidates, error)
}

func (s *stubResolver) Kind() string {
	return "stub"
}

func (s *stubResolver) Resolve(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
	return s.resolveFunc(ctx, dep)
}

func TestPlanCachesByResolveCacheKey(t *testing.T) {
	resolveCalls := 0
	resolver := &stubResolver{
		resolveFunc: func(ctx context.Context, dep model.Dependency) (model.Candidates, error) {
			resolveCalls++

			targetVersion := "v1.0.1"
			if dep.Metadata[model.MetadataResolutionRegistryRef] == "ref-b" {
				targetVersion = "v1.0.2"
			}

			return model.Candidates{
				Items: []model.Candidate{
					{
						Version:    semver.MustParse(targetVersion),
						ReleasedAt: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
					},
				},
			}, nil
		},
	}

	p, err := New(Options{
		Resolver: resolver,
		Strategy: config.StrategyPatch,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	deps := []model.Dependency{
		{
			Name:    "kubernetes/kubectl",
			Version: semver.MustParse("v1.0.0"),
			Metadata: map[string]string{
				model.MetadataResolutionObjectKind:   "package",
				model.MetadataResolutionRegistryType: "standard",
				model.MetadataResolutionRegistryRef:  "ref-a",
			},
		},
		{
			Name:    "kubernetes/kubectl",
			Version: semver.MustParse("v1.0.0"),
			Metadata: map[string]string{
				model.MetadataResolutionObjectKind:   "package",
				model.MetadataResolutionRegistryType: "standard",
				model.MetadataResolutionRegistryRef:  "ref-b",
			},
		},
	}

	changes, err := p.Plan(context.Background(), deps)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if resolveCalls != 2 {
		t.Fatalf("Resolve() called %d times, want 2", resolveCalls)
	}
	if got := changes[0].TargetVersion.Original(); got != "v1.0.1" {
		t.Fatalf("changes[0].TargetVersion = %q, want %q", got, "v1.0.1")
	}
	if got := changes[1].TargetVersion.Original(); got != "v1.0.2" {
		t.Fatalf("changes[1].TargetVersion = %q, want %q", got, "v1.0.2")
	}
}

func TestGenerateDiffLinkUsesRawVersionMetadata(t *testing.T) {
	link := generateDiffLink(
		model.Dependency{
			Version: semver.MustParse("v5.8.0"),
			Metadata: map[string]string{
				model.MetadataVersionRaw: "kustomize/v5.8.0",
			},
		},
		&model.Candidate{
			Version: semver.MustParse("v5.8.1"),
			Metadata: map[string]string{
				model.MetadataRepoURL:    "https://github.com/kubernetes-sigs/kustomize",
				model.MetadataVersionRaw: "kustomize/v5.8.1",
			},
		},
	)

	want := "https://github.com/kubernetes-sigs/kustomize/compare/kustomize/v5.8.0...kustomize/v5.8.1"
	if link != want {
		t.Fatalf("generateDiffLink() = %q, want %q", link, want)
	}
}
