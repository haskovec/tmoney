package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentSellOptions are the inputs to `tmoney investment sell`.
type investmentSellOptions struct {
	file          string
	account       string
	ticker        string
	shares        string
	amount        string
	pricePerShare string
	commission    string
	date          string
	memo          string
	lot           string
}

// newInvestmentSellCmd registers `tmoney investment sell`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--account`, `--ticker`, and `--shares` are required;
// at least one of `--amount` or `--price-per-share` must be supplied.
func newInvestmentSellCmd() *cobra.Command {
	opts := &investmentSellOptions{}
	cmd := &cobra.Command{
		Use:   "sell",
		Short: "Sell shares of a security in an investment account",
		Long: "Record a sell of shares in an investment account. " +
			"Supply either --amount (total proceeds) or --price-per-share, " +
			"or both. Cash is credited to the account; for lot-tracked " +
			"accounts pass --lot to allocate against a specific open lot.",
		Example: "  tmoney investment sell --account Brokerage --ticker AAPL --shares 5 --price-per-share 160\n" +
			"  tmoney investment sell --account Brokerage --ticker AAPL --shares 5 --amount 800 --commission 10",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentSell(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (required)")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "Number of shares (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Total proceeds (alternative or in addition to --price-per-share)")
	cmd.Flags().StringVar(&opts.pricePerShare, "price-per-share", "", "Price per share")
	cmd.Flags().StringVar(&opts.commission, "commission", "", "Commission amount (default 0)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().StringVar(&opts.lot, "lot", "", "Lot ID to allocate the sell against (lot-tracked accounts)")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("shares")
	return cmd
}

// runInvestmentSell executes `tmoney investment sell`: sell shares of a
// security in an investment account.
func runInvestmentSell(opts *investmentSellOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
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

	database, svc, err := openServices(opts.file)
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

	var lotAllocations []investment.SellLotAllocation
	if opts.lot != "" {
		lotID, err := types.ParseID(opts.lot)
		if err != nil {
			return fmt.Errorf("invalid --lot: %w", err)
		}
		lotAllocations = []investment.SellLotAllocation{
			{LotID: lotID, Shares: shares},
		}
	}

	txn, err := svc.Investment.Sell(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, commission, opts.memo, lotAllocations)
	if err != nil {
		return fmt.Errorf("failed to create sell transaction: %w", err)
	}

	fmt.Fprintln(w, "Sell transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())
	if txn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", formatMoney(txn.PricePerShare.Money, acct.Currency))
	}
	if txn.Commission.Valid {
		fmt.Fprintf(w, "  Commission: %s\n", formatMoney(txn.Commission.Money, acct.Currency))
	}
	fmt.Fprintf(w, "  Total:    %s\n", formatMoney(txn.TotalAmount, acct.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}
