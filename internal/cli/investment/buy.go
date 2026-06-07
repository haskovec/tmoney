package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentBuyOptions are the inputs to `tmoney investment buy`.
type investmentBuyOptions struct {
	file          string
	account       string
	ticker        string
	shares        string
	amount        string
	pricePerShare string
	commission    string
	date          string
	memo          string
	catchUpSplits bool
}

// newInvestmentBuyCmd registers `tmoney investment buy`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--account`, `--ticker`, and `--shares` are required;
// at least one of `--amount` or `--price-per-share` must be supplied.
func newInvestmentBuyCmd() *cobra.Command {
	opts := &investmentBuyOptions{}
	cmd := &cobra.Command{
		Use:   "buy",
		Short: "Buy shares of a security in an investment account",
		Long: "Record a buy of shares in an investment account. " +
			"Supply either --amount (total cost) or --price-per-share, " +
			"or both. Cash is debited from the account; if lot tracking " +
			"is enabled on the account a new lot is opened.",
		Example: "  tmoney investment buy --account Brokerage --ticker AAPL --shares 10 --amount 1500\n" +
			"  tmoney investment buy --account Brokerage --ticker AAPL --shares 10 --price-per-share 150",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentBuy(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (required)")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "Number of shares (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Total amount (alternative or in addition to --price-per-share)")
	cmd.Flags().StringVar(&opts.pricePerShare, "price-per-share", "", "Price per share")
	cmd.Flags().StringVar(&opts.commission, "commission", "", "Commission amount (default 0)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().BoolVar(&opts.catchUpSplits, "catch-up-splits", false,
		"After the buy, apply any existing splits dated on/after this buy to the "+
			"new lot (repair for a back-dated buy on a lot-tracked account)")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("shares")
	return cmd
}

// runInvestmentBuy executes `tmoney investment buy`: buy shares of a
// security in an investment account.
func runInvestmentBuy(opts *investmentBuyOptions, w io.Writer) error {
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

	commission := types.ZeroMoney
	if opts.commission != "" {
		commission, err = types.NewMoney(opts.commission)
		if err != nil {
			return fmt.Errorf("invalid --commission: %w", err)
		}
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

	txn, err := svc.Investment.Buy(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, commission, opts.memo)
	if err != nil {
		return fmt.Errorf("failed to create buy transaction: %w", err)
	}

	fmt.Fprintln(w, "Buy transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())
	if txn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", cmdutil.FormatMoney(txn.PricePerShare.Money, acct.Currency))
	}
	if txn.Commission.Valid {
		fmt.Fprintf(w, "  Commission: %s\n", cmdutil.FormatMoney(txn.Commission.Money, acct.Currency))
	}
	fmt.Fprintf(w, "  Total:    %s\n", cmdutil.FormatMoney(txn.TotalAmount.Neg(), acct.Currency))

	if opts.catchUpSplits {
		applied, err := svc.CorporateAction.CatchUpSplitsForTransaction(txn.ID)
		if err != nil {
			return fmt.Errorf("buy saved, but catching up splits failed: %w", err)
		}
		switch applied {
		case 0:
			fmt.Fprintln(w, "  Catch-up splits: none applied (no later splits, or non-lot account)")
		case 1:
			fmt.Fprintln(w, "  Catch-up splits: 1 split applied to the new lot")
		default:
			fmt.Fprintf(w, "  Catch-up splits: %d splits applied to the new lot\n", applied)
		}
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
