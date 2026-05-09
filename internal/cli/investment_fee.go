package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentFeeOptions are the inputs to `tmoney investment fee`.
type investmentFeeOptions struct {
	file    string
	account string
	amount  string
	date    string
	memo    string
}

// newInvestmentFeeCmd registers `tmoney investment fee`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the root
// command. `--account` and `--amount` are required.
func newInvestmentFeeCmd() *cobra.Command {
	opts := &investmentFeeOptions{}
	cmd := &cobra.Command{
		Use:   "fee",
		Short: "Record a fee in an investment account",
		Long: "Record a fee transaction in an investment account. Cash is " +
			"debited from the account; share counts are unchanged.",
		Example:      "  tmoney investment fee --account Brokerage --amount 25 --memo \"Annual fee\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentFee(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Fee amount (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runInvestmentFee executes `tmoney investment fee`: record a fee in an
// investment account.
func runInvestmentFee(opts *investmentFeeOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
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

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	_, err = svc.Investment.Fee(acct.ID, date, amount, opts.memo)
	if err != nil {
		return fmt.Errorf("failed to create fee transaction: %w", err)
	}

	fmt.Fprintln(w, "Investment fee transaction created successfully!")
	fmt.Fprintf(w, "  Account: %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:    %s\n", date.String())
	fmt.Fprintf(w, "  Amount:  %s\n", formatMoney(amount, acct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:    %s\n", opts.memo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
