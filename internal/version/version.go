package version

var (
	// Version is the semantic version, set via -ldflags at build time.
	Version = "dev"
	// Commit is the git commit hash, set via -ldflags at build time.
	Commit = "unknown"
	// Date is the build date, set via -ldflags at build time.
	Date = "unknown"
)
