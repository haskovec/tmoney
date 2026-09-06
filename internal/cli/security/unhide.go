package security

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// securityUnhideOptions are the inputs to `tmoney security unhide`.
type securityUnhideOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// newSecurityUnhideCmd registers `tmoney security unhide [ticker]`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Unhiding restores a previously
// hidden security to the default listings. Identify the security by a
// positional ticker, or by `--isin` / `--name` for securities that have
// no ticker.
func newSecurityUnhideCmd() *cobra.Command {
	opts := &securityUnhideOptions{}
	cmd := &cobra.Command{
		Use:   "unhide [ticker]",
		Short: "Restore a hidden security to default listings",
		Long: "Unhide a previously hidden security so it appears in default listings again. " +
			"Identify the security by a positional ticker, or by `--isin` / `--name` (exact) for a security that has no ticker.",
		Example:      "  tmoney security unhide AAPL",
		Args:         cobra.RangeArgs(0, 1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if len(args) > 0 {
				opts.ticker = args[0]
			}
			return runSecurityUnhide(opts, cmd.OutOrStdout())
		},
	}
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runSecurityUnhide unhides a security.
func runSecurityUnhide(opts *securityUnhideOptions, w io.Writer) error {
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

	if err := svc.Security.Unhide(sec.ID); err != nil {
		return fmt.Errorf("failed to unhide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s unhidden successfully.\n", securityLabel(sec))

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
