package cli

import (
	"github.com/spf13/cobra"
)

// newThemeCmd returns the `theme` parent command. Child subcommands
// (`list`, `generate-from-wal`) are filled in by their own files; this
// step (TH-035) only registers the parent and stub children so the
// command tree is discoverable.
func newThemeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage TMoney color themes",
		Long: "Subcommands for listing built-in and user themes and " +
			"generating themes from external sources (e.g. pywal).",
		Example: "  tmoney theme list                  List built-in and user themes\n" +
			"  tmoney theme generate-from-wal     Generate a wal.toml theme from pywal",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		SilenceUsage: true,
	}
	cmd.AddCommand(newThemeListCmd())
	cmd.AddCommand(newThemeGenerateFromWalCmd())
	return cmd
}

// newThemeListCmd is a stub; TH-037 replaces this with the real
// implementation in internal/cli/theme_list.go.
func newThemeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available themes (built-in and user)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.PrintErrln("theme list: not yet implemented")
			return nil
		},
	}
}

// newThemeGenerateFromWalCmd is a stub; TH-036 replaces this with the
// real implementation in internal/cli/theme_wal.go.
func newThemeGenerateFromWalCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "generate-from-wal",
		Short: "Generate a theme TOML file from the pywal cache",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.PrintErrln("theme generate-from-wal: not yet implemented")
			return nil
		},
	}
}
