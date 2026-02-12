package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
)

// openServices opens the database and creates all services via the shared registry.
// It also does a best-effort update of the recent files in the config.
func openServices(file string) (*db.DB, *service.Services, error) {
	database, err := db.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Best-effort update recent files
	if cfg, err := config.Load(); err == nil {
		cfg.AddRecentFile(file)
		_ = cfg.Save()
	}

	return database, service.NewServices(database), nil
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
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Get balance
	balance, err := svc.Account.GetBalance(account.ID)
	if err != nil {
		return fmt.Errorf("failed to get balance: %w", err)
	}

	// Print account details
	printAccountDetails(w, account, balance)

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
	account, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Parse date filters if provided
	var startDate, endDate models.Date
	hasDateFilter := false

	if opts.fromDate != "" {
		startDate, err = models.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		hasDateFilter = true
	}

	if opts.toDate != "" {
		endDate, err = models.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		hasDateFilter = true
	}

	// Fetch transactions
	var transactions []*models.Transaction
	if hasDateFilter {
		// If only one date provided, use it for both bounds
		if opts.fromDate == "" {
			startDate = models.Date{} // Zero date (far past)
		}
		if opts.toDate == "" {
			endDate = models.Today() // Today
		}
		transactions, err = svc.Transaction.ListByAccountAndDateRange(account.ID, startDate, endDate)
	} else {
		transactions, err = svc.Transaction.ListByAccount(account.ID)
	}
	if err != nil {
		return fmt.Errorf("failed to list transactions: %w", err)
	}

	// Apply limit if specified
	if opts.limit > 0 && len(transactions) > opts.limit {
		transactions = transactions[:opts.limit]
	}

	// Build payee and category lookup maps
	payeeNames := make(map[models.ID]string)
	categoryNames := make(map[models.ID]string)

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
	printTransactionsTable(w, account, transactions, payeeNames, categoryNames)

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
	amount, err := models.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Parse date (default to today)
	var date models.Date
	if opts.txDate != "" {
		date, err = models.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = models.Today()
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

	// Handle payee (auto-create if needed)
	var payeeID models.NullableID
	var payeeName string
	var payeeCreated bool
	if opts.txPayee != "" {
		payee, created, err := svc.Payee.GetOrCreate(opts.txPayee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		payeeID = models.NullableID{Valid: true, ID: payee.ID}
		payeeName = payee.Name
		payeeCreated = created
	}

	// Handle category
	var categoryID models.NullableID
	var categoryName string
	if opts.txCategory != "" {
		// First try top-level category, then search all categories
		category, err := svc.CategoryRepo.GetByName(opts.txCategory, nil)
		if err != nil {
			// Try finding it as a subcategory (search all categories)
			categories, listErr := svc.CategoryRepo.List()
			if listErr != nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			var found *models.Category
			for _, c := range categories {
				if c.Name == opts.txCategory {
					found = c
					break
				}
			}
			if found == nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			category = found
		}
		categoryID = models.NullableID{Valid: true, ID: category.ID}
		categoryName = category.Name
	}

	// Create transaction
	txn := models.NewTransaction(account.ID, date, amount)
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
	fmt.Fprintf(w, "  Account:  %s\n", account.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", formatMoney(amount, account.Currency))
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
	amount, err := models.NewMoney(opts.txAmount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Amount must be positive for transfers
	if !amount.IsPositive() {
		return fmt.Errorf("--amount must be positive for transfers")
	}

	// Parse date (default to today)
	var date models.Date
	if opts.txDate != "" {
		date, err = models.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = models.Today()
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
		err = svc.Transaction.UpdateTransfer(pair.FromTransaction.TransferID.ID, date, amount, opts.txMemo, models.TransactionStatusPending)
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
	criteria := repository.TransactionSearchCriteria{
		PayeeName: opts.searchTerm,
		Memo:      opts.searchTerm,
	}

	// Parse date filters if provided
	if opts.fromDate != "" {
		startDate, err := models.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		criteria.StartDate = &startDate
	}

	if opts.toDate != "" {
		endDate, err := models.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		criteria.EndDate = &endDate
	}

	// Parse account filter if provided
	if opts.accountName != "" {
		account, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}
		criteria.AccountID = &account.ID
	}

	// Parse category filter if provided
	if opts.txCategory != "" {
		criteria.CategoryName = opts.txCategory
	}

	// Parse min/max amount filters if provided
	if opts.minAmount != "" {
		minAmt, err := models.NewMoney(opts.minAmount)
		if err != nil {
			return fmt.Errorf("invalid --min amount: %w", err)
		}
		criteria.MinAmount = &minAmt
	}

	if opts.maxAmount != "" {
		maxAmt, err := models.NewMoney(opts.maxAmount)
		if err != nil {
			return fmt.Errorf("invalid --max amount: %w", err)
		}
		criteria.MaxAmount = &maxAmt
	}

	// Search for transactions - we need to search by payee OR memo
	// Since the Search method uses AND logic, we'll do two searches and merge
	var transactions []*models.Transaction

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
	payeeNames := make(map[models.ID]string)
	categoryNames := make(map[models.ID]string)
	accountNames := make(map[models.ID]string)
	accountCurrencies := make(map[models.ID]string)

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
	acctType, err := models.ParseAccountType(opts.acctType)
	if err != nil {
		validTypes := []string{}
		for _, t := range models.AllAccountTypes() {
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
	openingBalance := models.MustNewMoney("0")
	if opts.acctOpeningBal != "" {
		openingBalance, err = models.NewMoney(opts.acctOpeningBal)
		if err != nil {
			return fmt.Errorf("invalid --opening-balance: %w", err)
		}
	}

	// Parse opening date (default to today)
	openingDate := models.Today()
	if opts.acctOpeningDate != "" {
		openingDate, err = models.ParseDate(opts.acctOpeningDate)
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
	account := models.NewAccount(opts.acctName, acctType, currency, openingBalance, openingDate)

	// Set optional fields
	if opts.acctInstitution != "" {
		account.SetInstitution(opts.acctInstitution)
	}
	if opts.acctNumber != "" {
		account.SetAccountNumber(opts.acctNumber)
	}
	if opts.acctNotes != "" {
		account.SetNotes(opts.acctNotes)
	}

	// Handle type-specific fields
	if opts.acctCreditLimit != "" {
		if acctType != models.AccountTypeCreditCard {
			return fmt.Errorf("--credit-limit is only valid for credit_card accounts")
		}
		creditLimit, err := models.NewMoney(opts.acctCreditLimit)
		if err != nil {
			return fmt.Errorf("invalid --credit-limit: %w", err)
		}
		account.SetCreditLimit(creditLimit)
	}

	if opts.acctInterestRate != "" {
		if acctType != models.AccountTypeLoan {
			return fmt.Errorf("--interest-rate is only valid for loan accounts")
		}
		interestRate, err := models.NewMoney(opts.acctInterestRate)
		if err != nil {
			return fmt.Errorf("invalid --interest-rate: %w", err)
		}
		account.SetInterestRate(interestRate)
	}

	// Save account
	if err := svc.Account.Create(account); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Account created successfully!")
	fmt.Fprintf(w, "  Name:            %s\n", account.Name)
	fmt.Fprintf(w, "  Type:            %s\n", account.Type.DisplayName())
	fmt.Fprintf(w, "  Currency:        %s\n", account.Currency)
	fmt.Fprintf(w, "  Opening Balance: %s\n", formatMoney(account.OpeningBalance, account.Currency))
	fmt.Fprintf(w, "  Opening Date:    %s\n", account.OpeningDate.String())
	if account.Institution.Valid {
		fmt.Fprintf(w, "  Institution:     %s\n", account.Institution.String)
	}
	if account.AccountNumber.Valid {
		fmt.Fprintf(w, "  Account Number:  %s\n", account.AccountNumber.String)
	}
	if account.CreditLimit.Valid {
		fmt.Fprintf(w, "  Credit Limit:    %s\n", formatMoney(account.CreditLimit.Money, account.Currency))
	}
	if account.InterestRate.Valid {
		fmt.Fprintf(w, "  Interest Rate:   %s%%\n", account.InterestRate.Money.String())
	}
	if account.Notes.Valid {
		fmt.Fprintf(w, "  Notes:           %s\n", account.Notes.String)
	}

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
	var scheduledTxns []*models.ScheduledTransaction
	if opts.scheduledDue {
		scheduledTxns, err = svc.ScheduledTxn.ListDue()
	} else {
		scheduledTxns, err = svc.ScheduledTxn.List()
	}
	if err != nil {
		return fmt.Errorf("failed to list scheduled transactions: %w", err)
	}

	// Filter by account if specified
	if opts.accountName != "" {
		account, err := svc.Account.GetByName(opts.accountName)
		if err != nil {
			return fmt.Errorf("account %q not found", opts.accountName)
		}

		var filtered []*models.ScheduledTransaction
		for _, st := range scheduledTxns {
			if st.AccountID == account.ID {
				filtered = append(filtered, st)
			}
		}
		scheduledTxns = filtered
	}

	// Build lookup maps
	payeeNames := make(map[models.ID]string)
	categoryNames := make(map[models.ID]string)
	accountNames := make(map[models.ID]string)
	accountCurrencies := make(map[models.ID]string)

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
	stID, err := models.ParseID(opts.postScheduled)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	// Get the scheduled transaction first to show details
	st, err := svc.ScheduledTxn.GetByID(stID)
	if err != nil {
		return fmt.Errorf("scheduled transaction not found: %w", err)
	}

	// Remember the old next date
	oldNextDate := st.NextDate

	// Parse optional amount
	var amount *models.Money
	if opts.txAmount != "" {
		amt, err := models.NewMoney(opts.txAmount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		amount = &amt
	}

	// Parse optional date
	var date *models.Date
	if opts.txDate != "" {
		d, err := models.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
		date = &d
	}

	// Post the scheduled transaction
	var txn *models.Transaction
	if date != nil {
		txn, err = svc.ScheduledTxn.PostWithDate(stID, *date, amount)
	} else {
		txn, err = svc.ScheduledTxn.Post(stID, amount)
	}
	if err != nil {
		return fmt.Errorf("failed to post scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := svc.ScheduledTxn.GetByID(stID)

	// Get account info for currency
	account, _ := svc.AccountRepo.GetByID(st.AccountID)
	currency := "USD"
	accountName := "Unknown"
	if account != nil {
		currency = account.Currency
		accountName = account.Name
	}

	// Get payee name
	payeeName := "-"
	if st.HasPayee() {
		payee, err := svc.PayeeRepo.GetByID(st.PayeeID.ID)
		if err == nil {
			payeeName = payee.Name
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
	stID, err := models.ParseID(opts.skipScheduled)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	// Get the scheduled transaction first to show details
	st, err := svc.ScheduledTxn.GetByID(stID)
	if err != nil {
		return fmt.Errorf("scheduled transaction not found: %w", err)
	}

	// Remember the old next date
	oldNextDate := st.NextDate

	// Skip the scheduled transaction
	err = svc.ScheduledTxn.Skip(stID)
	if err != nil {
		return fmt.Errorf("failed to skip scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := svc.ScheduledTxn.GetByID(stID)

	// Get account info
	account, _ := svc.AccountRepo.GetByID(st.AccountID)
	accountName := "Unknown"
	if account != nil {
		accountName = account.Name
	}

	// Get payee name
	payeeName := "-"
	if st.HasPayee() {
		payee, err := svc.PayeeRepo.GetByID(st.PayeeID.ID)
		if err == nil {
			payeeName = payee.Name
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
		d, err := models.ParseDate(opts.reportAsOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
		asOf = time.Time(d)
	} else {
		asOf = time.Now()
	}

	// Generate report
	var report *models.NetWorthReport
	if opts.includeClosed {
		report, err = svc.Report.NetWorthAsOfIncludingClosed(asOf)
	} else {
		report, err = svc.Report.NetWorthAsOf(asOf)
	}
	if err != nil {
		return fmt.Errorf("failed to generate net worth report: %w", err)
	}

	// Print report
	printNetWorthReport(w, report)
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
	var report *models.SpendingReport

	if opts.reportMonth != "" {
		// Parse YYYY-MM format
		year, month, err := parseYearMonth(opts.reportMonth)
		if err != nil {
			return fmt.Errorf("invalid --month format: %w", err)
		}
		report, err = svc.Report.SpendingByCategoryMonth(year, month)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	} else if opts.reportYear != 0 {
		report, err = svc.Report.SpendingByCategoryYear(opts.reportYear)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	} else if opts.fromDate != "" {
		// Custom date range
		startDate, err := models.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}

		var endDate models.Date
		if opts.toDate != "" {
			endDate, err = models.ParseDate(opts.toDate)
			if err != nil {
				return fmt.Errorf("invalid --to date: %w", err)
			}
		} else {
			endDate = models.Today()
		}

		report, err = svc.Report.SpendingByCategoryDateRange(time.Time(startDate), time.Time(endDate))
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	}

	// Print report
	printSpendingReport(w, report)
	return nil
}
