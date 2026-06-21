package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentFeeLiquidationOptions are the inputs to
// `tmoney investment fee-liquidation`.
type investmentFeeLiquidationOptions struct {
	file          string
	account       string
	ticker        string
	isin          string
	name          string
	shares        string
	amount        string
	pricePerShare string
	commission    string
	date          string
	memo          string
	lot           string
}

// newInvestmentFeeLiquidationCmd registers `tmoney investment fee-liquidation`.
// The database file is taken from the persistent `--file` / `-f` flag inherited
// from the root command. `--account` and `--shares` are required; identify the
// security with `--ticker`, `--isin`, or `--name`; at least one of `--amount`
// or `--price-per-share` must be supplied.
func newInvestmentFeeLiquidationCmd() *cobra.Command {
	opts := &investmentFeeLiquidationOptions{}
	cmd := &cobra.Command{
		Use:   "fee-liquidation",
		Short: "Record a fee paid by liquidating shares of a security",
		Long: "Record a fee-via-liquidation in an investment account: shares of a " +
			"security are sold and the proceeds pay the fee, so there is no net " +
			"cash effect (the share count drops; cash is unchanged). This is how " +
			"some retirement plans charge fees against a fund instead of a cash " +
			"balance. Supply either --amount (the fee total) or --price-per-share, " +
			"or both; the third value is derived. For lot-tracked accounts pass " +
			"--lot to allocate against a specific open lot.",
		Example: "  tmoney investment fee-liquidation --account \"Fidelity 401k\" --ticker FXAIX --shares 0.123 --amount 5.00\n" +
			"  tmoney investment fee-liquidation --account \"Fidelity 401k\" --ticker FXAIX --shares 0.123 --price-per-share 40.65 --memo \"Q2 recordkeeping fee\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentFeeLiquidation(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker to liquidate (or use --isin / --name)")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "Number of shares to liquidate (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Fee total (alternative or in addition to --price-per-share)")
	cmd.Flags().StringVar(&opts.pricePerShare, "price-per-share", "", "Price per share")
	cmd.Flags().StringVar(&opts.commission, "commission", "", "Commission amount (default 0)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().StringVar(&opts.lot, "lot", "", "Lot ID to allocate against (lot-tracked accounts)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("shares")
	return cmd
}

// runInvestmentFeeLiquidation executes `tmoney investment fee-liquidation`:
// pay a fee by liquidating shares of a security in an investment account.
func runInvestmentFeeLiquidation(opts *investmentFeeLiquidationOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if opts.amount == "" && opts.pricePerShare == "" {
		return fmt.Errorf("--amount (fee total) and/or --price-per-share is required")
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

	sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
	if err != nil {
		return err
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to create transactions", cmdutil.SecurityRef(sec.Ticker, sec.Name))
	}

	// Non-lot accounts: leave allocations nil (feeLiquidationWithPosition ignores
	// them). Lot-tracked accounts: --lot builds a single allocation, which
	// feeLiquidationWithLots requires — exactly mirroring `investment sell`.
	var lotAllocations []investmentdom.SellLotAllocation
	if opts.lot != "" {
		lotID, err := types.ParseID(opts.lot)
		if err != nil {
			return fmt.Errorf("invalid --lot: %w", err)
		}
		lotAllocations = []investmentdom.SellLotAllocation{
			{LotID: lotID, Shares: shares},
		}
	}

	txn, err := svc.Investment.FeeLiquidation(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, commission, opts.memo, lotAllocations)
	if err != nil {
		return fmt.Errorf("failed to create fee-liquidation transaction: %w", err)
	}

	fmt.Fprintln(w, "Fee-liquidation transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s\n", cmdutil.SecurityDisplay(sec.Ticker, sec.Name))
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())
	if txn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", cmdutil.FormatMoney(txn.PricePerShare.Money, acct.Currency))
	}
	if txn.Commission.Valid {
		fmt.Fprintf(w, "  Commission: %s\n", cmdutil.FormatMoney(txn.Commission.Money, acct.Currency))
	}
	// TotalAmount is stored positive (no net cash effect) — print it as the fee.
	fmt.Fprintf(w, "  Fee:      %s\n", cmdutil.FormatMoney(txn.TotalAmount, acct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:     %s\n", opts.memo)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
