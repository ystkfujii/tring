package apply

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/internal/domain/planner"
	"github.com/ystkfujii/tring/internal/output/render"
	"github.com/ystkfujii/tring/pkg/impl/resolver/githubrelease"
	"github.com/ystkfujii/tring/pkg/impl/resolver/goproxy"
	"github.com/ystkfujii/tring/pkg/impl/resolver/gotoolchain"
	"github.com/ystkfujii/tring/pkg/impl/sources/envfile"
	"github.com/ystkfujii/tring/pkg/impl/sources/githubaction"
	"github.com/ystkfujii/tring/pkg/impl/sources/gomod"
)

// Options configures the apply executor.
type Options struct {
	ConfigPath   string
	GroupName    string
	DryRun       bool
	ShowDiffLink bool
	Output       io.Writer
}

// ValidationError is returned when validation fails.
type ValidationError struct {
	Err error
}

func (e ValidationError) Error() string {
	return e.Err.Error()
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

// IsValidationError returns true if the error is a validation error.
func IsValidationError(err error) bool {
	var ve ValidationError
	return errors.As(err, &ve)
}

// Run executes the apply command.
func Run(ctx context.Context, opts Options) error {
	cfg, err := config.LoadResolved(opts.ConfigPath)
	if err != nil {
		return ValidationError{fmt.Errorf("failed to load config: %w", err)}
	}

	group, err := cfg.Group(opts.GroupName)
	if err != nil {
		return ValidationError{err}
	}

	srcs, err := buildSources(group.Sources, cfg.BasePath)
	if err != nil {
		return fmt.Errorf("failed to build sources: %w", err)
	}

	res, err := buildResolver(group.Resolver, group.ResolverConfig)
	if err != nil {
		return fmt.Errorf("failed to build resolver: %w", err)
	}

	var allDeps []model.Dependency
	for _, src := range srcs {
		deps, err := src.Extract(ctx)
		if err != nil {
			return fmt.Errorf("failed to extract dependencies: %w", err)
		}
		allDeps = append(allDeps, deps...)
	}

	p, err := planner.New(planner.Options{
		Resolver:     res,
		Strategy:     group.Strategy,
		MinAge:       group.MinReleaseAge,
		Selectors:    group.Selectors,
		Constraints:  group.Constraints,
		ShowDiffLink: opts.ShowDiffLink,
	})
	if err != nil {
		return fmt.Errorf("failed to create planner: %w", err)
	}

	changes, err := p.Plan(ctx, allDeps)
	if err != nil {
		return fmt.Errorf("failed to plan changes: %w", err)
	}

	changeSet := model.ChangeSet{
		GroupName: opts.GroupName,
		Changes:   changes,
	}

	renderer := render.NewTerminalRenderer(opts.Output, opts.ShowDiffLink)

	if opts.DryRun {
		renderer.RenderDryRunHeader()
		renderer.Render(changeSet)
		return nil
	}

	renderer.RenderApplyHeader()
	renderer.Render(changeSet)

	updates := changeSet.Updates()
	if len(updates) == 0 {
		return nil
	}

	for _, src := range srcs {
		if err := src.Apply(ctx, updates); err != nil {
			return fmt.Errorf("failed to apply changes: %w", err)
		}
	}

	renderer.RenderApplySuccess(len(updates))
	return nil
}

func buildSources(rawSources []config.RawSource, basePath string) ([]model.Source, error) {
	var srcs []model.Source

	for _, raw := range rawSources {
		src, err := buildSource(raw, basePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create source %q: %w", raw.Type, err)
		}

		srcs = append(srcs, src)
	}

	return srcs, nil
}

func buildSource(raw config.RawSource, basePath string) (model.Source, error) {
	switch raw.Type {
	case gomod.Kind:
		return gomod.NewSource(raw.Config, basePath)
	case envfile.Kind:
		return envfile.NewSource(raw.Config, basePath)
	case githubaction.Kind:
		return githubaction.NewSource(raw.Config, basePath)
	default:
		return nil, fmt.Errorf("unknown source type: %q", raw.Type)
	}
}

func buildResolver(resolverType string, resolverConfig map[string]interface{}) (model.Resolver, error) {
	if resolverType == "" {
		return nil, fmt.Errorf("no resolver specified")
	}

	switch resolverType {
	case goproxy.Kind:
		return goproxy.NewResolver(resolverConfig)
	case githubrelease.Kind:
		return githubrelease.NewResolver(resolverConfig)
	case gotoolchain.Kind:
		return gotoolchain.NewResolver(resolverConfig)
	default:
		return nil, fmt.Errorf("unknown resolver type: %q", resolverType)
	}
}
