package theme

import (
	"github.com/spf13/cobra"
)

// NewCmd returns the `theme` parent command. Child subcommands
// (`list`, `generate-from-wal`) are filled in by their own files; this
// step (TH-035) only registers the parent and stub children so the
// command tree is discoverable.
func NewCmd() *cobra.Command {
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
