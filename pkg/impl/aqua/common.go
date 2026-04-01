package aqua

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/ystkfujii/tring/internal/domain/model"
)

const (
	MetadataObjectKind        = model.MetadataResolutionObjectKind
	MetadataRegistryType      = model.MetadataResolutionRegistryType
	MetadataRegistryName      = "aqua_registry_name"
	MetadataVersionField      = "aqua_version_field"
	MetadataRawVersion        = model.MetadataVersionRaw
	MetadataVersionPrefix     = "aqua_version_prefix"
	MetadataPackageName       = "aqua_package_name"
	MetadataItemIndex         = "aqua_item_index"
	MetadataLocalRegistryPath = model.MetadataResolutionRegistryPath
	MetadataStandardRef       = model.MetadataResolutionRegistryRef
)

const (
	ObjectKindPackage  = "package"
	ObjectKindRegistry = "registry"
)

const (
	RegistryTypeStandard = "standard"
	RegistryTypeLocal    = "local"
)

const (
	VersionFieldNameEmbedded = "name_embedded"
	VersionFieldVersionField = "version_field"
	VersionFieldRef          = "ref"
)

const (
	TargetPackages   = "packages"
	TargetRegistries = "registries"
)

const (
	UnsupportedVersionSkip  = "skip"
	UnsupportedVersionError = "error"
)

var semverSuffixPattern = regexp.MustCompile(`^(.*?)([vV]?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$`)

// NormalizeVersion parses a raw aqua version string into a semver version and prefix.
func NormalizeVersion(raw string) (*semver.Version, string, error) {
	return NormalizeVersionWithPrefix(raw, "")
}

// NormalizeVersionWithPrefix parses a raw aqua version string into a semver version and prefix.
// The configured prefix is tried first so package definitions such as "V1.2.3" work reliably.
func NormalizeVersionWithPrefix(raw, configuredPrefix string) (*semver.Version, string, error) {
	if raw == "" {
		return nil, "", fmt.Errorf("version is empty")
	}

	if configuredPrefix != "" && strings.HasPrefix(raw, configuredPrefix) {
		trimmed := raw[len(configuredPrefix):]
		if v, err := semver.NewVersion(normalizeSemverToken(trimmed)); err == nil {
			return v, configuredPrefix, nil
		}
	}

	if v, err := semver.NewVersion(normalizeSemverToken(raw)); err == nil {
		return v, "", nil
	}

	matches := semverSuffixPattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return nil, "", fmt.Errorf("version %q is not semver-compatible", raw)
	}

	v, err := semver.NewVersion(normalizeSemverToken(matches[2]))
	if err != nil {
		return nil, "", fmt.Errorf("version %q is not semver-compatible: %w", raw, err)
	}

	return v, matches[1], nil
}

func normalizeSemverToken(version string) string {
	if strings.HasPrefix(version, "V") {
		return "v" + strings.TrimPrefix(version, "V")
	}
	return version
}

// ComposeRawVersion reconstructs a raw aqua version string from a semver string and prefix.
func ComposeRawVersion(version, prefix string) string {
	return prefix + version
}

// MatchesPackageName reports whether a registry package definition matches the requested aqua package name.
func MatchesPackageName(pkg RegistryPackage, packageName string) bool {
	for _, candidate := range pkg.Names() {
		if candidate == packageName {
			return true
		}
	}
	return false
}

// RegistryFile is the minimal registry shape required by tring.
type RegistryFile struct {
	Packages []RegistryPackage `yaml:"packages"`
}

// RegistryPackage is the minimal package definition shape required by tring.
type RegistryPackage struct {
	Name          string          `yaml:"name"`
	Type          string          `yaml:"type"`
	RepoOwner     string          `yaml:"repo_owner"`
	RepoName      string          `yaml:"repo_name"`
	VersionSource string          `yaml:"version_source"`
	VersionPrefix string          `yaml:"version_prefix"`
	VersionFilter string          `yaml:"version_filter"`
	Aliases       []RegistryAlias `yaml:"aliases"`
}

// Names returns all package names that can match an aqua package reference.
func (p RegistryPackage) Names() []string {
	var names []string
	if p.Name != "" {
		names = append(names, p.Name)
	}
	if p.RepoOwner != "" && p.RepoName != "" {
		defaultName := p.RepoOwner + "/" + p.RepoName
		if defaultName != p.Name {
			names = append(names, defaultName)
		}
	}
	for _, alias := range p.Aliases {
		if alias.Name != "" {
			names = append(names, alias.Name)
		}
	}
	return names
}

// RegistryAlias is the minimal alias definition shape required by tring.
type RegistryAlias struct {
	Name string `yaml:"name"`
}
