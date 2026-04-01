package aqua

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ystkfujii/tring/internal/domain/model"
	aquahelper "github.com/ystkfujii/tring/pkg/impl/aqua"
	"github.com/ystkfujii/tring/pkg/impl/sources"
)

const sourceKind = "aqua"

func init() {
	sources.Register(sourceKind, &Factory{})
}

// Factory creates aqua sources.
type Factory struct{}

// Kind returns the source type.
func (f *Factory) Kind() string {
	return sourceKind
}

// Create creates a new aqua source from configuration map.
func (f *Factory) Create(config map[string]interface{}, basePath string) (model.Source, error) {
	var cfg Config
	if err := decodeConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode aqua config: %w", err)
	}

	paths := make([]string, len(cfg.FilePaths))
	for i, p := range cfg.FilePaths {
		if filepath.IsAbs(p) {
			paths[i] = p
		} else {
			paths[i] = filepath.Join(basePath, p)
		}
	}

	targets := make(map[string]bool, len(cfg.Targets))
	for _, target := range cfg.Targets {
		targets[target] = true
	}

	mode := cfg.UnsupportedVersion
	if mode == "" {
		mode = aquahelper.UnsupportedVersionSkip
	}

	return &Source{
		paths:              paths,
		targets:            targets,
		unsupportedVersion: mode,
	}, nil
}

func decodeConfig(raw map[string]interface{}, cfg *Config) error {
	if raw == nil {
		return nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
}

// Source extracts and updates dependencies from aqua.yaml files.
type Source struct {
	paths              []string
	targets            map[string]bool
	unsupportedVersion string
}

var _ model.Source = (*Source)(nil)

// Kind returns the source type.
func (s *Source) Kind() string {
	return sourceKind
}

// Extract extracts dependencies from all configured aqua.yaml files.
func (s *Source) Extract(ctx context.Context) ([]model.Dependency, error) {
	var deps []model.Dependency

	for _, path := range s.paths {
		fileDeps, err := s.extractFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to extract from %s: %w", path, err)
		}
		deps = append(deps, fileDeps...)
	}

	return deps, nil
}

type registryContext struct {
	firstStandardRef string
	localRegistries  map[string]string
}

func (s *Source) extractFromFile(path string) ([]model.Dependency, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse aqua yaml: %w", err)
	}

	root := documentRoot(&doc)
	if root == nil {
		return nil, fmt.Errorf("aqua yaml root must be a mapping")
	}

	var deps []model.Dependency
	ctx, registryDeps, err := s.extractRegistries(path, aquahelper.MappingValue(root, aquahelper.TargetRegistries))
	if err != nil {
		return nil, err
	}
	deps = append(deps, registryDeps...)

	packageDeps, err := s.extractPackages(path, aquahelper.MappingValue(root, aquahelper.TargetPackages), ctx)
	if err != nil {
		return nil, err
	}
	deps = append(deps, packageDeps...)

	return deps, nil
}

func (s *Source) extractRegistries(path string, node *yaml.Node) (registryContext, []model.Dependency, error) {
	ctx := registryContext{
		localRegistries: make(map[string]string),
	}
	if node == nil {
		return ctx, nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return ctx, nil, fmt.Errorf("registries must be a sequence in %s", path)
	}

	var deps []model.Dependency
	baseDir := filepath.Dir(path)

	for i, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}

		typeNode := aquahelper.MappingValue(item, "type")
		if typeNode == nil {
			continue
		}

		switch typeNode.Value {
		case aquahelper.RegistryTypeStandard:
			refNode := aquahelper.MappingValue(item, "ref")
			if refNode == nil || refNode.Value == "" {
				return ctx, nil, fmt.Errorf("registries[%d].ref is required for standard registry in %s", i, path)
			}
			if ctx.firstStandardRef == "" {
				ctx.firstStandardRef = refNode.Value
			}

			if !s.targets[aquahelper.TargetRegistries] {
				continue
			}

			version, prefix, err := aquahelper.NormalizeVersion(refNode.Value)
			if err != nil {
				skip, handleErr := s.handleUnsupportedVersion(
					fmt.Sprintf("registries[%d].ref in %s", i, path),
					refNode.Value,
					err,
				)
				if handleErr != nil {
					return ctx, nil, handleErr
				}
				if skip {
					continue
				}
			}

			deps = append(deps, model.Dependency{
				Name:       "aquaproj/aqua-registry",
				Version:    version,
				SourceKind: sourceKind,
				FilePath:   path,
				Locator:    registryLocator(i),
				Metadata: map[string]string{
					aquahelper.MetadataObjectKind:    aquahelper.ObjectKindRegistry,
					aquahelper.MetadataRegistryType:  aquahelper.RegistryTypeStandard,
					aquahelper.MetadataRegistryName:  aquahelper.RegistryTypeStandard,
					aquahelper.MetadataVersionField:  aquahelper.VersionFieldRef,
					aquahelper.MetadataRawVersion:    refNode.Value,
					aquahelper.MetadataVersionPrefix: prefix,
					aquahelper.MetadataItemIndex:     strconv.Itoa(i),
				},
			})

		case aquahelper.RegistryTypeLocal:
			nameNode := aquahelper.MappingValue(item, "name")
			pathNode := aquahelper.MappingValue(item, "path")
			if nameNode == nil || nameNode.Value == "" {
				return ctx, nil, fmt.Errorf("registries[%d].name is required for local registry in %s", i, path)
			}
			if pathNode == nil || pathNode.Value == "" {
				return ctx, nil, fmt.Errorf("registries[%d].path is required for local registry %q in %s", i, nameNode.Value, path)
			}
			resolvedPath := pathNode.Value
			if !filepath.IsAbs(resolvedPath) {
				resolvedPath = filepath.Join(baseDir, resolvedPath)
			}
			ctx.localRegistries[nameNode.Value] = resolvedPath
		}
	}

	return ctx, deps, nil
}

func (s *Source) extractPackages(path string, node *yaml.Node, ctx registryContext) ([]model.Dependency, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("packages must be a sequence in %s", path)
	}

	var deps []model.Dependency
	for i, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}

		dep, err := s.extractPackage(path, i, item, ctx)
		if err != nil {
			return nil, err
		}
		if dep != nil {
			deps = append(deps, *dep)
		}
	}

	return deps, nil
}

func (s *Source) extractPackage(path string, index int, node *yaml.Node, ctx registryContext) (*model.Dependency, error) {
	nameNode := aquahelper.MappingValue(node, "name")
	if nameNode == nil || nameNode.Value == "" {
		return nil, nil
	}

	registryType := aquahelper.RegistryTypeStandard
	registryName := aquahelper.RegistryTypeStandard
	metadata := map[string]string{
		aquahelper.MetadataObjectKind: aquahelper.ObjectKindPackage,
		aquahelper.MetadataItemIndex:  strconv.Itoa(index),
	}

	registryNode := aquahelper.MappingValue(node, "registry")
	if registryNode != nil && registryNode.Value != "" {
		registryType = aquahelper.RegistryTypeLocal
		registryName = registryNode.Value
		resolvedPath, ok := ctx.localRegistries[registryName]
		if !ok {
			return nil, fmt.Errorf("packages[%d] in %s references unknown local registry %q", index, path, registryName)
		}
		if _, err := os.Stat(resolvedPath); err != nil {
			return nil, fmt.Errorf("packages[%d] in %s references invalid local registry path %q: %w", index, path, resolvedPath, err)
		}
		metadata[aquahelper.MetadataLocalRegistryPath] = resolvedPath
	} else if ctx.firstStandardRef == "" {
		return nil, fmt.Errorf("packages[%d] in %s requires a standard registry ref", index, path)
	}

	metadata[aquahelper.MetadataRegistryType] = registryType
	metadata[aquahelper.MetadataRegistryName] = registryName
	if ctx.firstStandardRef != "" {
		metadata[aquahelper.MetadataStandardRef] = ctx.firstStandardRef
	}

	packageName := nameNode.Value
	rawVersion := ""
	versionField := ""

	if strings.Contains(nameNode.Value, "@") {
		parts := strings.SplitN(nameNode.Value, "@", 2)
		packageName = parts[0]
		rawVersion = parts[1]
		versionField = aquahelper.VersionFieldNameEmbedded
	} else if versionNode := aquahelper.MappingValue(node, "version"); versionNode != nil && versionNode.Value != "" {
		rawVersion = versionNode.Value
		versionField = aquahelper.VersionFieldVersionField
	} else {
		skip, err := s.handleUnsupportedVersion(
			fmt.Sprintf("packages[%d] in %s", index, path),
			nameNode.Value,
			fmt.Errorf("missing supported version field"),
		)
		if err != nil {
			return nil, err
		}
		if skip {
			return nil, nil
		}
	}

	version, prefix, err := aquahelper.NormalizeVersion(rawVersion)
	if err != nil {
		skip, handleErr := s.handleUnsupportedVersion(
			fmt.Sprintf("packages[%d] in %s", index, path),
			rawVersion,
			err,
		)
		if handleErr != nil {
			return nil, handleErr
		}
		if skip {
			return nil, nil
		}
	}

	metadata[aquahelper.MetadataPackageName] = packageName
	metadata[aquahelper.MetadataVersionField] = versionField
	metadata[aquahelper.MetadataRawVersion] = rawVersion
	metadata[aquahelper.MetadataVersionPrefix] = prefix

	return &model.Dependency{
		Name:       packageName,
		Version:    version,
		SourceKind: sourceKind,
		FilePath:   path,
		Locator:    packageLocator(index),
		Metadata:   metadata,
	}, nil
}

func (s *Source) handleUnsupportedVersion(location, raw string, cause error) (bool, error) {
	if s.unsupportedVersion == aquahelper.UnsupportedVersionSkip {
		return true, nil
	}
	return false, fmt.Errorf("%s has unsupported version %q: %w", location, raw, cause)
}

// Apply applies the planned changes to the aqua.yaml files.
func (s *Source) Apply(ctx context.Context, changes []model.PlannedChange) error {
	changesByFile := make(map[string][]model.PlannedChange)
	for _, change := range changes {
		if change.IsSkipped() || !change.HasUpdate() {
			continue
		}
		if change.Dependency.SourceKind != sourceKind {
			continue
		}
		changesByFile[change.Dependency.FilePath] = append(changesByFile[change.Dependency.FilePath], change)
	}

	for path, fileChanges := range changesByFile {
		if err := s.applyToFile(path, fileChanges); err != nil {
			return fmt.Errorf("failed to apply changes to %s: %w", path, err)
		}
	}

	return nil
}

func (s *Source) applyToFile(path string, changes []model.PlannedChange) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse aqua yaml: %w", err)
	}

	root := documentRoot(&doc)
	if root == nil {
		return fmt.Errorf("aqua yaml root must be a mapping")
	}

	changesByLocator := make(map[string]model.PlannedChange, len(changes))
	for _, change := range changes {
		if _, exists := changesByLocator[change.Dependency.Locator]; exists {
			return fmt.Errorf("duplicate aqua locator %q in planned changes", change.Dependency.Locator)
		}
		changesByLocator[change.Dependency.Locator] = change
	}

	if err := applyRegistryChanges(aquahelper.MappingValue(root, aquahelper.TargetRegistries), changesByLocator); err != nil {
		return err
	}
	if err := applyPackageChanges(aquahelper.MappingValue(root, aquahelper.TargetPackages), changesByLocator); err != nil {
		return err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("failed to encode aqua yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to finalize aqua yaml encoding: %w", err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write aqua yaml: %w", err)
	}

	return nil
}

func applyRegistryChanges(node *yaml.Node, changes map[string]model.PlannedChange) error {
	if node == nil {
		return nil
	}
	for i, item := range node.Content {
		change, ok := changes[registryLocator(i)]
		if !ok {
			continue
		}
		refNode := aquahelper.MappingValue(item, "ref")
		if refNode == nil {
			return fmt.Errorf("registries[%d].ref not found during apply", i)
		}
		refNode.Value = targetRawVersion(change)
	}
	return nil
}

func applyPackageChanges(node *yaml.Node, changes map[string]model.PlannedChange) error {
	if node == nil {
		return nil
	}
	for i, item := range node.Content {
		change, ok := changes[packageLocator(i)]
		if !ok {
			continue
		}
		rawVersion := targetRawVersion(change)
		switch change.Dependency.Metadata[aquahelper.MetadataVersionField] {
		case aquahelper.VersionFieldNameEmbedded:
			nameNode := aquahelper.MappingValue(item, "name")
			if nameNode == nil {
				return fmt.Errorf("packages[%d].name not found during apply", i)
			}
			nameNode.Value = change.Dependency.Metadata[aquahelper.MetadataPackageName] + "@" + rawVersion
		case aquahelper.VersionFieldVersionField:
			versionNode := aquahelper.MappingValue(item, "version")
			if versionNode == nil {
				return fmt.Errorf("packages[%d].version not found during apply", i)
			}
			versionNode.Value = rawVersion
		default:
			return fmt.Errorf("packages[%d] has unsupported aqua version field %q", i, change.Dependency.Metadata[aquahelper.MetadataVersionField])
		}
	}
	return nil
}

func targetRawVersion(change model.PlannedChange) string {
	if change.SelectedCandidate != nil {
		if raw := change.SelectedCandidate.Metadata[aquahelper.MetadataRawVersion]; raw != "" {
			return raw
		}
	}

	prefix := change.Dependency.Metadata[aquahelper.MetadataVersionPrefix]
	if change.SelectedCandidate != nil {
		if candidatePrefix := change.SelectedCandidate.Metadata[aquahelper.MetadataVersionPrefix]; candidatePrefix != "" || prefix == "" {
			prefix = candidatePrefix
		}
	}
	if change.TargetVersion != nil {
		return aquahelper.ComposeRawVersion(change.TargetVersion.Original(), prefix)
	}
	return change.Dependency.Metadata[aquahelper.MetadataRawVersion]
}

func registryLocator(index int) string {
	return fmt.Sprintf("registry:%d", index)
}

func packageLocator(index int) string {
	return fmt.Sprintf("package:%d", index)
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}
