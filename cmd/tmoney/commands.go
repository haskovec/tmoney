package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
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

// runCreateDB creates a new database file.
func runCreateDB(opts *cliOptions, w io.Writer) error {
	database, err := db.Create(opts.createDB)
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}
	defer database.Close()

	fmt.Fprintf(w, "Created database: %s\n", database.Path())
	return nil
}

// runListAccounts lists accounts from the database.
func runListAccounts(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--list-accounts requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// List accounts (activeOnly = !includeClosed)
	accounts, err := svc.Account.List(!opts.includeClosed)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get all balances
	balances, err := svc.Account.GetAllBalances()
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}

	// Print accounts table
	printAccountsTable(w, accounts, balances)

	return nil
}

// runAccountDetails shows detailed information for a specific account.
func runAccountDetails(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--account requires --file to specify a database")
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

	// Get balance
	bal, err := svc.Account.GetBalance(acct.ID)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	// Print account details
	printAccountDetails(w, acct, bal)

	return nil
}

// runBalance shows balances for all accounts with net worth.
func runBalance(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--balance requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// List accounts (active only)
	accounts, err := svc.Account.List(true)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get all balances
	balances, err := svc.Account.GetAllBalances()
	if err != nil {
		return fmt.Errorf("failed to get balances: %w", err)
	}

	// Print balances table
	printBalancesTable(w, accounts, balances)

	return nil
}

// runTransactions lists transactions for an account.
func runTransactions(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--transactions requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--transactions requires --account to specify an account")
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

	// Parse date filters if provided
	var startDate, endDate types.Date
	hasDateFilter := false

	if opts.fromDate != "" {
		startDate, err = types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		hasDateFilter = true
	}

	if opts.toDate != "" {
		endDate, err = types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		hasDateFilter = true
	}

	// Fetch transactions
	var transactions []*transaction.Transaction
	if hasDateFilter {
		// If only one date provided, use it for both bounds
		if opts.fromDate == "" {
			startDate = types.Date{} // Zero date (far past)
		}
		if opts.toDate == "" {
			endDate = types.Today() // Today
		}
		transactions, err = svc.Transaction.ListByAccountAndDateRange(acct.ID, startDate, endDate)
	} else {
		transactions, err = svc.Transaction.ListByAccount(acct.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to list transactions: %w", err)
	}

	// Filter by status if specified
	if opts.txStatus != "" {
		status, err := transaction.ParseStatus(opts.txStatus)
		if err != nil {
			return fmt.Errorf("invalid --status: %w", err)
		}
		var filtered []*transaction.Transaction
		for _, txn := range transactions {
			if txn.Status == status {
				filtered = append(filtered, txn)
			}
		}
		transactions = filtered
	}

	// Apply limit if specified
	if opts.limit > 0 && len(transactions) > opts.limit {
		transactions = transactions[:opts.limit]
	}

	// Build payee and category lookup maps
	payeeNames := make(map[types.ID]string)
	categoryNames := make(map[types.ID]string)

	// Fetch all payees and categories for name lookup
	payees, _ := svc.PayeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := svc.CategoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	// Print transactions table
	printTransactionsTable(w, acct, transactions, payeeNames, categoryNames)

	return nil
}

// runAddTransaction creates a new transaction.
func runAddTransaction(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-transaction requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--add-transaction requires --account to specify an account")
	}
	if opts.txAmount == "" {
		return fmt.Errorf("--add-transaction requires --amount to specify a value")
	}

	// Parse amount
	amount, err := types.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
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

	// Get account by name
	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Handle payee (auto-create if needed)
	var payeeID types.NullableID
	var payeeName string
	var payeeCreated bool
	if opts.txPayee != "" {
		py, created, err := svc.Payee.GetOrCreate(opts.txPayee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		payeeID = types.NullableID{Valid: true, ID: py.ID}
		payeeName = py.Name
		payeeCreated = created
	}

	// Handle category
	var categoryID types.NullableID
	var categoryName string
	if opts.txCategory != "" {
		// First try top-level category, then search all categories
		cat, err := svc.CategoryRepo.GetByName(opts.txCategory, nil)
		if err != nil {
			// Try finding it as a subcategory (search all categories)
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
		categoryID = types.NullableID{Valid: true, ID: cat.ID}
		categoryName = cat.Name
	}

	// Create transaction
	txn := transaction.NewTransaction(acct.ID, date, amount)
	if payeeID.Valid {
		txn.SetPayee(payeeID.ID)
	}
	if categoryID.Valid {
		txn.SetCategory(categoryID.ID)
	}
	if opts.txMemo != "" {
		txn.SetMemo(opts.txMemo)
	}

	// Save transaction
	if err := svc.Transaction.Create(txn); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Transaction created successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", acct.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", formatMoney(amount, acct.Currency))
	if payeeName != "" {
		if payeeCreated {
			fmt.Fprintf(w, "  Payee:    %s (new)\n", payeeName)
		} else {
			fmt.Fprintf(w, "  Payee:    %s\n", payeeName)
		}
	}
	if categoryName != "" {
		fmt.Fprintf(w, "  Category: %s\n", categoryName)
	}
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:     %s\n", opts.txMemo)
	}

	autoBackupAfterModification(opts.file)
	return nil
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

// runVoidTransaction voids a transaction by ID.
func runVoidTransaction(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--void requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Parse the transaction ID
	txnID, err := types.ParseID(opts.voidTxn)
	if err != nil {
		return fmt.Errorf("invalid transaction ID: %w", err)
	}

	// Get the transaction first to show details
	txn, err := svc.TransactionRepo.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("transaction not found: %w", err)
	}

	// Get account info for display
	acct, _ := svc.AccountRepo.GetByID(txn.AccountID)
	accountName := "Unknown"
	currency := "USD"
	if acct != nil {
		accountName = acct.Name
		currency = acct.Currency
	}

	// Remember original amount for confirmation display
	originalAmount := txn.Amount

	// Void the transaction
	if err := svc.Transaction.VoidTransaction(txnID); err != nil {
		return fmt.Errorf("failed to void transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Transaction voided successfully!")
	fmt.Fprintf(w, "  Account:  %s\n", accountName)
	fmt.Fprintf(w, "  Date:     %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Amount:   %s (was %s)\n", formatMoney(types.ZeroMoney, currency), formatMoney(originalAmount, currency))
	fmt.Fprintf(w, "  Status:   Void\n")
	if txn.IsTransfer() {
		fmt.Fprintln(w, "  Note:     Transfer counterpart was also voided")
	}

	autoBackupAfterModification(opts.file)
	return nil
}

// runSearch searches for transactions matching the search term and filters.
func runSearch(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--search requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Build search criteria
	criteria := transaction.SearchCriteria{
		PayeeName: opts.searchTerm,
		Memo:      opts.searchTerm,
	}

	// Parse date filters if provided
	if opts.fromDate != "" {
		startDate, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		criteria.StartDate = &startDate
	}

	if opts.toDate != "" {
		endDate, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		criteria.EndDate = &endDate
	}

	// Parse account filter if provided
	if opts.accountName != "" {
		acct, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}
		criteria.AccountID = &acct.ID
	}

	// Parse category filter if provided
	if opts.txCategory != "" {
		criteria.CategoryName = opts.txCategory
	}

	// Parse min/max amount filters if provided
	if opts.minAmount != "" {
		minAmt, err := types.NewMoney(opts.minAmount)
		if err != nil {
			return fmt.Errorf("invalid --min amount: %w", err)
		}
		criteria.MinAmount = &minAmt
	}

	if opts.maxAmount != "" {
		maxAmt, err := types.NewMoney(opts.maxAmount)
		if err != nil {
			return fmt.Errorf("invalid --max amount: %w", err)
		}
		criteria.MaxAmount = &maxAmt
	}

	// Search for transactions - we need to search by payee OR memo
	// Since the Search method uses AND logic, we'll do two searches and merge
	var transactions []*transaction.Transaction

	// Search by payee name
	payeeCriteria := criteria
	payeeCriteria.Memo = ""
	payeeResults, err := svc.TransactionRepo.Search(payeeCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	// Search by memo
	memoCriteria := criteria
	memoCriteria.PayeeName = ""
	memoResults, err := svc.TransactionRepo.Search(memoCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	// Merge results, avoiding duplicates
	seen := make(map[string]bool)
	for _, txn := range payeeResults {
		if !seen[txn.ID.String()] {
			seen[txn.ID.String()] = true
			transactions = append(transactions, txn)
		}
	}
	for _, txn := range memoResults {
		if !seen[txn.ID.String()] {
			seen[txn.ID.String()] = true
			transactions = append(transactions, txn)
		}
	}

	// Build lookup maps for display
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

	// Print search results
	printSearchResults(w, opts.searchTerm, transactions, accountNames, accountCurrencies, payeeNames, categoryNames)

	return nil
}

// runAddAccount creates a new account.
func runAddAccount(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-account requires --file to specify a database")
	}
	if opts.acctName == "" {
		return fmt.Errorf("--add-account requires --name to specify an account name")
	}
	if opts.acctType == "" {
		return fmt.Errorf("--add-account requires --type to specify an account type")
	}

	// Parse account type
	acctType, err := account.ParseType(opts.acctType)
	if err != nil {
		validTypes := []string{}
		for _, t := range account.AllTypes() {
			validTypes = append(validTypes, string(t))
		}
		return fmt.Errorf("invalid --type %q: valid types are %s", opts.acctType, strings.Join(validTypes, ", "))
	}

	// Parse currency (default to USD)
	currency := "USD"
	if opts.acctCurrency != "" {
		currency = strings.ToUpper(opts.acctCurrency)
	}

	// Parse opening balance (default to 0)
	openingBalance := types.MustNewMoney("0")
	if opts.acctOpeningBal != "" {
		openingBalance, err = types.NewMoney(opts.acctOpeningBal)
		if err != nil {
			return fmt.Errorf("invalid --opening-balance: %w", err)
		}
	}

	// Parse opening date (default to today)
	openingDate := types.Today()
	if opts.acctOpeningDate != "" {
		openingDate, err = types.ParseDate(opts.acctOpeningDate)
		if err != nil {
			return fmt.Errorf("invalid --opening-date: %w", err)
		}
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Check if account name already exists
	if _, err := svc.Account.GetByName(opts.acctName); err == nil {
		return fmt.Errorf("account %q already exists", opts.acctName)
	}

	// Create account
	acct := account.NewAccount(opts.acctName, acctType, currency, openingBalance, openingDate)

	// Set optional fields
	if opts.acctInstitution != "" {
		acct.SetInstitution(opts.acctInstitution)
	}
	if opts.acctNumber != "" {
		acct.SetAccountNumber(opts.acctNumber)
	}
	if opts.acctNotes != "" {
		acct.SetNotes(opts.acctNotes)
	}

	// Handle type-specific fields
	if opts.acctCreditLimit != "" {
		if acctType != account.TypeCreditCard {
			return fmt.Errorf("--credit-limit is only valid for credit_card accounts")
		}
		creditLimit, err := types.NewMoney(opts.acctCreditLimit)
		if err != nil {
			return fmt.Errorf("invalid --credit-limit: %w", err)
		}
		acct.SetCreditLimit(creditLimit)
	}

	if opts.acctInterestRate != "" {
		if acctType != account.TypeLoan {
			return fmt.Errorf("--interest-rate is only valid for loan accounts")
		}
		interestRate, err := types.NewMoney(opts.acctInterestRate)
		if err != nil {
			return fmt.Errorf("invalid --interest-rate: %w", err)
		}
		acct.SetInterestRate(interestRate)
	}

	// Save account
	if err := svc.Account.Create(acct); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Account created successfully!")
	fmt.Fprintf(w, "  Name:            %s\n", acct.Name)
	fmt.Fprintf(w, "  Type:            %s\n", acct.Type.DisplayName())
	fmt.Fprintf(w, "  Currency:        %s\n", acct.Currency)
	fmt.Fprintf(w, "  Opening Balance: %s\n", formatMoney(acct.OpeningBalance, acct.Currency))
	fmt.Fprintf(w, "  Opening Date:    %s\n", acct.OpeningDate.String())
	if acct.Institution.Valid {
		fmt.Fprintf(w, "  Institution:     %s\n", acct.Institution.String)
	}
	if acct.AccountNumber.Valid {
		fmt.Fprintf(w, "  Account Number:  %s\n", acct.AccountNumber.String)
	}
	if acct.CreditLimit.Valid {
		fmt.Fprintf(w, "  Credit Limit:    %s\n", formatMoney(acct.CreditLimit.Money, acct.Currency))
	}
	if acct.InterestRate.Valid {
		fmt.Fprintf(w, "  Interest Rate:   %s%%\n", acct.InterestRate.Money.String())
	}
	if acct.Notes.Valid {
		fmt.Fprintf(w, "  Notes:           %s\n", acct.Notes.String)
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

// runBackup creates a manual backup of the database file.
func runBackup(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--backup requires --file to specify a database")
	}

	backupPath, err := backup.CreateManualBackup(opts.file)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Fprintf(w, "Backup created: %s\n", backupPath)
	return nil
}

// runListBackups lists available backups for the database file.
func runListBackups(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--list-backups requires --file to specify a database")
	}

	backups, err := backup.ListBackups(opts.file)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	dbBase := filepath.Base(opts.file)
	fmt.Fprintf(w, "BACKUPS: %s\n", dbBase)
	fmt.Fprintln(w, strings.Repeat("=", len("BACKUPS: ")+len(dbBase)))

	if len(backups) == 0 {
		fmt.Fprintln(w, "No backups found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 4, ' ', 0)
	fmt.Fprintln(tw, "Date\tSize\tType")
	fmt.Fprintln(tw, "----\t----\t----")

	for _, b := range backups {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			b.Timestamp.Format("2006-01-02 15:04:05"),
			backup.FormatSize(b.Size),
			b.Type,
		)
	}

	tw.Flush()
	fmt.Fprintf(w, "\n%d backup(s) found\n", len(backups))

	return nil
}

// runRestore restores the database from a backup file.
func runRestore(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--restore requires --file to specify a database")
	}

	fmt.Fprintln(w, "Creating backup of current state...")

	safetyPath, err := backup.Restore(opts.file, opts.restore)
	if safetyPath != "" {
		fmt.Fprintf(w, "Backup created: %s\n", safetyPath)
	}
	if err != nil {
		return fmt.Errorf("failed to restore: %w", err)
	}

	fmt.Fprintf(w, "\nRestoring from: %s\n", opts.restore)
	fmt.Fprintln(w, "Restore complete.")

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
	backup.CreateAutoBackup(dbPath)
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
		&cliCategoryResolver{categorySvc: svc.Category},
		&cliPayeeResolver{payeeSvc: svc.Payee},
		&cliTransactionStore{
			transactionRepo: svc.TransactionRepo,
			payeeRepo:       svc.PayeeRepo,
		},
		&cliTransactionCreator{transactionSvc: svc.Transaction},
	)

	// Run preview
	importOpts := imexport.ImportOptions{
		Format:            format,
		DuplicateHandling: dupHandling,
	}
	result, err := importSvc.Preview(file, format, account.ID, importOpts)
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
		outFile.Close()
		os.Remove(opts.exportFile)
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

// --- Import adapter types ---
// These adapt the existing service/repository types to the interfaces expected
// by imexport.ImportService.

// cliCategoryResolver adapts CategoryService to imexport.CategoryResolver.
type cliCategoryResolver struct {
	categorySvc *category.Service
}

func (r *cliCategoryResolver) ResolveCategoryByName(name string) (types.ID, error) {
	// Handle hierarchical names like "Food:Groceries"
	parts := strings.SplitN(name, ":", 2)

	if len(parts) == 1 {
		// Top-level category
		cat, err := r.categorySvc.GetByName(name, nil)
		if err != nil {
			return types.ID{}, err
		}
		return cat.ID, nil
	}

	// Find parent first, then child
	parent, err := r.categorySvc.GetByName(parts[0], nil)
	if err != nil {
		return types.ID{}, fmt.Errorf("parent category %q not found: %w", parts[0], err)
	}

	child, err := r.categorySvc.GetByName(parts[1], &parent.ID)
	if err != nil {
		return types.ID{}, fmt.Errorf("subcategory %q not found under %q: %w", parts[1], parts[0], err)
	}

	return child.ID, nil
}

// cliPayeeResolver adapts PayeeService to imexport.PayeeResolver.
type cliPayeeResolver struct {
	payeeSvc *payee.Service
}

func (r *cliPayeeResolver) ResolvePayee(name string) (types.ID, types.NullableID, error) {
	payee, created, err := r.payeeSvc.ResolveOrCreate(name)
	if err != nil {
		return types.ID{}, types.NullableID{}, err
	}
	_ = created

	if payee == nil {
		return types.ID{}, types.NullableID{}, nil
	}

	var defaultCatID types.NullableID
	if payee.DefaultCategoryID.Valid {
		defaultCatID = payee.DefaultCategoryID
	}

	return payee.ID, defaultCatID, nil
}

// cliTransactionStore adapts repositories to imexport.TransactionStore.
type cliTransactionStore struct {
	transactionRepo *transaction.Repository
	payeeRepo       *payee.Repository
}

func (s *cliTransactionStore) ListByAccount(accountID types.ID) ([]*transaction.Transaction, error) {
	return s.transactionRepo.ListByAccount(accountID)
}

func (s *cliTransactionStore) GetPayeeName(payeeID types.ID) string {
	if payeeID.IsNil() {
		return ""
	}
	payee, err := s.payeeRepo.GetByID(payeeID)
	if err != nil {
		return ""
	}
	return payee.Name
}

func (s *cliTransactionStore) GetBankReferenceID(txn *transaction.Transaction) string {
	if txn.HasBankReferenceID() {
		return txn.BankReferenceID.String
	}
	return ""
}

// cliTransactionCreator adapts TransactionService to imexport.TransactionCreator.
type cliTransactionCreator struct {
	transactionSvc *transaction.Service
}

func (c *cliTransactionCreator) CreateTransaction(txn *transaction.Transaction) error {
	return c.transactionSvc.Create(txn)
}

func (c *cliTransactionCreator) CreateTransactionWithSplits(txn *transaction.Transaction, splits []*transaction.Split) error {
	return c.transactionSvc.CreateWithSplits(txn, splits)
}

func (c *cliTransactionCreator) UpdateTransaction(txn *transaction.Transaction) error {
	return c.transactionSvc.Update(txn)
}
