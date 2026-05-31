package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/spf13/cobra"
)

// investmentRebuildPositionsOptions are the inputs to `tmoney investment rebuild-positions`.
type investmentRebuildPositionsOptions struct {
	file    string
	account string // optional: restrict to one account
}

// newInvestmentRebuildPositionsCmd registers `tmoney investment rebuild-positions`.
// Recomputes investment_positions (and lot shares/closed for lot-tracking
// accounts) from the transaction ledger + junction records. Use this to
// recover from desynced state caused by aborted edits or older code that
// failed to reverse position updates correctly.
func newInvestmentRebuildPositionsCmd() *cobra.Command {
	opts := &investmentRebuildPositionsOptions{}
	cmd := &cobra.Command{
		Use:   "rebuild-positions",
		Short: "Recompute positions and lot shares from the transaction ledger",
		Long: "Recompute investment_positions (and lot shares/closed for " +
			"lot-tracking accounts) from the transaction ledger + lot " +
			"junction records. Use this to recover from desynced state. " +
			"If --account is omitted, every investment account is rebuilt. " +
			"Refuses to run on databases with corporate-action history.",
		Example: "  tmoney -f personal.tdb investment rebuild-positions\n" +
			"  tmoney -f personal.tdb investment rebuild-positions --account Brokerage",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentRebuildPositions(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Limit rebuild to a single investment account (default: all)")
	return cmd
}

// runInvestmentRebuildPositions runs the rebuild against one or every investment account.
func runInvestmentRebuildPositions(opts *investmentRebuildPositionsOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	if opts.account != "" {
		acct, err := svc.Account.GetByName(opts.account)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.account)
		}
		res, err := svc.Investment.RebuildPositions(acct.ID)
		if err != nil {
			return fmt.Errorf("rebuild failed: %w", err)
		}
		printRebuildResult(w, res)
		cmdutil.AutoBackupAfterModification(opts.file)
		return nil
	}

	accounts, err := svc.Account.List(false)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}
	processed := 0
	for _, acct := range accounts {
		if !acct.Type.IsInvestmentType() {
			continue
		}
		res, err := svc.Investment.RebuildPositions(acct.ID)
		if err != nil {
			return fmt.Errorf("rebuild for %q failed: %w", acct.Name, err)
		}
		printRebuildResult(w, res)
		processed++
	}
	if processed == 0 {
		fmt.Fprintln(w, "No investment accounts found.")
		return nil
	}
	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// printRebuildResult writes a human-readable summary for one account's rebuild.
func printRebuildResult(w io.Writer, res *investmentdom.RebuildResult) {
	if res.HasCorporateActions {
		fmt.Fprintf(w, "%s: skipped (corporate-action history present; rebuild not safe)\n", res.AccountName)
		return
	}
	fmt.Fprintf(w, "%s: %d position(s) recomputed, %d lot(s) recomputed\n",
		res.AccountName, res.PositionsRecomputed, res.LotsRecomputed)
}
