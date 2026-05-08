package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// runPostScheduled posts a scheduled transaction (creates a real transaction).
func runPostScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--post-scheduled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse the scheduled transaction ID
	stID, err := types.ParseID(opts.postScheduled)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	// Get the scheduled transaction first to show details
	st, err := svc.Scheduled.GetByID(stID)
	if err != nil {
		return fmt.Errorf("scheduled transaction not found: %w", err)
	}

	// Remember the old next date
	oldNextDate := st.NextDate

	// Parse optional amount
	var amount *types.Money
	if opts.txAmount != "" {
		amt, err := types.NewMoney(opts.txAmount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		amount = &amt
	}

	// Parse optional date
	var date *types.Date
	if opts.txDate != "" {
		d, err := types.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
		date = &d
	}

	// Post the scheduled transaction
	var txn *transaction.Transaction
	if date != nil {
		txn, err = svc.Scheduled.PostWithDate(stID, *date, amount)
	} else {
		txn, err = svc.Scheduled.Post(stID, amount)
	}
	if err != nil {
		return fmt.Errorf("failed to post scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := svc.Scheduled.GetByID(stID)

	// Get account info for currency
	acct, _ := svc.AccountRepo.GetByID(st.AccountID)
	currency := "USD"
	accountName := "Unknown"
	if acct != nil {
		currency = acct.Currency
		accountName = acct.Name
	}

	// Get payee name
	payeeName := "-"
	if st.HasPayee() {
		py, err := svc.PayeeRepo.GetByID(st.PayeeID.ID)
		if err == nil {
			payeeName = py.Name
		}
	}

	// Print confirmation
	fmt.Fprintln(w, "Scheduled transaction posted successfully!")
	fmt.Fprintf(w, "  Account:     %s\n", accountName)
	if payeeName != "-" {
		fmt.Fprintf(w, "  Payee:       %s\n", payeeName)
	}
	fmt.Fprintf(w, "  Amount:      %s\n", formatMoney(txn.Amount, currency))
	fmt.Fprintf(w, "  Date:        %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Frequency:   %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Previous:    %s\n", oldNextDate.String())
	if stUpdated != nil && !stUpdated.IsCompleted() {
		fmt.Fprintf(w, "  Next:        %s\n", stUpdated.NextDate.String())
	} else {
		fmt.Fprintln(w, "  Status:      Completed (no more occurrences)")
	}

	autoBackupAfterModification(opts.file)
	return nil
}
