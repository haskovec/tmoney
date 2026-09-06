package security

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// securityHideOptions are the inputs to `tmoney security hide`.
type securityHideOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// newSecurityHideCmd registers `tmoney security hide [ticker]`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Hiding marks the security as
// hidden so it no longer appears in default listings; data is
// preserved. Identify the security by a positional ticker, or by
// `--isin` / `--name` for securities that have no ticker.
func newSecurityHideCmd() *cobra.Command {
	opts := &securityHideOptions{}
	cmd := &cobra.Command{
		Use:   "hide [ticker]",
		Short: "Hide a security from default listings",
		Long: "Mark a security as hidden so it no longer appears in default listings. Data is preserved; use `security unhide` to restore visibility. " +
			"Identify the security by a positional ticker, or by `--isin` / `--name` (exact) for a security that has no ticker.",
		Example:      "  tmoney security hide AAPL",
		Args:         cobra.RangeArgs(0, 1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if len(args) > 0 {
				opts.ticker = args[0]
			}
			return runSecurityHide(opts, cmd.OutOrStdout())
		},
	}
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runSecurityHide hides a security.
func runSecurityHide(opts *securityHideOptions, w io.Writer) error {
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

	if err := svc.Security.Hide(sec.ID); err != nil {
		return fmt.Errorf("failed to hide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s hidden successfully.\n", securityLabel(sec))

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
