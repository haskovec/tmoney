package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// runSell executes the --sell command: sell shares of a security in an investment account.
func runSell(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--sell requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--sell requires --account to specify an investment account")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--sell requires --ticker to specify a security")
	}
	if opts.shares == "" {
		return fmt.Errorf("--sell requires --shares to specify the number of shares")
	}
	if opts.txAmount == "" && opts.pricePerShare == "" {
		return fmt.Errorf("--sell requires --amount (total) and/or --price-per-share")
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

	commission := types.ZeroMoney
	if opts.commission != "" {
		commission, err = types.NewMoney(opts.commission)
		if err != nil {
			return fmt.Errorf("invalid --commission: %w", err)
		}
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

	// Parse lot allocations if provided (for lot-tracking accounts)
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

	txn, err := svc.Investment.Sell(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, commission, opts.txMemo, lotAllocations)
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
