package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/ystkfujii/tring/internal/app/apply"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply dependency updates",
	Long: `Apply dependency updates based on the configuration.

This command reads the configuration file, extracts dependencies from the
specified group's sources, resolves new versions, and applies updates according
to the policy.`,
	RunE: runApply,
}

var (
	configPath   string
	groupName    string
	dryRun       bool
	showDiffLink bool
)

func init() {
	applyCmd.Flags().StringVarP(&configPath, "config", "c", "tring.yaml", "path to configuration file")
	applyCmd.Flags().StringVarP(&groupName, "group", "g", "", "group name to apply (required)")
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show planned changes without applying")
	applyCmd.Flags().BoolVar(&showDiffLink, "diff-link", false, "show diff links where available")

	_ = applyCmd.MarkFlagRequired("group")
}

func runApply(cmd *cobra.Command, args []string) error {
	opts := apply.Options{
		ConfigPath:   configPath,
		GroupName:    groupName,
		DryRun:       dryRun,
		ShowDiffLink: showDiffLink,
		Output:       os.Stdout,
	}

	return apply.Run(cmd.Context(), opts)
}
