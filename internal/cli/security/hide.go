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
}

// newSecurityHideCmd registers `tmoney security hide <ticker>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Hiding marks the security as
// hidden so it no longer appears in default listings; data is
// preserved.
func newSecurityHideCmd() *cobra.Command {
	opts := &securityHideOptions{}
	cmd := &cobra.Command{
		Use:          "hide <ticker>",
		Short:        "Hide a security from default listings",
		Long:         "Mark a security as hidden so it no longer appears in default listings. Data is preserved; use `security unhide <ticker>` to restore visibility.",
		Example:      "  tmoney security hide AAPL",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runSecurityHide(opts, cmd.OutOrStdout())
		},
	}
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

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}

	if err := svc.Security.Hide(sec.ID); err != nil {
		return fmt.Errorf("failed to hide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) hidden successfully.\n", sec.Ticker, sec.Name)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
