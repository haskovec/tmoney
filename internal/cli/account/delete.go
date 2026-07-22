package account

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/spf13/cobra"
)

// accountDeleteOptions are the inputs to `tmoney account delete <name>`.
type accountDeleteOptions struct {
	file    string
	name    string
	confirm bool
}

// newAccountDeleteCmd registers `tmoney account delete <name>`. The database
// file is taken from the persistent `--file` / `-f` flag; the single positional
// argument is the account name. Without `--confirm` it prints a dry-run preview.
func newAccountDeleteCmd() *cobra.Command {
	opts := &accountDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an account",
		Long: "Permanently delete an account. Delete only works on an account with no " +
			"transactions and no scheduled transactions referencing it; for an account " +
			"with history, `tmoney account close` is usually the better option (it freezes " +
			"the account while preserving its transactions). Prints a dry-run preview by " +
			"default; pass --confirm to delete.",
		Example: "  tmoney account delete \"Old Savings\"\n" +
			"  tmoney account delete \"Old Savings\" --confirm",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.name = args[0]
			return runAccountDelete(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Actually delete the account (default: dry-run preview only)")
	return cmd
}

// runAccountDelete deletes an account after guarding against transactions
// (enforced by the repo) and scheduled references (a CLI-layer guard).
func runAccountDelete(opts *accountDeleteOptions, w io.Writer) error {
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

	// Scheduled templates that reference this account would be orphaned by a
	// delete (the repo does not check this), so it is refused at both stages.
	refs, err := svc.Scheduled.ListReferencing(acct.ID)
	if err != nil {
		return fmt.Errorf("failed to check scheduled transactions: %w", err)
	}

	if !opts.confirm {
		fmt.Fprintln(w, "Would delete account:")
		fmt.Fprintf(w, "  Name: %s\n", acct.Name)
		fmt.Fprintf(w, "  Type: %s\n", acct.Type.DisplayName())
		if bal, berr := svc.Account.GetBalance(acct.ID); berr == nil {
			fmt.Fprintf(w, "  Balance: %s\n", cmdutil.FormatMoney(bal.CurrentBalance, acct.Currency))
		}
		if len(refs) > 0 {
			fmt.Fprintf(w, "\nWarning: %d scheduled transaction(s) reference this account; "+
				"delete is blocked until they are redirected (tmoney scheduled edit --account) "+
				"or removed (tmoney scheduled delete).\n", len(refs))
		}
		fmt.Fprintln(w, "\nRe-run with --confirm to delete.")
		return nil
	}

	if len(refs) > 0 {
		return fmt.Errorf("cannot delete account %q: %d scheduled transaction(s) reference it; "+
			"redirect them (tmoney scheduled edit --account) or remove them (tmoney scheduled delete) first",
			acct.Name, len(refs))
	}

	if err := svc.Account.Delete(acct.ID); err != nil {
		var depErr *dberrors.HasDependentsError
		if errors.As(err, &depErr) {
			if depErr.Dependents == "transactions" {
				return fmt.Errorf("cannot delete account %q: it has %d %s — close it instead (tmoney account close)",
					acct.Name, depErr.Count, depErr.Dependents)
			}
			return fmt.Errorf("cannot delete account %q: it has %d %s",
				acct.Name, depErr.Count, depErr.Dependents)
		}
		return fmt.Errorf("failed to delete account: %w", err)
	}

	fmt.Fprintf(w, "Deleted account %q.\n", acct.Name)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
