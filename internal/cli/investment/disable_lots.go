package investment

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/spf13/cobra"
)

// investmentDisableLotsOptions are the inputs to `tmoney investment disable-lots`.
type investmentDisableLotsOptions struct {
	file    string
	account string
	all     bool
	confirm bool
}

// newInvestmentDisableLotsCmd registers `tmoney investment disable-lots`.
func newInvestmentDisableLotsCmd() *cobra.Command {
	opts := &investmentDisableLotsOptions{}
	cmd := &cobra.Command{
		Use:   "disable-lots",
		Short: "Disable lot tracking on an account and revert it to average cost",
		Long: "Disable lot tracking on an existing lot-tracked investment or HSA " +
			"account: delete its lots and lot-junction rows and recompute its " +
			"positions to average cost. By default the command prints the plan and " +
			"makes no changes; pass --confirm to execute. Run `db backup` first. " +
			"Refuses when the account is not lot-tracked. A held security with a " +
			"stock split is fine (the split replays into average cost), but a held " +
			"merger or spin-off is refused — those holdings live only in lots and " +
			"their average cost cannot be rebuilt from the ledger.",
		Example: "  tmoney -f personal.tdb investment disable-lots --account \"Fidelity 401k\"\n" +
			"  tmoney -f personal.tdb investment disable-lots --account \"Fidelity 401k\" --confirm\n" +
			"  tmoney -f personal.tdb investment disable-lots --all --confirm",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentDisableLots(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Lot-tracked investment/HSA account to disable (required unless --all)")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Disable lots on every lot-tracked investment/HSA account")
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Execute the change (default prints the plan only)")
	return cmd
}

// runInvestmentDisableLots executes `tmoney investment disable-lots`.
func runInvestmentDisableLots(opts *investmentDisableLotsOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	if opts.account == "" && !opts.all {
		return fmt.Errorf("provide --account NAME or --all")
	}
	if opts.account != "" && opts.all {
		return fmt.Errorf("--account and --all are mutually exclusive")
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var targets []*account.Account
	if opts.account != "" {
		acct, err := svc.Account.GetByName(opts.account)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.account)
		}
		targets = []*account.Account{acct}
	} else {
		accts, err := svc.Account.List(false)
		if err != nil {
			return fmt.Errorf("failed to list accounts: %w", err)
		}
		for _, a := range accts {
			if a.Type.IsInvestmentType() && a.TrackLots {
				targets = append(targets, a)
			}
		}
		if len(targets) == 0 {
			fmt.Fprintln(w, "No lot-tracked investment/HSA accounts to disable.")
			return nil
		}
	}

	anyApplied := false
	failed := false
	for _, acct := range targets {
		res, err := svc.Investment.DisableLots(acct.ID, opts.confirm)
		if err != nil {
			var blocked *investmentdom.DisableLotsBlockedError
			if errors.As(err, &blocked) {
				printDisableLotsBlocked(w, svc, blocked)
				if opts.account != "" {
					return err
				}
				failed = true
				continue
			}
			if opts.account != "" {
				return err
			}
			fmt.Fprintf(w, "%s: error: %v\n", acct.Name, err)
			failed = true
			continue
		}
		printDisableLotsResult(w, svc, res)
		if res.Applied {
			anyApplied = true
		}
	}

	if anyApplied {
		cmdutil.AutoBackupAfterModification(opts.file)
	}
	if failed {
		return fmt.Errorf("one or more accounts could not be disabled (see above)")
	}
	return nil
}

// printDisableLotsResult writes the plan/applied summary for one account, plus a
// warn-and-proceed note for any corporate-action holdings.
func printDisableLotsResult(w io.Writer, svc *app.Services, res *investmentdom.DisableLotsResult) {
	if res.Applied {
		fmt.Fprintf(w, "%s: lot tracking disabled — removed %d lot(s) across %d security(ies), %d junction(s); %d position(s) recomputed to average cost\n",
			res.AccountName, res.LotsDeleted, res.Securities, res.JunctionsDeleted, res.PositionsRecomputed)
	} else {
		fmt.Fprintf(w, "%s: preview — would remove %d lot(s) across %d security(ies) and %d junction(s), then revert to average cost\n",
			res.AccountName, res.LotsDeleted, res.Securities, res.JunctionsDeleted)
	}
	if res.HasCorporateActions() {
		fmt.Fprintf(w, "  note: %d held security(ies) have stock splits; these replay cleanly into average cost:\n", len(res.Blockers))
		for _, b := range res.Blockers {
			fmt.Fprintf(w, "    %-8s %d action(s)\n", tickerFor(svc, b.SecurityID), b.Actions)
		}
	}
	if !res.Applied {
		fmt.Fprintln(w, "  Re-run with --confirm to apply (run `db backup` first).")
	}
}

// printDisableLotsBlocked reports the merger/spin-off holdings that block a
// disable-lots (their average cost can't be rebuilt from the ledger).
func printDisableLotsBlocked(w io.Writer, svc *app.Services, blocked *investmentdom.DisableLotsBlockedError) {
	fmt.Fprintf(w, "%s: cannot disable lots — these holdings have a merger or spin-off:\n", blocked.AccountName)
	for _, b := range blocked.Blockers {
		fmt.Fprintf(w, "  %-8s %d action(s)\n", tickerFor(svc, b.SecurityID), b.Actions)
	}
	fmt.Fprintln(w, "  Their average cost can't be rebuilt from the ledger, so the account stays lot-tracked.")
}
