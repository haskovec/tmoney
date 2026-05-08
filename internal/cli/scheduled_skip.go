package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runSkipScheduled skips a scheduled transaction (advances to next date without posting).
func runSkipScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--skip-scheduled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse the scheduled transaction ID
	stID, err := types.ParseID(opts.skipScheduled)
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

	// Skip the scheduled transaction
	err = svc.Scheduled.Skip(stID)
	if err != nil {
		return fmt.Errorf("failed to skip scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := svc.Scheduled.GetByID(stID)

	// Get account info
	acct, _ := svc.AccountRepo.GetByID(st.AccountID)
	accountName := "Unknown"
	if acct != nil {
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
	fmt.Fprintln(w, "Scheduled transaction skipped!")
	fmt.Fprintf(w, "  Account:     %s\n", accountName)
	if payeeName != "-" {
		fmt.Fprintf(w, "  Payee:       %s\n", payeeName)
	}
	fmt.Fprintf(w, "  Frequency:   %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Skipped:     %s\n", oldNextDate.String())
	if stUpdated != nil && !stUpdated.IsCompleted() {
		fmt.Fprintf(w, "  Next:        %s\n", stUpdated.NextDate.String())
	} else {
		fmt.Fprintln(w, "  Status:      Completed (no more occurrences)")
	}

	autoBackupAfterModification(opts.file)
	return nil
}
