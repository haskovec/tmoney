package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runReinvest executes the --reinvest command: reinvest a dividend into additional shares.
func runReinvest(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--reinvest requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--reinvest requires --account to specify an investment account")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--reinvest requires --ticker to specify a security")
	}
	if opts.shares == "" {
		return fmt.Errorf("--reinvest requires --shares to specify the number of shares")
	}
	if opts.txAmount == "" && opts.pricePerShare == "" {
		return fmt.Errorf("--reinvest requires --amount (total) and/or --price-per-share")
	}

	shares, err := types.NewQuantity(opts.shares)
	if err != nil {
		return fmt.Errorf("invalid --shares: %w", err)
	}

	var totalAmount *types.Money
	if opts.txAmount != "" {
		a, err := types.NewMoney(opts.txAmount)
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

	txn, err := svc.Investment.ReinvestDividend(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create reinvest dividend transaction: %w", err)
	}

	fmt.Fprintln(w, "Reinvest dividend transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())
	if txn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", formatMoney(txn.PricePerShare.Money, acct.Currency))
	}
	fmt.Fprintf(w, "  Total:    %s\n", formatMoney(txn.TotalAmount, acct.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}
