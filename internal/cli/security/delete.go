package security

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	securitydom "github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityDeleteOptions are the inputs to `tmoney security delete`.
type securityDeleteOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// newSecurityDeleteCmd registers `tmoney security delete [ticker]`.
// Refuses to delete a security that has dependent prices or
// transactions and suggests `security hide` instead. Identify the
// security by a positional ticker, or by `--isin` / `--name` for
// securities that have no ticker.
func newSecurityDeleteCmd() *cobra.Command {
	opts := &securityDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete [ticker]",
		Short: "Delete a security",
		Long: "Delete a security from the database. Refuses to delete securities with dependent prices or transactions; use `security hide` instead in that case. " +
			"Identify the security by a positional ticker, or by `--isin` / `--name` (exact) for a security that has no ticker.",
		Example:      "  tmoney security delete AAPL",
		Args:         cobra.RangeArgs(0, 1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if len(args) > 0 {
				opts.ticker = args[0]
			}
			return runSecurityDelete(opts, cmd.OutOrStdout())
		},
	}
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runSecurityDelete deletes a security.
func runSecurityDelete(opts *securityDeleteOptions, w io.Writer) error {
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

	if err := svc.Security.Delete(sec.ID); err != nil {
		var depErr *securitydom.HasDependentsError
		if errors.As(err, &depErr) {
			hint := sec.Ticker
			if hint == "" {
				hint = "--isin " + sec.ISIN
			}
			return fmt.Errorf("%s\nUse tmoney security hide %s instead", depErr.Error(), hint)
		}
		return fmt.Errorf("failed to delete security: %w", err)
	}

	fmt.Fprintf(w, "Security %s deleted successfully.\n", securityLabel(sec))

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
