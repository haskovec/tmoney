package security

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// securityShowOptions are the inputs to `tmoney security show`.
type securityShowOptions struct {
	file   string
	ticker string
}

// newSecurityShowCmd registers `tmoney security show <ticker>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newSecurityShowCmd() *cobra.Command {
	opts := &securityShowOptions{}
	cmd := &cobra.Command{
		Use:          "show <ticker>",
		Short:        "Show details for a specific security",
		Long:         "Show full details for a security identified by ticker.",
		Example:      "  tmoney security show AAPL",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runSecurityShow(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runSecurityShow shows detailed information for a specific security.
func runSecurityShow(opts *securityShowOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}

	printSecurityDetails(w, sec)

	return nil
}
