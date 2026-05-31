package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentWithdrawOptions are the inputs to `tmoney investment withdraw`.
type investmentWithdrawOptions struct {
	file    string
	account string
	amount  string
	date    string
	memo    string
}

// newInvestmentWithdrawCmd registers `tmoney investment withdraw`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--account` and `--amount` are
// required.
func newInvestmentWithdrawCmd() *cobra.Command {
	opts := &investmentWithdrawOptions{}
	cmd := &cobra.Command{
		Use:   "withdraw",
		Short: "Withdraw cash from an investment account",
		Long: "Withdraw cash from an investment account. Cash is debited " +
			"from the account; share counts are unchanged.",
		Example:      "  tmoney investment withdraw --account Brokerage --amount 500 --memo \"Quarterly draw\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentWithdraw(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Withdrawal amount (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("account")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runInvestmentWithdraw executes `tmoney investment withdraw`: withdraw
// cash from an investment account.
func runInvestmentWithdraw(opts *investmentWithdrawOptions, w io.Writer) error {
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

	_, err = svc.Investment.Withdrawal(acct.ID, date, amount, opts.memo)
	if err != nil {
		return fmt.Errorf("failed to create withdrawal transaction: %w", err)
	}

	fmt.Fprintln(w, "Investment withdrawal created successfully!")
	fmt.Fprintf(w, "  Account: %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:    %s\n", date.String())
	fmt.Fprintf(w, "  Amount:  %s\n", cmdutil.FormatMoney(amount, acct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:    %s\n", opts.memo)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
