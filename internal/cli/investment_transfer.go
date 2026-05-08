package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// runTransferShares executes the --transfer-shares command: transfer shares between investment accounts.
func runTransferShares(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--transfer-shares requires --file to specify a database")
	}
	if opts.fromAccount == "" {
		return fmt.Errorf("--transfer-shares requires --from to specify the source account")
	}
	if opts.toAccount == "" {
		return fmt.Errorf("--transfer-shares requires --to to specify the destination account")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--transfer-shares requires --ticker to specify a security")
	}
	if opts.shares == "" {
		return fmt.Errorf("--transfer-shares requires --shares to specify the number of shares")
	}

	shares, err := types.NewQuantity(opts.shares)
	if err != nil {
		return fmt.Errorf("invalid --shares: %w", err)
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

	fromAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	toAcct, err := svc.Account.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to create transactions", opts.secTicker)
	}

	// Parse lot allocations if provided (for lot-tracking source accounts)
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

	result, err := svc.Investment.TransferShares(fromAcct.ID, toAcct.ID, sec.ID, date, shares, opts.txMemo, lotAllocations)
	if err != nil {
		return fmt.Errorf("failed to transfer shares: %w", err)
	}

	_ = result // used for linking; confirmation below covers it

	fmt.Fprintln(w, "Share transfer created successfully!")
	fmt.Fprintf(w, "  From:     %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:       %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())

	autoBackupAfterModification(opts.file)
	return nil
}
