package account

import (
	"errors"
	"fmt"
	"io"

	accountdom "github.com/haskovec/tmoney/internal/account"
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
			"transactions or investment history, no transfer rows in other accounts that " +
			"name it, no scheduled transactions referencing it, and no reconciliation in " +
			"progress; its completed reconciliations are deleted with it. For an account " +
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
		// The preview reports what the account holds; a valuation error is
		// shown, not fatal, so the blocking warning below still prints.
		if figs, ferr := svc.Report.AccountFigures([]*accountdom.Account{acct}); ferr == nil {
			label, line := "Balance", formatFigure(figs[0])
			if acct.Type.IsInvestmentType() {
				label = "Total Value"
			}
			if figs[0].Err != nil {
				line += " (" + figs[0].Err.Error() + ")"
			}
			fmt.Fprintf(w, "  %s: %s\n", label, line)
		}
		if len(refs) > 0 {
			fmt.Fprintf(w, "\nWarning: %d scheduled transaction(s) reference this account; "+
				"delete is blocked until they are redirected (tmoney scheduled edit --account) "+
				"or removed (tmoney scheduled delete).\n", len(refs))
		} else if berr := svc.Account.DeleteBlocker(acct.ID); berr != nil {
			fmt.Fprintf(w, "\nWarning: %v\n", deleteRefusal(acct.Name, berr))
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
		return deleteRefusal(acct.Name, err)
	}

	fmt.Fprintf(w, "Deleted account %q.\n", acct.Name)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}

// deleteRefusal turns a delete refusal into a sentence that names the account
// and says what to do. Other errors pass through wrapped.
func deleteRefusal(name string, err error) error {
	var depErr *dberrors.HasDependentsError
	if !errors.As(err, &depErr) {
		return fmt.Errorf("failed to delete account: %w", err)
	}
	switch depErr.Dependents {
	case "transactions", "investment transactions":
		return fmt.Errorf("cannot delete account %q: it has %d %s — close it instead (tmoney account close)",
			name, depErr.Count, depErr.Dependents)
	case "transfer references":
		return fmt.Errorf("cannot delete account %q: %d transfer row(s) in other accounts name it as their other side; "+
			"delete those transfers first", name, depErr.Count)
	case "active reconciliation":
		return fmt.Errorf("cannot delete account %q: it has a reconciliation in progress; "+
			"finish it (tmoney reconcile finish) or cancel it in the TUI first", name)
	default:
		return fmt.Errorf("cannot delete account %q: it has %d %s", name, depErr.Count, depErr.Dependents)
	}
}
