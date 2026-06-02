package investment

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentEnableLotsOptions are the inputs to `tmoney investment enable-lots`.
type investmentEnableLotsOptions struct {
	file    string
	account string
	all     bool
	method  string
	confirm bool
}

// newInvestmentEnableLotsCmd registers `tmoney investment enable-lots`.
func newInvestmentEnableLotsCmd() *cobra.Command {
	opts := &investmentEnableLotsOptions{}
	cmd := &cobra.Command{
		Use:   "enable-lots",
		Short: "Enable lot tracking on an existing account and backfill lots from history",
		Long: "Enable lot tracking on an existing investment or HSA account and " +
			"backfill its lots by replaying the full transaction ledger. Buys, " +
			"reinvested dividends, and inbound share transfers open lots; sells, " +
			"fee liquidations, and outbound transfers consume open lots by the " +
			"--method (fifo, lifo, or hifo; default fifo). By default the command " +
			"prints the plan and makes no changes; pass --confirm to execute. " +
			"Run `db backup` first. Refuses when the account already has lots or " +
			"holds a security with a recorded corporate action.",
		Example: "  tmoney -f personal.tdb investment enable-lots --account \"Wealthfront IRA\"\n" +
			"  tmoney -f personal.tdb investment enable-lots --account \"Wealthfront IRA\" --confirm\n" +
			"  tmoney -f personal.tdb investment enable-lots --all --confirm",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentEnableLots(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment/HSA account to enable (required unless --all)")
	cmd.Flags().BoolVar(&opts.all, "all", false, "Enable lots on every investment/HSA account not already lot-tracked")
	cmd.Flags().StringVar(&opts.method, "method", "fifo", "Lot-selection method for historical sells: fifo, lifo, or hifo")
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Execute the backfill (default prints the plan only)")
	return cmd
}

// runInvestmentEnableLots executes `tmoney investment enable-lots`.
func runInvestmentEnableLots(opts *investmentEnableLotsOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	method, err := investmentdom.ParseLotMethod(opts.method)
	if err != nil {
		return fmt.Errorf("invalid --method: %w", err)
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
			if a.Type.IsInvestmentType() && !a.TrackLots {
				targets = append(targets, a)
			}
		}
		if len(targets) == 0 {
			fmt.Fprintln(w, "No investment/HSA accounts to enable (all are already lot-tracked).")
			return nil
		}
	}

	anyApplied := false
	failed := false
	for _, acct := range targets {
		res, err := svc.Investment.EnableLots(acct.ID, method, opts.confirm)
		if err != nil {
			var blocked *investmentdom.BackfillBlockedError
			if errors.As(err, &blocked) {
				printEnableLotsBlocked(w, svc, blocked)
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
		printEnableLotsResult(w, svc, res)
		if res.Applied {
			anyApplied = true
		}
	}

	if anyApplied {
		cmdutil.AutoBackupAfterModification(opts.file)
	}
	if failed {
		return fmt.Errorf("one or more accounts could not be enabled (see above)")
	}
	return nil
}

// tickerFor resolves a security id to its ticker, falling back to the id.
func tickerFor(svc *app.Services, secID types.ID) string {
	sec, err := svc.Security.GetByID(secID)
	if err != nil || sec == nil {
		return secID.String()
	}
	return sec.Ticker
}

// printEnableLotsResult writes the plan/applied summary for one account.
func printEnableLotsResult(w io.Writer, svc *app.Services, res *investmentdom.EnableLotsResult) {
	plan := res.Plan
	perSec := plan.LotsPerSecurity()

	type row struct {
		ticker string
		count  int
	}
	rows := make([]row, 0, len(perSec))
	for secID, n := range perSec {
		rows = append(rows, row{ticker: tickerFor(svc, secID), count: n})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ticker < rows[j].ticker })

	if res.Applied {
		fmt.Fprintf(w, "%s: lot tracking enabled — %d lot(s) across %d security(ies), %d junction(s) (method=%s)\n",
			res.AccountName, len(plan.Lots), len(perSec), len(plan.Junctions), res.Method)
	} else {
		fmt.Fprintf(w, "%s: preview (method=%s) — would create %d lot(s) across %d security(ies), %d junction(s):\n",
			res.AccountName, res.Method, len(plan.Lots), len(perSec), len(plan.Junctions))
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-8s %d lot(s)\n", r.ticker, r.count)
	}
	for _, sf := range plan.Shortfalls {
		fmt.Fprintf(w, "  ! shortfall: %s on %s — requested %s, covered %s (uncovered sale)\n",
			tickerFor(svc, sf.SecurityID), sf.Date.String(), sf.Requested.String(), sf.Covered.String())
	}
	if !res.Applied {
		fmt.Fprintln(w, "  Re-run with --confirm to apply (run `db backup` first).")
	}
}

// printEnableLotsBlocked reports the corporate-action holdings blocking an account.
func printEnableLotsBlocked(w io.Writer, svc *app.Services, blocked *investmentdom.BackfillBlockedError) {
	fmt.Fprintf(w, "%s: cannot enable lots — these holdings have corporate actions:\n", blocked.AccountName)
	for _, b := range blocked.Blockers {
		fmt.Fprintf(w, "  %-8s %d action(s)\n", tickerFor(svc, b.SecurityID), b.Actions)
	}
	fmt.Fprintln(w, "  Lots cannot be reconstructed across a split/merger/spin-off; this account stays on average cost.")
}
