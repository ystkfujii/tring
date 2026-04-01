package model

import "github.com/Masterminds/semver/v3"

const (
	// MetadataVersionRaw stores the raw version string used for display and write-back.
	MetadataVersionRaw = "raw_version"
	// MetadataRepoURL stores the repository URL used for diff link generation.
	MetadataRepoURL = "repo_url"

	// MetadataResolutionObjectKind distinguishes resolver-affecting object kinds.
	MetadataResolutionObjectKind = "resolution_object_kind"
	// MetadataResolutionRegistryType distinguishes resolver-affecting registry types.
	MetadataResolutionRegistryType = "resolution_registry_type"
	// MetadataResolutionRegistryRef distinguishes resolver-affecting standard registry refs.
	MetadataResolutionRegistryRef = "resolution_registry_ref"
	// MetadataResolutionRegistryPath distinguishes resolver-affecting local registry paths.
	MetadataResolutionRegistryPath = "resolution_registry_path"
)

var resolutionCacheMetadataKeys = []string{
	MetadataResolutionObjectKind,
	MetadataResolutionRegistryType,
	MetadataResolutionRegistryRef,
	MetadataResolutionRegistryPath,
}

// DisplayVersion returns the raw display version when available, falling back to the semver original.
func DisplayVersion(version *semver.Version, metadata map[string]string) string {
	if raw := metadata[MetadataVersionRaw]; raw != "" {
		return raw
	}
	if version == nil {
		return ""
	}
	return version.Original()
}
