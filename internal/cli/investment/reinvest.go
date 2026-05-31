package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentReinvestOptions are the inputs to `tmoney investment reinvest`.
type investmentReinvestOptions struct {
	file          string
	account       string
	ticker        string
	shares        string
	amount        string
	pricePerShare string
	date          string
	memo          string
}

// newInvestmentReinvestCmd registers `tmoney investment reinvest`. The
// database file is taken from the persistent `--file` / `-f` flag inherited
// from the root command. `--account`, `--ticker`, and `--shares` are
// required; at least one of `--amount` or `--price-per-share` must be
// supplied.
func newInvestmentReinvestCmd() *cobra.Command {
	opts := &investmentReinvestOptions{}
	cmd := &cobra.Command{
		Use:   "reinvest",
		Short: "Reinvest a dividend into additional shares of a security",
		Long: "Record a dividend reinvestment in an investment account: " +
			"shares are added to the position and no cash leaves the " +
			"account. Supply either --amount (total) or " +
			"--price-per-share, or both.",
		Example: "  tmoney investment reinvest --account Brokerage --ticker AAPL --shares 2 --price-per-share 150\n" +
			"  tmoney investment reinvest --account Brokerage --ticker AAPL --shares 2 --amount 300",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentReinvest(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (required)")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "Number of shares received (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Total dividend amount (alternative or in addition to --price-per-share)")
	cmd.Flags().StringVar(&opts.pricePerShare, "price-per-share", "", "Price per share")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("shares")
	return cmd
}

// runInvestmentReinvest executes `tmoney investment reinvest`: reinvest a
// dividend into additional shares of a security.
func runInvestmentReinvest(opts *investmentReinvestOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if opts.amount == "" && opts.pricePerShare == "" {
		return fmt.Errorf("--amount (total) and/or --price-per-share is required")
	}

	shares, err := types.NewQuantity(opts.shares)
	if err != nil {
		return fmt.Errorf("invalid --shares: %w", err)
	}

	var totalAmount *types.Money
	if opts.amount != "" {
		a, err := types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		totalAmount = &a
	}

	var pricePerShare *types.Money
	if opts.pricePerShare != "" {
		p, err := types.NewMoney(opts.pricePerShare)
		if err != nil {
			return fmt.Errorf("invalid --price-per-share: %w", err)
		}
		pricePerShare = &p
	}

	var date types.Date
	if opts.date != "" {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
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

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to create transactions", opts.ticker)
	}

	txn, err := svc.Investment.ReinvestDividend(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, opts.memo)
	if err != nil {
		return fmt.Errorf("failed to create reinvest dividend transaction: %w", err)
	}

	fmt.Fprintln(w, "Reinvest dividend transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())
	if txn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", cmdutil.FormatMoney(txn.PricePerShare.Money, acct.Currency))
	}
	fmt.Fprintf(w, "  Total:    %s\n", cmdutil.FormatMoney(txn.TotalAmount, acct.Currency))

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
