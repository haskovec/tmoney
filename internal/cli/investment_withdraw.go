package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runInvestWithdraw executes the --invest-withdraw command: withdraw cash from an investment account.
func runInvestWithdraw(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--invest-withdraw requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--invest-withdraw requires --account to specify an investment account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--invest-withdraw requires --amount to specify the withdrawal amount")
	}

	amount, err := types.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	var date types.Date
	if opts.txDate != "" {
		date, err = types.ParseDate(opts.txDate)
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

	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	_, err = svc.Investment.Withdrawal(acct.ID, date, amount, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create withdrawal transaction: %w", err)
	}

	fmt.Fprintln(w, "Investment withdrawal created successfully!")
	fmt.Fprintf(w, "  Account: %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:    %s\n", date.String())
	fmt.Fprintf(w, "  Amount:  %s\n", formatMoney(amount, acct.Currency))
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:    %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
