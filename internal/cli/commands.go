package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/types"
)

// openServices opens the database and creates all services via the shared registry.
// It also does a best-effort update of the recent files in the config.
// Auto-posts due scheduled transactions and prints a summary if any were posted.
func openServices(file string) (*db.DB, *app.Services, error) {
	database, err := db.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Best-effort update recent files
	if cfg, err := config.Load(); err == nil {
		cfg.AddRecentFile(file)
		_ = cfg.Save()
	}

	svc := app.NewServices(database)

	// Auto-post due scheduled transactions on file open
	if summary, err := svc.Scheduled.AutoPost(); err == nil && summary.PostedCount > 0 {
		fmt.Fprintf(os.Stdout, "Auto-posted %d scheduled transaction(s)\n", summary.PostedCount)
	}

	return database, svc, nil
}

// runTransfer creates a transfer between two accounts.
func runTransfer(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--transfer requires --file to specify a database")
	}
	if opts.fromAccount == "" {
		return fmt.Errorf("--transfer requires --from to specify the source account")
	}
	if opts.toAccount == "" {
		return fmt.Errorf("--transfer requires --to to specify the destination account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--transfer requires --amount to specify the transfer amount")
	}

	// Parse amount
	amount, err := types.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Amount must be positive for transfers
	if !amount.IsPositive() {
		return fmt.Errorf("--amount must be positive for transfers")
	}

	// Parse date (default to today)
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

	// Get source account by name
	fromAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	// Get destination account by name
	toAcct, err := svc.Account.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	// Create the transfer
	pair, err := svc.Transaction.CreateTransfer(fromAcct.ID, toAcct.ID, date, amount)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	// Set memo if provided
	if opts.txMemo != "" {
		err = svc.Transaction.UpdateTransfer(pair.FromTransaction.TransferID.ID, date, amount, opts.txMemo, transaction.StatusUncleared)
		if err != nil {
			return fmt.Errorf("failed to set memo on transfer: %w", err)
		}
	}

	// Print confirmation
	fmt.Fprintln(w, "Transfer created successfully!")
	fmt.Fprintf(w, "  From:   %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:     %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Date:   %s\n", date.String())
	fmt.Fprintf(w, "  Amount: %s\n", formatMoney(amount, fromAcct.Currency))
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:   %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runScheduled lists scheduled transactions.
func runScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--scheduled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get scheduled transactions
	var scheduledTxns []*scheduled.Transaction
	if opts.scheduledDue {
		scheduledTxns, err = svc.Scheduled.ListDue()
	} else {
		scheduledTxns, err = svc.Scheduled.List()
	}
	if err != nil {
		return fmt.Errorf("failed to list scheduled transactions: %w", err)
	}

	// Filter by account if specified
	if opts.accountName != "" {
		acct, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}

		var filtered []*scheduled.Transaction
		for _, st := range scheduledTxns {
			if st.AccountID == acct.ID {
				filtered = append(filtered, st)
			}
		}
		scheduledTxns = filtered
	}

	// Build lookup maps
	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)
	accountNames := make(map[types.ID]string)
	accountCurrencies := make(map[types.ID]string)

	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	accounts, _ := svc.AccountRepo.List(false)
	for _, a := range accounts {
		accountNames[a.ID] = a.Name
		accountCurrencies[a.ID] = a.Currency
	}

	// Print scheduled transactions table
	printScheduledTransactionsTable(w, scheduledTxns, opts.scheduledDue, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}

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

// runAddScheduled creates a new scheduled transaction.
func runAddScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-scheduled requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--add-scheduled requires --account to specify an account")
	}
	if opts.stFrequency == "" {
		return fmt.Errorf("--add-scheduled requires --frequency to specify a frequency")
	}

	// Parse frequency
	frequency, err := scheduled.ParseFrequency(opts.stFrequency)
	if err != nil {
		validFreqs := []string{}
		for _, f := range scheduled.AllFrequencies() {
			validFreqs = append(validFreqs, string(f))
		}
		return fmt.Errorf("invalid --frequency %q: valid values are %s", opts.stFrequency, strings.Join(validFreqs, ", "))
	}

	// Parse start date (default to today)
	var startDate types.Date
	if opts.txDate != "" {
		startDate, err = types.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		startDate = types.Today()
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Create scheduled transaction
	st := scheduled.NewTransaction(acct.ID, frequency, startDate)

	// Handle amount (optional - null means variable amount)
	if opts.txAmount != "" {
		amount, err := types.NewMoney(opts.txAmount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		st.SetAmount(amount)
	}

	// Handle payee
	var payeeName string
	if opts.txPayee != "" {
		py, _, err := svc.Payee.GetOrCreate(opts.txPayee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		st.SetPayee(py.ID)
		payeeName = py.Name
	}

	// Handle category
	var categoryName string
	if opts.txCategory != "" {
		cat, err := svc.CategoryRepo.GetByName(opts.txCategory, nil)
		if err != nil {
			categories, listErr := svc.CategoryRepo.List()
			if listErr != nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			var found *category.Category
			for _, c := range categories {
				if c.Name == opts.txCategory {
					found = c
					break
				}
			}
			if found == nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			cat = found
		}
		st.SetCategory(cat.ID)
		categoryName = cat.Name
	}

	// Handle memo
	if opts.txMemo != "" {
		st.SetMemo(opts.txMemo)
	}

	// Handle day of month
	if opts.stDay != "" {
		day, err := strconv.Atoi(opts.stDay)
		if err != nil {
			return fmt.Errorf("invalid --day: %w", err)
		}
		st.SetDayOfMonth(day)
	}

	// Handle occurrences
	if opts.stOccurrences != "" {
		occ, err := strconv.ParseInt(opts.stOccurrences, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid --occurrences: %w", err)
		}
		st.SetOccurrences(occ)
	}

	// Handle end date
	if opts.stEndDate != "" {
		endDate, err := types.ParseDate(opts.stEndDate)
		if err != nil {
			return fmt.Errorf("invalid --end-date: %w", err)
		}
		st.SetEndDate(endDate)
	}

	// Handle auto-post
	if opts.autoPost {
		st.SetAutoPost(true)
	}

	// Handle lead days
	if opts.leadDays != "" {
		days, err := strconv.Atoi(opts.leadDays)
		if err != nil {
			return fmt.Errorf("invalid --lead-days: %w", err)
		}
		if days != 0 && days != 3 && days != 7 {
			return fmt.Errorf("--lead-days must be 0, 3, or 7")
		}
		if !opts.autoPost {
			return fmt.Errorf("--lead-days requires --auto-post")
		}
		st.SetPostLeadDays(days)
	}

	// Save scheduled transaction
	if err := svc.Scheduled.Create(st); err != nil {
		return fmt.Errorf("failed to create scheduled transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Scheduled transaction created successfully!")
	fmt.Fprintf(w, "  Account:   %s\n", acct.Name)
	fmt.Fprintf(w, "  Frequency: %s\n", frequency.DisplayName())
	fmt.Fprintf(w, "  Next Date: %s\n", st.NextDate.String())
	if st.HasAmount() {
		fmt.Fprintf(w, "  Amount:    %s\n", formatMoney(st.Amount.Money, acct.Currency))
	} else {
		fmt.Fprintf(w, "  Amount:    Variable\n")
	}
	if payeeName != "" {
		fmt.Fprintf(w, "  Payee:     %s\n", payeeName)
	}
	if categoryName != "" {
		fmt.Fprintf(w, "  Category:  %s\n", categoryName)
	}
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:      %s\n", opts.txMemo)
	}
	if st.AutoPost {
		if st.PostLeadDays > 0 {
			fmt.Fprintf(w, "  Auto-post: Yes (%d days early)\n", st.PostLeadDays)
		} else {
			fmt.Fprintf(w, "  Auto-post: Yes\n")
		}
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runReport generates and displays reports.
func runReport(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--report requires --file to specify a database")
	}

	// Validate report type
	if opts.reportType == "" {
		return fmt.Errorf("--report requires a report type (net-worth or spending)")
	}

	switch opts.reportType {
	case "net-worth":
		return runNetWorthReport(opts, w)
	case "spending":
		return runSpendingReport(opts, w)
	default:
		return fmt.Errorf("unknown report type %q: valid types are net-worth, spending", opts.reportType)
	}
}

// runNetWorthReport generates and displays the net worth report.
func runNetWorthReport(opts *cliOptions, w io.Writer) error {
	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Determine as-of date
	var asOf time.Time
	if opts.reportAsOf != "" {
		d, err := types.ParseDate(opts.reportAsOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
		asOf = time.Time(d)
	} else {
		asOf = time.Now()
	}

	// Generate report
	var rpt *report.NetWorth
	if opts.includeClosed {
		rpt, err = svc.Report.NetWorthAsOfIncludingClosed(asOf)
	} else {
		rpt, err = svc.Report.NetWorthAsOf(asOf)
	}
	if err != nil {
		return fmt.Errorf("failed to generate net worth report: %w", err)
	}

	// Print report
	printNetWorthReport(w, rpt)
	return nil
}

// runSpendingReport generates and displays the spending by category report.
func runSpendingReport(opts *cliOptions, w io.Writer) error {
	// Validate that we have a time period
	if opts.reportMonth == "" && opts.reportYear == 0 && opts.fromDate == "" {
		return fmt.Errorf("--report spending requires --month YYYY-MM, --year YYYY, or --from/--to date range")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Generate report based on period type
	var rpt *report.Spending

	if opts.reportMonth != "" {
		// Parse YYYY-MM format
		year, month, err := parseYearMonth(opts.reportMonth)
		if err != nil {
			return fmt.Errorf("invalid --month format: %w", err)
		}
		rpt, err = svc.Report.SpendingByCategoryMonth(year, month)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	} else if opts.reportYear != 0 {
		rpt, err = svc.Report.SpendingByCategoryYear(opts.reportYear)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	} else if opts.fromDate != "" {
		// Custom date range
		startDate, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}

		var endDate types.Date
		if opts.toDate != "" {
			endDate, err = types.ParseDate(opts.toDate)
			if err != nil {
				return fmt.Errorf("invalid --to date: %w", err)
			}
		} else {
			endDate = types.Today()
		}

		rpt, err = svc.Report.SpendingByCategoryDateRange(time.Time(startDate), time.Time(endDate))
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	}

	// Print report
	printSpendingReport(w, rpt)
	return nil
}

// runStartReconcile starts a reconciliation session for an account.
func runStartReconcile(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--start-reconcile requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--start-reconcile requires --account to specify an account")
	}
	if opts.statementDate == "" {
		return fmt.Errorf("--start-reconcile requires --statement-date")
	}
	if opts.statementBalance == "" {
		return fmt.Errorf("--start-reconcile requires --statement-balance")
	}

	// Parse statement date
	stmtDate, err := types.ParseDate(opts.statementDate)
	if err != nil {
		return fmt.Errorf("invalid --statement-date: %w", err)
	}

	// Parse statement balance
	stmtBalance, err := types.NewMoney(opts.statementBalance)
	if err != nil {
		return fmt.Errorf("invalid --statement-balance: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Start reconciliation
	session, err := svc.Reconciliation.StartReconciliation(account.ID, stmtDate, stmtBalance)
	if err != nil {
		return fmt.Errorf("failed to start reconciliation: %w", err)
	}

	// Get candidate transaction count
	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, stmtDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	_ = session // session created successfully
	fmt.Fprintf(w, "Reconciliation started for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:    %s\n", stmtDate.String())
	fmt.Fprintf(w, "  Statement balance: %s\n", formatMoney(stmtBalance, account.Currency))
	fmt.Fprintf(w, "  Unreconciled transactions: %d\n", len(candidates))

	autoBackupAfterModification(opts.file)
	return nil
}

// runMarkReconciled marks transactions for reconciliation.
func runMarkReconciled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--mark-reconciled requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse transaction IDs
	var txnIDs []types.ID
	for _, idStr := range opts.markReconciled {
		id, err := types.ParseID(idStr)
		if err != nil {
			return fmt.Errorf("invalid transaction ID %q: %w", idStr, err)
		}
		txnIDs = append(txnIDs, id)
	}

	// Get the first transaction to find its account
	firstTxn, err := svc.TransactionRepo.GetByID(txnIDs[0])
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	// Get active session for this account
	session, err := svc.Reconciliation.GetActiveSession(firstTxn.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for this account; use --start-reconcile first")
	}

	// Calculate cleared total with these transactions marked
	clearedTotal, err := svc.Reconciliation.CalculateClearedTotal(firstTxn.AccountID, txnIDs)
	if err != nil {
		return fmt.Errorf("failed to calculate cleared total: %w", err)
	}

	difference := session.StatementBalance.Sub(clearedTotal)

	// Get account for currency
	account, _ := svc.AccountRepo.GetByID(firstTxn.AccountID)
	currency := "USD"
	if account != nil {
		currency = account.Currency
	}

	fmt.Fprintf(w, "Marked %d transaction(s) for reconciliation\n", len(txnIDs))
	fmt.Fprintf(w, "  Current difference: %s\n", formatMoney(difference, currency))

	return nil
}

// runFinishReconcile completes the reconciliation for an account.
func runFinishReconcile(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--finish-reconcile requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--finish-reconcile requires --account to specify an account")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Get active session
	session, err := svc.Reconciliation.GetActiveSession(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for %s", account.Name)
	}

	// Get all candidate transactions to mark as reconciled
	candidates, err := svc.Reconciliation.GetCandidateTransactions(account.ID, session.StatementDate)
	if err != nil {
		return fmt.Errorf("failed to get candidate transactions: %w", err)
	}

	// Collect all candidate transaction IDs
	var txnIDs []types.ID
	for _, txn := range candidates {
		txnIDs = append(txnIDs, txn.ID)
	}

	// Finish reconciliation
	err = svc.Reconciliation.FinishReconciliation(account.ID, txnIDs, opts.reconcileForce)
	if err != nil {
		// Check for difference error and provide helpful message
		if diffErr, ok := err.(*reconciliation.DifferenceError); ok {
			return fmt.Errorf("cannot complete reconciliation. Difference: %s\nUse --mark-reconciled to mark additional transactions, or --force to complete anyway",
				formatMoney(diffErr.Difference, account.Currency))
		}
		return fmt.Errorf("failed to finish reconciliation: %w", err)
	}

	fmt.Fprintf(w, "Reconciliation completed for %s\n", account.Name)
	fmt.Fprintf(w, "  Statement date:         %s\n", session.StatementDate.String())
	fmt.Fprintf(w, "  Transactions reconciled: %d\n", len(txnIDs))
	fmt.Fprintf(w, "  Balance:                %s\n", formatMoney(session.StatementBalance, account.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}

// runReconcileStatus shows the reconciliation status for an account.
func runReconcileStatus(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--reconcile-status requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--reconcile-status requires --account to specify an account")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Get reconciliation status
	status, err := svc.Reconciliation.GetReconciliationStatus(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get reconciliation status: %w", err)
	}

	printReconcileStatus(w, account, status)
	return nil
}

// autoBackupAfterModification creates an auto-backup after a data-modifying CLI command.
func autoBackupAfterModification(dbPath string) {
	// Best-effort: don't fail the CLI command if backup fails
	_, _ = backup.CreateAutoBackup(dbPath)
}

// runImport handles the --import command.
func runImport(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--import requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--import requires --account to specify the target account")
	}
	if opts.skipDuplicates && opts.updateDuplicates {
		return fmt.Errorf("--skip-duplicates and --update-duplicates are mutually exclusive")
	}

	// Detect or override format
	var format imexport.Format
	if opts.formatOverride != "" {
		switch strings.ToLower(opts.formatOverride) {
		case "csv":
			format = imexport.FormatCSV
		case "qif":
			format = imexport.FormatQIF
		case "ofx", "qfx":
			format = imexport.FormatOFX
		default:
			return fmt.Errorf("unsupported --format value %q (must be csv, qif, or ofx)", opts.formatOverride)
		}
	} else {
		var err error
		format, err = imexport.DetectFormat(opts.importFile)
		if err != nil {
			return fmt.Errorf("cannot detect format: %w\nUse --format to specify the format explicitly", err)
		}
	}

	// Open the import file
	file, err := os.Open(opts.importFile)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()

	// Open database and services
	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Resolve the target account
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found: %w", opts.accountName, err)
	}
	if !account.Active {
		return fmt.Errorf("account %q is closed; cannot import into a closed account", opts.accountName)
	}

	// Determine duplicate handling
	dupHandling := imexport.DuplicateHandlingNone
	if opts.skipDuplicates {
		dupHandling = imexport.DuplicateHandlingSkip
	} else if opts.updateDuplicates {
		dupHandling = imexport.DuplicateHandlingUpdate
	}

	// Create import service with adapters
	importSvc := imexport.NewImportService(
		imexport.NewServiceCategoryResolver(svc.Category),
		imexport.NewServicePayeeResolver(svc.Payee),
		imexport.NewRepoTransactionStore(svc.TransactionRepo, svc.PayeeRepo),
		imexport.NewServiceTransactionCreator(svc.Transaction),
	)

	// Parse the file once, then check whether it contains rows for more
	// than one source account (Quicken Mac's "Register Transactions to
	// CSV" emits a single file covering every account). If so, the user
	// must pick which one to import via --source-account.
	parseResult, err := importSvc.Parse(file, format)
	if err != nil {
		return fmt.Errorf("import parse failed: %w", err)
	}
	sources := imexport.DistinctAccounts(parseResult)
	if len(sources) > 1 && opts.sourceAccount == "" {
		return fmt.Errorf("import file contains transactions for %d accounts: %s\n"+
			"Pass --source-account \"<name>\" to choose which one to import (run once per account)",
			len(sources), strings.Join(sources, ", "))
	}
	if opts.sourceAccount != "" {
		if len(sources) > 0 && !slices.Contains(sources, opts.sourceAccount) {
			return fmt.Errorf("source account %q not found in import file (available: %s)",
				opts.sourceAccount, strings.Join(sources, ", "))
		}
		parseResult = imexport.FilterByAccount(parseResult, opts.sourceAccount)
	}

	// Run preview from the (possibly filtered) records
	importOpts := imexport.ImportOptions{
		Format:            format,
		DuplicateHandling: dupHandling,
	}
	result, err := importSvc.PreviewRecords(parseResult, account.ID, importOpts)
	if err != nil {
		return fmt.Errorf("import preview failed: %w", err)
	}

	// If not confirming, show dry-run summary
	if !opts.confirm {
		printImportPreview(w, opts.importFile, opts.accountName, result)
		return nil
	}

	// Execute the import
	if err := importSvc.Execute(result, account.ID); err != nil {
		return fmt.Errorf("import execution failed: %w", err)
	}

	// Print execution summary
	printImportResult(w, opts.importFile, opts.accountName, result)

	autoBackupAfterModification(opts.file)
	return nil
}

// runExport exports transactions to a file in CSV or QIF format.
func runExport(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--export requires --file to specify a database")
	}

	// Detect or override format
	var format imexport.Format
	if opts.formatOverride != "" {
		switch strings.ToLower(opts.formatOverride) {
		case "csv":
			format = imexport.FormatCSV
		case "qif":
			format = imexport.FormatQIF
		default:
			return fmt.Errorf("unsupported export --format value %q (must be csv or qif)", opts.formatOverride)
		}
	} else {
		detected, err := imexport.DetectFormat(opts.exportFile)
		if err != nil {
			return fmt.Errorf("cannot detect format: %w\nUse --format to specify the format explicitly", err)
		}
		if detected == imexport.FormatOFX {
			return fmt.Errorf("OFX format is not supported for export; use csv or qif")
		}
		format = detected
	}

	// Open database and services
	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Build export options
	exportOpts := imexport.ExportOptions{
		Format: format,
	}

	// Resolve account filter
	if opts.accountName != "" {
		account, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found: %w", opts.accountName, err)
		}
		exportOpts.AccountID = &account.ID
	}

	// Parse date filters
	if opts.fromDate != "" {
		d, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		exportOpts.StartDate = &d
	}
	if opts.toDate != "" {
		d, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		exportOpts.EndDate = &d
	}

	// Create export service using repositories directly (they satisfy the provider interfaces)
	exportSvc := imexport.NewExportService(
		svc.AccountRepo,
		svc.TransactionRepo,
		svc.SplitRepo,
		svc.PayeeRepo,
		svc.CategoryRepo,
	)

	// Create output file
	outFile, err := os.Create(opts.exportFile)
	if err != nil {
		return fmt.Errorf("failed to create export file: %w", err)
	}
	defer outFile.Close()

	// Run export
	result, err := exportSvc.Export(outFile, exportOpts)
	if err != nil {
		// Clean up the file on error
		_ = outFile.Close()
		_ = os.Remove(opts.exportFile)
		return fmt.Errorf("export failed: %w", err)
	}

	// Print summary
	fmt.Fprintf(w, "EXPORT COMPLETE: %s\n", filepath.Base(opts.exportFile))
	fmt.Fprintln(w, strings.Repeat("=", 40))
	fmt.Fprintf(w, "Format:       %s\n", strings.ToUpper(string(format)))
	fmt.Fprintf(w, "Accounts:     %d\n", result.AccountCount)
	fmt.Fprintf(w, "Transactions: %d\n", result.TransactionCount)
	fmt.Fprintf(w, "Output file:  %s\n", opts.exportFile)

	return nil
}

// printImportPreview prints the dry-run import summary.
func printImportPreview(w io.Writer, importFile, accountName string, result *imexport.ImportResult) {
	fmt.Fprintf(w, "IMPORT PREVIEW: %s → %s\n", filepath.Base(importFile), accountName)
	fmt.Fprintln(w, strings.Repeat("=", 44))
	fmt.Fprintf(w, "Parsed: %d transactions\n", len(result.Rows))
	fmt.Fprintf(w, "  New:      %3d transactions (will be created)\n", result.NewCount())
	fmt.Fprintf(w, "  Matched:  %3d transactions (will be updated)\n", result.MatchCount())
	fmt.Fprintf(w, "  Review:   %3d transactions (low-confidence match)\n", result.ReviewCount())
	fmt.Fprintf(w, "  Skipped:  %3d transactions (duplicates)\n", result.SkipCount())

	if len(result.Rows) > 0 {
		fmt.Fprintf(w, "\nDate range: %s to %s\n", result.DateFrom.String(), result.DateTo.String())
		fmt.Fprintf(w, "Total amount: $%.2f\n", result.TotalAmount().Float64())
	}

	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "\nWarnings:\n")
		for _, e := range result.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	fmt.Fprintln(w, "\nRun with --confirm to execute the import.")
}

// printImportResult prints the import execution summary.
func printImportResult(w io.Writer, importFile, accountName string, result *imexport.ImportResult) {
	fmt.Fprintf(w, "IMPORT COMPLETE: %s → %s\n", filepath.Base(importFile), accountName)
	fmt.Fprintln(w, strings.Repeat("=", 45))
	fmt.Fprintf(w, "Created:  %d new transactions\n", result.Created)
	fmt.Fprintf(w, "Updated:  %d existing transactions\n", result.Updated)
	fmt.Fprintf(w, "Skipped:  %d duplicates\n", result.Skipped)

	if len(result.Errors) > 0 {
		fmt.Fprintf(w, "\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}
}

// runLinkTransfers handles the --link-transfers command. By default it
// performs a dry-run preview of the candidate pairs that would be linked;
// passing --confirm executes the linking.
func runLinkTransfers(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--link-transfers requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	maxDays := opts.maxDateDiffDays
	if maxDays == 0 {
		maxDays = transferlink.DefaultMaxDateDiffDays
	}

	result, err := svc.TransferLink.FindUnlinked(maxDays)
	if err != nil {
		return fmt.Errorf("scan for transfer candidates failed: %w", err)
	}

	if !opts.confirm {
		printLinkTransferPreview(w, result, maxDays)
		return nil
	}

	linked, errs := svc.TransferLink.Link(result.Clean)
	fmt.Fprintf(w, "LINK TRANSFERS COMPLETE\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 40))
	fmt.Fprintf(w, "Linked:    %d pairs\n", linked)
	fmt.Fprintf(w, "Ambiguous: %d pairs (left untouched — review by hand)\n", len(result.Ambiguous))
	if len(errs) > 0 {
		fmt.Fprintf(w, "\nErrors:\n")
		for _, e := range errs {
			fmt.Fprintf(w, "  - %s\n", e)
		}
		return fmt.Errorf("%d link errors", len(errs))
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// printLinkTransferPreview renders a dry-run summary of FindUnlinked.
func printLinkTransferPreview(w io.Writer, result *transferlink.Result, maxDays int) {
	fmt.Fprintf(w, "LINK TRANSFERS PREVIEW (window: %d days)\n", maxDays)
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 40))
	fmt.Fprintf(w, "Scanned:   %d eligible transactions\n", result.Scanned)
	fmt.Fprintf(w, "Clean:     %d pairs (will be linked)\n", len(result.Clean))
	fmt.Fprintf(w, "Ambiguous: %d pairs (need manual review)\n\n", len(result.Ambiguous))

	if len(result.Clean) > 0 {
		fmt.Fprintln(w, "Clean pairs:")
		writeCandidateTable(w, result.Clean)
	}
	if len(result.Ambiguous) > 0 {
		fmt.Fprintln(w, "\nAmbiguous pairs:")
		writeCandidateTable(w, result.Ambiguous)
	}

	if len(result.Clean) > 0 {
		fmt.Fprintf(w, "\nRun with --confirm to link the %d clean pairs.\n", len(result.Clean))
	} else {
		fmt.Fprintln(w, "\nNothing to link.")
	}
}

func writeCandidateTable(w io.Writer, cs []*transferlink.Candidate) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  From date\tFrom account\tAmount\tTo date\tTo account\tΔ days")
	for _, c := range cs {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%d\n",
			c.From.Date.String(),
			c.FromAccount,
			c.From.Amount.String(),
			c.To.Date.String(),
			c.ToAccount,
			c.DateDiffDays,
		)
	}
	tw.Flush()
}

// =============================================================================
// Security Management Commands
// =============================================================================

// runListSecurities lists securities from the database.
func runListSecurities(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--list-securities requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	filter := security.Filter{}
	if !opts.includeHidden {
		excludeHidden := true
		filter.ExcludeHidden = &excludeHidden
	}
	if opts.acctType != "" {
		secType, err := security.ParseType(opts.acctType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		filter.SecurityType = &secType
	}
	if opts.secAssetClass != "" {
		ac, err := security.ParseAssetClass(opts.secAssetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		filter.AssetClass = &ac
	}

	securities, err := svc.Security.List(filter)
	if err != nil {
		return fmt.Errorf("failed to list securities: %w", err)
	}

	printSecuritiesTable(w, securities)

	return nil
}

// runSecurityDetail shows detailed information for a specific security.
func runSecurityDetail(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.securityTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.securityTicker)
	}

	printSecurityDetails(w, sec)

	return nil
}

// runAddSecurity creates a new security.
func runAddSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-security requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--add-security requires --ticker to specify a ticker symbol")
	}
	if opts.acctName == "" {
		return fmt.Errorf("--add-security requires --name to specify a security name")
	}
	if opts.acctType == "" {
		return fmt.Errorf("--add-security requires --type to specify a security type (stock, etf, mutual_fund, other)")
	}

	secType, err := security.ParseType(opts.acctType)
	if err != nil {
		return fmt.Errorf("invalid --type: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec := security.NewSecurity(opts.secTicker, opts.acctName, secType)

	if opts.secAssetClass != "" {
		ac, err := security.ParseAssetClass(opts.secAssetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		sec.AssetClass = ac
	}

	if opts.acctCurrency != "" {
		sec.Currency = opts.acctCurrency
	}

	if opts.secExchange != "" {
		sec.SetExchange(opts.secExchange)
	}

	if err := svc.Security.Create(sec); err != nil {
		return fmt.Errorf("failed to create security: %w", err)
	}

	fmt.Fprintln(w, "Security created successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)
	if sec.Exchange.Valid {
		fmt.Fprintf(w, "  Exchange:    %s\n", sec.Exchange.String)
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runEditSecurity edits an existing security.
func runEditSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--edit-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.editSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.editSecurity)
	}

	// Apply changes
	if opts.secTicker != "" {
		sec.Ticker = opts.secTicker
	}
	if opts.acctName != "" {
		sec.Name = opts.acctName
	}
	if opts.acctType != "" {
		secType, err := security.ParseType(opts.acctType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		sec.SecurityType = secType
	}
	if opts.secAssetClass != "" {
		ac, err := security.ParseAssetClass(opts.secAssetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		sec.AssetClass = ac
	}
	if opts.acctCurrency != "" {
		sec.Currency = opts.acctCurrency
	}
	if opts.secExchange != "" {
		sec.SetExchange(opts.secExchange)
	}

	if err := svc.Security.Update(sec); err != nil {
		return fmt.Errorf("failed to update security: %w", err)
	}

	fmt.Fprintln(w, "Security updated successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)

	autoBackupAfterModification(opts.file)
	return nil
}

// runHideSecurity hides a security.
func runHideSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--hide-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.hideSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.hideSecurity)
	}

	if err := svc.Security.Hide(sec.ID); err != nil {
		return fmt.Errorf("failed to hide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) hidden successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}

// runUnhideSecurity unhides a security.
func runUnhideSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--unhide-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.unhideSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.unhideSecurity)
	}

	if err := svc.Security.Unhide(sec.ID); err != nil {
		return fmt.Errorf("failed to unhide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) unhidden successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}

// runDeleteSecurity deletes a security.
func runDeleteSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--delete-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.deleteSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.deleteSecurity)
	}

	if err := svc.Security.Delete(sec.ID); err != nil {
		if depErr, ok := err.(*security.HasDependentsError); ok {
			return fmt.Errorf("%s\nUse --hide-security %s instead", depErr.Error(), sec.Ticker)
		}
		return fmt.Errorf("failed to delete security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) deleted successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}

// runListPrices lists prices for a security ticker.
func runListPrices(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--prices requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--prices requires --ticker to specify a security")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}

	var from, to *types.Date
	if opts.fromDate != "" {
		d, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		from = &d
	}
	if opts.toDate != "" {
		d, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		to = &d
	}

	prices, err := svc.Price.GetPriceHistory(sec.ID, from, to)
	if err != nil {
		return fmt.Errorf("failed to get prices: %w", err)
	}

	printPricesTable(w, sec.Ticker, prices)

	return nil
}

// runAddPrice adds a price for a security.
func runAddPrice(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-price requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--add-price requires --ticker to specify a security")
	}
	if opts.txDate == "" {
		return fmt.Errorf("--add-price requires --date to specify a price date")
	}
	if opts.priceValue == "" {
		return fmt.Errorf("--add-price requires --price to specify a price value")
	}

	priceDate, err := types.ParseDate(opts.txDate)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	priceAmount, err := types.NewMoney(opts.priceValue)
	if err != nil {
		return fmt.Errorf("invalid --price: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}

	p := price.NewPrice(sec.ID, priceDate, priceAmount, price.SourceManual)
	if err := svc.Price.AddPrice(p); err != nil {
		return fmt.Errorf("failed to add price: %w", err)
	}

	fmt.Fprintf(w, "Price added for %s on %s: %s\n", sec.Ticker, priceDate.String(), formatMoney(priceAmount, sec.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}

// runCurrentPrice shows the most recent price for a security.
func runCurrentPrice(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--current-price requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--current-price requires --ticker to specify a security")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}

	asOf := types.Today()
	p, err := svc.Price.GetCurrentPrice(sec.ID, asOf)
	if err != nil {
		return fmt.Errorf("no price found for %s", sec.Ticker)
	}

	fmt.Fprintf(w, "CURRENT PRICE: %s\n", sec.Ticker)
	fmt.Fprintf(w, "Ticker:  %s\n", sec.Ticker)
	fmt.Fprintf(w, "Name:    %s\n", sec.Name)
	fmt.Fprintf(w, "Date:    %s\n", p.Date.String())
	fmt.Fprintf(w, "Price:   %s\n", formatMoney(p.Price, sec.Currency))
	fmt.Fprintf(w, "Source:  %s\n", p.Source.DisplayName())

	return nil
}

// runImportPrices imports prices from a CSV file.
func runImportPrices(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--import-prices requires --file to specify a database")
	}

	file, err := os.Open(opts.importPrices)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()

	// Parse the CSV
	parseResult, err := imexport.ParsePriceCSV(file)
	if err != nil {
		return fmt.Errorf("failed to parse price CSV: %w", err)
	}

	// Report parse errors
	if parseResult.HasErrors() {
		for _, e := range parseResult.Errors {
			fmt.Fprintf(w, "  Warning: %s\n", e.Error())
		}
	}

	if len(parseResult.Records) == 0 {
		fmt.Fprintln(w, "No valid price records found in CSV file.")
		return nil
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Resolve tickers to security IDs and build price objects
	var prices []*price.Price
	tickerErrors := 0
	hiddenSkipped := 0
	for _, rec := range parseResult.Records {
		sec, secErr := svc.Security.GetByTicker(rec.Ticker, "")
		if secErr != nil {
			fmt.Fprintf(w, "  Warning: line %d: unknown ticker %q\n", rec.SourceLine, rec.Ticker)
			tickerErrors++
			continue
		}
		if sec.Hidden {
			fmt.Fprintf(w, "  Warning: line %d: skipping hidden security %q\n", rec.SourceLine, rec.Ticker)
			hiddenSkipped++
			continue
		}

		p := price.NewPrice(sec.ID, rec.Date, rec.Price, price.SourceImport)
		prices = append(prices, p)
	}

	if len(prices) == 0 {
		fmt.Fprintln(w, "No prices to import after resolving tickers.")
		return nil
	}

	// Import prices
	result, err := svc.Price.BulkImport(prices, opts.overwrite)
	if err != nil {
		return fmt.Errorf("failed to import prices: %w", err)
	}

	// Display summary
	fmt.Fprintf(w, "IMPORT COMPLETE: %s\n", filepath.Base(opts.importPrices))
	fmt.Fprintf(w, "  Total rows:     %d\n", result.Total+tickerErrors+len(parseResult.Errors))
	fmt.Fprintf(w, "  Imported:       %d\n", result.Imported)
	fmt.Fprintf(w, "  Skipped:        %d\n", result.Skipped)
	if tickerErrors > 0 {
		fmt.Fprintf(w, "  Unknown ticker: %d\n", tickerErrors)
	}
	if len(parseResult.Errors) > 0 {
		fmt.Fprintf(w, "  Parse errors:   %d\n", len(parseResult.Errors))
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runBuy executes the --buy command: buy shares of a security in an investment account.
func runBuy(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--buy requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--buy requires --account to specify an investment account")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--buy requires --ticker to specify a security")
	}
	if opts.shares == "" {
		return fmt.Errorf("--buy requires --shares to specify the number of shares")
	}
	if opts.txAmount == "" && opts.pricePerShare == "" {
		return fmt.Errorf("--buy requires --amount (total) and/or --price-per-share")
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

	txn, err := svc.Investment.Buy(acct.ID, sec.ID, date, shares, totalAmount, pricePerShare, commission, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create buy transaction: %w", err)
	}

	fmt.Fprintln(w, "Buy transaction created successfully!")
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
	fmt.Fprintf(w, "  Total:    %s\n", formatMoney(txn.TotalAmount.Neg(), acct.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}

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

// runInvestmentFee executes the --investment-fee command: record a fee in an investment account.
func runInvestmentFee(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--investment-fee requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--investment-fee requires --account to specify an investment account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--investment-fee requires --amount to specify the fee amount")
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

	_, err = svc.Investment.Fee(acct.ID, date, amount, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create fee transaction: %w", err)
	}

	fmt.Fprintln(w, "Investment fee transaction created successfully!")
	fmt.Fprintf(w, "  Account: %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:    %s\n", date.String())
	fmt.Fprintf(w, "  Amount:  %s\n", formatMoney(amount, acct.Currency))
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:    %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runInvestDeposit executes the --invest-deposit command: deposit cash into an investment account.
func runInvestDeposit(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--invest-deposit requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--invest-deposit requires --account to specify an investment account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--invest-deposit requires --amount to specify the deposit amount")
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

	_, err = svc.Investment.Deposit(acct.ID, date, amount, opts.txMemo)
	if err != nil {
		return fmt.Errorf("failed to create deposit transaction: %w", err)
	}

	fmt.Fprintln(w, "Investment deposit created successfully!")
	fmt.Fprintf(w, "  Account: %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:    %s\n", date.String())
	fmt.Fprintf(w, "  Amount:  %s\n", formatMoney(amount, acct.Currency))
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:    %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}

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

// runPortfolio executes the --portfolio command: show investment portfolio for an account.
func runPortfolio(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--portfolio requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--portfolio requires --account to specify an investment account")
	}

	// Parse optional as-of date (defaults to today)
	var asOf types.Date
	if opts.reportAsOf != "" {
		var err error
		asOf, err = types.ParseDate(opts.reportAsOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
	} else {
		asOf = types.Today()
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

	valuation, err := svc.Investment.GetAccountValuation(acct.ID, asOf)
	if err != nil {
		return fmt.Errorf("failed to get portfolio valuation: %w", err)
	}

	// Build security lookup for display
	securityMap := make(map[types.ID]*security.Security)
	for _, h := range valuation.Holdings {
		sec, secErr := svc.Security.GetByID(h.SecurityID)
		if secErr == nil {
			securityMap[h.SecurityID] = sec
		}
	}

	if opts.showLots && acct.TrackLots {
		printPortfolioWithLots(w, acct, valuation, securityMap, svc, asOf)
	} else {
		printPortfolioSummary(w, acct, valuation, securityMap)
	}

	return nil
}

// runSplit executes the --split command: apply a stock split or reverse split.
func runSplit(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--split requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--split requires --ticker to specify a security")
	}
	if opts.splitRatio == "" {
		return fmt.Errorf("--split requires --ratio to specify the split ratio (e.g. 4:1)")
	}

	params, err := investment.ParseSplitRatio(opts.splitRatio)
	if err != nil {
		return fmt.Errorf("invalid --ratio: %w", err)
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

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to apply corporate actions", opts.secTicker)
	}

	action, err := svc.CorporateAction.Split(sec.ID, date, *params)
	if err != nil {
		return fmt.Errorf("failed to apply stock split: %w", err)
	}

	fmt.Fprintln(w, "Stock split applied successfully!")
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Ratio:    %s\n", params.RatioString())
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}

// runMergeSecurity executes the --merge-security command: apply a merger/acquisition.
func runMergeSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--merge-security requires --file to specify a database")
	}
	if opts.mergeSource == "" {
		return fmt.Errorf("--merge-security requires --source to specify the source security ticker")
	}
	if opts.mergeTarget == "" {
		return fmt.Errorf("--merge-security requires --target to specify the target security ticker")
	}
	if opts.exchangeRatio == "" {
		return fmt.Errorf("--merge-security requires --exchange-ratio to specify the exchange ratio")
	}

	ratio, err := strconv.ParseFloat(opts.exchangeRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --exchange-ratio: %w", err)
	}

	var cashPerShare float64
	if opts.cashPerShare != "" {
		cashPerShare, err = strconv.ParseFloat(opts.cashPerShare, 64)
		if err != nil {
			return fmt.Errorf("invalid --cash-per-share: %w", err)
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

	params := investment.MergerParams{
		ExchangeRatio: ratio,
		CashPerShare:  cashPerShare,
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sourceSec, err := svc.Security.GetByTicker(opts.mergeSource, "")
	if err != nil {
		return fmt.Errorf("source security %q not found", opts.mergeSource)
	}
	if sourceSec.Hidden {
		return fmt.Errorf("source security %q is hidden; unhide it first to apply corporate actions", opts.mergeSource)
	}

	targetSec, err := svc.Security.GetByTicker(opts.mergeTarget, "")
	if err != nil {
		return fmt.Errorf("target security %q not found", opts.mergeTarget)
	}
	if targetSec.Hidden {
		return fmt.Errorf("target security %q is hidden; unhide it first to apply corporate actions", opts.mergeTarget)
	}

	action, err := svc.CorporateAction.Merger(sourceSec.ID, targetSec.ID, date, params)
	if err != nil {
		return fmt.Errorf("failed to apply merger: %w", err)
	}

	fmt.Fprintln(w, "Merger applied successfully!")
	fmt.Fprintf(w, "  Source:   %s (%s)\n", sourceSec.Ticker, sourceSec.Name)
	fmt.Fprintf(w, "  Target:   %s (%s)\n", targetSec.Ticker, targetSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Exchange Ratio: %s\n", opts.exchangeRatio)
	if params.HasCashConsideration() {
		fmt.Fprintf(w, "  Cash/Share: $%.2f\n", cashPerShare)
	}
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}

// runSpinOff executes the --spin-off command: apply a corporate spin-off.
func runSpinOff(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--spin-off requires --file to specify a database")
	}
	if opts.spinOffParent == "" {
		return fmt.Errorf("--spin-off requires --parent to specify the parent security ticker")
	}
	if opts.spinOffChild == "" {
		return fmt.Errorf("--spin-off requires --spinoff to specify the spin-off security ticker")
	}
	if opts.shareRatio == "" {
		return fmt.Errorf("--spin-off requires --share-ratio to specify the share ratio")
	}
	if opts.parentAllocation == "" {
		return fmt.Errorf("--spin-off requires --parent-allocation to specify the parent cost basis allocation percentage")
	}
	if opts.spinOffPrice == "" {
		return fmt.Errorf("--spin-off requires --spin-off-price to specify the spin-off security price")
	}

	shareRatio, err := strconv.ParseFloat(opts.shareRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --share-ratio: %w", err)
	}

	parentAlloc, err := strconv.ParseFloat(opts.parentAllocation, 64)
	if err != nil {
		return fmt.Errorf("invalid --parent-allocation: %w", err)
	}

	spinPrice, err := types.NewMoney(opts.spinOffPrice)
	if err != nil {
		return fmt.Errorf("invalid --spin-off-price: %w", err)
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

	params := investment.SpinOffParams{
		ShareRatio:          shareRatio,
		ParentAllocationPct: parentAlloc,
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	parentSec, err := svc.Security.GetByTicker(opts.spinOffParent, "")
	if err != nil {
		return fmt.Errorf("parent security %q not found", opts.spinOffParent)
	}
	if parentSec.Hidden {
		return fmt.Errorf("parent security %q is hidden; unhide it first to apply corporate actions", opts.spinOffParent)
	}

	childSec, err := svc.Security.GetByTicker(opts.spinOffChild, "")
	if err != nil {
		return fmt.Errorf("spin-off security %q not found", opts.spinOffChild)
	}
	if childSec.Hidden {
		return fmt.Errorf("spin-off security %q is hidden; unhide it first to apply corporate actions", opts.spinOffChild)
	}

	action, err := svc.CorporateAction.SpinOff(parentSec.ID, childSec.ID, date, params, spinPrice)
	if err != nil {
		return fmt.Errorf("failed to apply spin-off: %w", err)
	}

	fmt.Fprintln(w, "Spin-off applied successfully!")
	fmt.Fprintf(w, "  Parent:   %s (%s)\n", parentSec.Ticker, parentSec.Name)
	fmt.Fprintf(w, "  Spin-off: %s (%s)\n", childSec.Ticker, childSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Share Ratio: %s\n", opts.shareRatio)
	fmt.Fprintf(w, "  Parent Allocation: %s%%\n", opts.parentAllocation)
	fmt.Fprintf(w, "  Spin-off Price: %s\n", formatMoney(spinPrice, "USD"))
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}
