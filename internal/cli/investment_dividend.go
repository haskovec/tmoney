package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runDividend executes the --dividend command: record a cash dividend for a security.
func runDividend(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--dividend requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--dividend requires --account to specify an investment account")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--dividend requires --ticker to specify a security")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--dividend requires --amount to specify the dividend amount")
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

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to create transactions", opts.secTicker)
	}

	_, err = svc.Investment.Dividend(acct.ID, sec.ID, date, amount, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create dividend transaction: %w", err)
	}

	fmt.Fprintln(w, "Dividend transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", formatMoney(amount, acct.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}
