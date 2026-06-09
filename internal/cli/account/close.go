package account

import (
	"errors"
	"fmt"
	"io"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// accountCloseOptions are the inputs to `tmoney account close <name>`.
type accountCloseOptions struct {
	file string
	name string
	date string
}

// newAccountCloseCmd registers `tmoney account close <name>`. The database
// file is taken from the persistent `--file` / `-f` flag; the single
// positional argument is the account name. `--date` sets the close date
// (default today).
func newAccountCloseCmd() *cobra.Command {
	opts := &accountCloseOptions{}
	cmd := &cobra.Command{
		Use:   "close <name>",
		Short: "Close an account",
		Long: "Close an account as of a date (default today). The account must have a " +
			"zero balance, and the close date must fall on or after the account's " +
			"opening date and its latest transaction, and not in the future. A closed " +
			"account is frozen — no new transactions, edits, or transfers — and it no " +
			"longer appears in account pickers. Reopen it with `account reopen`.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.name = args[0]
			return runAccountClose(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.date, "date", "", "Close date YYYY-MM-DD (default today)")
	return cmd
}

// runAccountClose closes an account, validating zero balance and the close date.
func runAccountClose(opts *accountCloseOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	closeDate := types.Today()
	if opts.date != "" {
		var derr error
		closeDate, derr = types.ParseDate(opts.date)
		if derr != nil {
			return fmt.Errorf("invalid --date: %w", derr)
		}
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

	if err := svc.Account.Close(acct.ID, closeDate); err != nil {
		// Surface the account name (the service errors carry the UUID).
		var balErr *accountdom.HasBalanceError
		var dateErr *accountdom.InvalidCloseDateError
		var alreadyErr *accountdom.AlreadyClosedError
		switch {
		case errors.As(err, &balErr):
			return fmt.Errorf("cannot close %q: balance is %s (must be zero to close)",
				acct.Name, cmdutil.FormatMoney(balErr.Balance, acct.Currency))
		case errors.As(err, &dateErr):
			return fmt.Errorf("cannot close %q: close date %s must be between %s and %s",
				acct.Name, dateErr.Date, dateErr.Earliest, dateErr.Today)
		case errors.As(err, &alreadyErr):
			return fmt.Errorf("account %q is already closed", acct.Name)
		default:
			return err
		}
	}

	fmt.Fprintln(w, "Account closed.")
	fmt.Fprintf(w, "  Name:   %s\n", acct.Name)
	fmt.Fprintf(w, "  Closed: %s\n", closeDate.String())

	// Soft warning (print and proceed): schedules still pointed at this account
	// will be skipped on auto-post and refused on manual post.
	if refs, rerr := svc.Scheduled.ListReferencing(acct.ID); rerr == nil && len(refs) > 0 {
		fmt.Fprintf(w, "\nWarning: %d scheduled transaction(s) reference this account; "+
			"they will be skipped on auto-post and refused on manual post until "+
			"redirected or deleted.\n", len(refs))
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
