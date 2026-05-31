package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentPortfolioOptions are the inputs to `tmoney investment portfolio`.
type investmentPortfolioOptions struct {
	file          string
	account       string
	asOf          string
	showLots      bool
	includeClosed bool
}

// newInvestmentPortfolioCmd registers `tmoney investment portfolio`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--account` is required.
func newInvestmentPortfolioCmd() *cobra.Command {
	opts := &investmentPortfolioOptions{}
	cmd := &cobra.Command{
		Use:   "portfolio",
		Short: "Show investment portfolio holdings and summary for an account",
		Long: "Show holdings, market value, cost basis, and gain/loss for " +
			"an investment account. Pass --as-of to value the portfolio at " +
			"a specific date, --show-lots to drill into per-lot detail on " +
			"lot-tracking accounts, or --include-closed to surface fully-" +
			"sold positions in a separate Closed positions section.",
		Example: "  tmoney investment portfolio --account Brokerage\n" +
			"  tmoney investment portfolio --account Brokerage --as-of 2024-12-31\n" +
			"  tmoney investment portfolio --account Brokerage --show-lots\n" +
			"  tmoney investment portfolio --account Brokerage --include-closed",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentPortfolio(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.asOf, "as-of", "", "Valuation date YYYY-MM-DD (default today)")
	cmd.Flags().BoolVar(&opts.showLots, "show-lots", false, "Show per-lot detail (lot-tracking accounts only)")
	cmd.Flags().BoolVar(&opts.includeClosed, "include-closed", false, "Include fully-sold positions in a separate Closed positions section")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runInvestmentPortfolio executes `tmoney investment portfolio`: show the
// investment portfolio for an account.
func runInvestmentPortfolio(opts *investmentPortfolioOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	var asOf types.Date
	if opts.asOf != "" {
		var err error
		asOf, err = types.ParseDate(opts.asOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
	} else {
		asOf = types.Today()
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	valuation, err := svc.Investment.GetAccountValuation(acct.ID, asOf, investmentdom.ValuationOptions{IncludeClosed: opts.includeClosed})
	if err != nil {
		return fmt.Errorf("failed to get portfolio valuation: %w", err)
	}

	securityMap := make(map[types.ID]*security.Security)
	for _, h := range valuation.Holdings {
		sec, secErr := svc.Security.GetByID(h.SecurityID)
		if secErr == nil {
			securityMap[h.SecurityID] = sec
		}
	}

	if opts.showLots && acct.TrackLots {
		printPortfolioWithLots(w, acct, valuation, securityMap, svc, asOf)
	} else {
		printPortfolioSummary(w, acct, valuation, securityMap)
	}

	if !opts.includeClosed && valuation.HasClosedPositions {
		fmt.Fprintf(w, "Hint: --include-closed adds %d closed-position rows.\n", valuation.ClosedPositionCount)
	}

	return nil
}
