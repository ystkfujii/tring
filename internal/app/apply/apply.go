package apply

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ystkfujii/tring/internal/config"
	"github.com/ystkfujii/tring/internal/domain/model"
	"github.com/ystkfujii/tring/internal/domain/planner"
	"github.com/ystkfujii/tring/internal/output/render"
	"github.com/ystkfujii/tring/pkg/impl/resolver"
	"github.com/ystkfujii/tring/pkg/impl/sources"
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
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return ValidationError{fmt.Errorf("failed to load config: %w", err)}
	}

	validator := config.NewValidator(sources.RegisteredTypes(), resolver.RegisteredTypes())
	if errs := validator.Validate(cfg); !errs.IsEmpty() {
		return ValidationError{fmt.Errorf("config validation failed: %w", errs)}
	}

	if err := config.ValidateGroupExists(cfg, opts.GroupName); err != nil {
		return ValidationError{err}
	}

	group, err := cfg.FindGroup(opts.GroupName)
	if err != nil {
		return ValidationError{err}
	}

	basePath, err := filepath.Abs(filepath.Dir(opts.ConfigPath))
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	srcs, err := buildSources(group.Sources, basePath)
	if err != nil {
		return fmt.Errorf("failed to build sources: %w", err)
	}

	res, err := buildResolver(group.GetResolver(), group.ResolverConfig)
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

	effectivePolicy, err := group.GetEffectivePolicy(cfg.Defaults)
	if err != nil {
		return ValidationError{fmt.Errorf("invalid policy: %w", err)}
	}

	p, err := planner.New(planner.Options{
		Resolver:     res,
		Strategy:     effectivePolicy.Strategy,
		MinAge:       effectivePolicy.MinReleaseAge,
		Selectors:    group.Selectors,
		Constraints:  effectivePolicy.Constraints,
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
		factory, err := sources.Get(raw.Type)
		if err != nil {
			return nil, err
		}

		src, err := factory.Create(raw.Config, basePath)
		if err != nil {
			return nil, fmt.Errorf("failed to create source %q: %w", raw.Type, err)
		}

		srcs = append(srcs, src)
	}

	return srcs, nil
}

func buildResolver(resolverType string, resolverConfig map[string]interface{}) (model.Resolver, error) {
	if resolverType == "" {
		return nil, fmt.Errorf("no resolver specified")
	}

	factory, err := resolver.Get(resolverType)
	if err != nil {
		return nil, err
	}

	return factory.Create(resolverConfig)
}
