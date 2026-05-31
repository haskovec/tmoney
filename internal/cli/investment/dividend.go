package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentDividendOptions are the inputs to `tmoney investment dividend`.
type investmentDividendOptions struct {
	file    string
	account string
	ticker  string
	amount  string
	date    string
	memo    string
}

// newInvestmentDividendCmd registers `tmoney investment dividend`. The
// database file is taken from the persistent `--file` / `-f` flag inherited
// from the root command. `--account`, `--ticker`, and `--amount` are
// required.
func newInvestmentDividendCmd() *cobra.Command {
	opts := &investmentDividendOptions{}
	cmd := &cobra.Command{
		Use:   "dividend",
		Short: "Record a cash dividend for a security",
		Long: "Record a cash dividend received for a security in an " +
			"investment account. Cash is credited to the account; the " +
			"share count is unchanged.",
		Example:      "  tmoney investment dividend --account Brokerage --ticker AAPL --amount 125.50",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentDividend(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Dividend amount (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runInvestmentDividend executes `tmoney investment dividend`: record a
// cash dividend for a security.
func runInvestmentDividend(opts *investmentDividendOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	amount, err := types.NewMoney(opts.amount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
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

	if _, err := svc.Investment.Dividend(acct.ID, sec.ID, date, amount, opts.memo); err != nil {
		return fmt.Errorf("failed to create dividend transaction: %w", err)
	}

	fmt.Fprintln(w, "Dividend transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", cmdutil.FormatMoney(amount, acct.Currency))

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
