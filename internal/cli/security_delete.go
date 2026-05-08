package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// securityDeleteOptions are the inputs to `tmoney security delete`.
type securityDeleteOptions struct {
	file   string
	ticker string
}

// newSecurityDeleteCmd registers `tmoney security delete <ticker>`.
// Refuses to delete a security that has dependent prices or
// transactions and suggests `security hide` instead.
func newSecurityDeleteCmd() *cobra.Command {
	opts := &securityDeleteOptions{}
	cmd := &cobra.Command{
		Use:          "delete <ticker>",
		Short:        "Delete a security",
		Long:         "Delete a security from the database. Refuses to delete securities with dependent prices or transactions; use `security hide <ticker>` instead in that case.",
		Example:      "  tmoney security delete AAPL",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runSecurityDelete(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runSecurityDelete deletes a security.
func runSecurityDelete(opts *securityDeleteOptions, w io.Writer) error {
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

	if err := svc.Security.Delete(sec.ID); err != nil {
		var depErr *security.HasDependentsError
		if errors.As(err, &depErr) {
			return fmt.Errorf("%s\nUse tmoney security hide %s instead", depErr.Error(), sec.Ticker)
		}
		return fmt.Errorf("failed to delete security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) deleted successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}
