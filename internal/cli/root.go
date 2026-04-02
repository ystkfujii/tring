package cli

import (
	"context"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "tring",
	Short: "A CLI tool to track and update local dependencies",
	Long: `tring is a CLI tool that helps you manage and update local dependencies.

It extracts dependencies from various sources (go.mod, envfile),
resolves new versions using configurable resolvers, and applies updates based
on your policy constraints.`,
}

// Execute runs the root command with background context.
func Execute() error {
	return ExecuteContext(context.Background())
}

// ExecuteContext runs the root command with the given context.
func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(versionCmd)
}
