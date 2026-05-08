package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// securityUnhideOptions are the inputs to `tmoney security unhide`.
type securityUnhideOptions struct {
	file   string
	ticker string
}

// newSecurityUnhideCmd registers `tmoney security unhide <ticker>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Unhiding restores a previously
// hidden security to the default listings.
func newSecurityUnhideCmd() *cobra.Command {
	opts := &securityUnhideOptions{}
	cmd := &cobra.Command{
		Use:          "unhide <ticker>",
		Short:        "Restore a hidden security to default listings",
		Long:         "Unhide a previously hidden security so it appears in default listings again.",
		Example:      "  tmoney security unhide AAPL",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runSecurityUnhide(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runSecurityUnhide unhides a security.
func runSecurityUnhide(opts *securityUnhideOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}

	if err := svc.Security.Unhide(sec.ID); err != nil {
		return fmt.Errorf("failed to unhide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) unhidden successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}
