package account

import (
	"errors"
	"fmt"
	"io"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// accountReopenOptions are the inputs to `tmoney account reopen <name>`.
type accountReopenOptions struct {
	file string
	name string
}

// newAccountReopenCmd registers `tmoney account reopen <name>`. Reopening
// clears the account's close date and allows transactions again.
func newAccountReopenCmd() *cobra.Command {
	opts := &accountReopenOptions{}
	cmd := &cobra.Command{
		Use:          "reopen <name>",
		Short:        "Reopen a closed account",
		Long:         "Reopen a closed account, clearing its close date and allowing transactions again.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.name = args[0]
			return runAccountReopen(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runAccountReopen reopens a closed account.
func runAccountReopen(opts *accountReopenOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.name)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.name)
	}

	if err := svc.Account.Reopen(acct.ID); err != nil {
		var notClosed *accountdom.NotClosedError
		if errors.As(err, &notClosed) {
			return fmt.Errorf("account %q is not closed", acct.Name)
		}
		return err
	}

	fmt.Fprintln(w, "Account reopened.")
	fmt.Fprintf(w, "  Name: %s\n", acct.Name)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
