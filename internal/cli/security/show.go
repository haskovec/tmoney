package security

import (
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// securityShowOptions are the inputs to `tmoney security show`.
type securityShowOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// newSecurityShowCmd registers `tmoney security show [ticker]`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Identify the security by a positional
// ticker, or by `--isin` / `--name` for securities that have no ticker.
func newSecurityShowCmd() *cobra.Command {
	opts := &securityShowOptions{}
	cmd := &cobra.Command{
		Use:   "show [ticker]",
		Short: "Show details for a specific security",
		Long: "Show full details for a security. Identify it by a positional ticker, " +
			"or by `--isin` / `--name` (exact) for a security that has no ticker.",
		Example: "  tmoney security show AAPL\n" +
			"  tmoney security show --isin US0378331005\n" +
			"  tmoney security show --name \"MFS Mid Cap Value CT\"",
		Args:         cobra.RangeArgs(0, 1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if len(args) > 0 {
				opts.ticker = args[0]
			}
			return runSecurityShow(opts, cmd.OutOrStdout())
		},
	}
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
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

	sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
	if err != nil {
		return err
	}

	printSecurityDetails(w, sec)

	return nil
}
