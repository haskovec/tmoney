package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version information - will be set via build flags in production
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, build time, and git commit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "tmoney %s\n", Version)
			fmt.Fprintf(out, "  build time: %s\n", BuildTime)
			fmt.Fprintf(out, "  git commit: %s\n", GitCommit)
			return nil
		},
	}
}
