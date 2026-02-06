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

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
	"github.com/haskovec/tmoney/internal/tui"
)

// Version information - will be set via build flags in production
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// CLI option flags
type cliOptions struct {
	file          string
	listAccounts  bool
	includeClosed bool
	createDB      string
	accountName   string // --account <name> to show details
	showBalance   bool   // --balance to show all balances
	transactions  bool   // --transactions to list transactions
	limit         int    // --limit <n> to limit results
	fromDate      string // --from <YYYY-MM-DD> start date filter
	toDate        string // --to <YYYY-MM-DD> end date filter

	// Add transaction options
	addTransaction bool   // --add-transaction flag
	txAmount       string // --amount <value>
	txPayee        string // --payee <name>
	txCategory     string // --category <name>
	txDate         string // --date <YYYY-MM-DD>
	txMemo         string // --memo <text>

	// Transfer options
	transfer    bool   // --transfer flag
	fromAccount string // --from <account> for transfers
	toAccount   string // --to <account> for transfers

	// Search options
	searchTerm string // --search <term>
	minAmount  string // --min <amount>
	maxAmount  string // --max <amount>

	// Scheduled transaction options
	scheduled     bool   // --scheduled flag
	scheduledDue  bool   // --due flag (with --scheduled)
	postScheduled string // --post-scheduled <id>
	skipScheduled string // --skip-scheduled <id>

	// Add account options
	addAccount        bool   // --add-account flag
	acctName          string // --name <name>
	acctType          string // --type <type>
	acctCurrency      string // --currency <code>
	acctOpeningBal    string // --opening-balance <value>
	acctOpeningDate   string // --opening-date <YYYY-MM-DD>
	acctInstitution   string // --institution <name>
	acctNumber        string // --account-number <number>
	acctNotes         string // --notes <text>
	acctCreditLimit   string // --credit-limit <value>
	acctInterestRate  string // --interest-rate <value>

	// Report options
	report       bool   // --report flag
	reportType   string // net-worth or spending
	reportMonth  string // --month YYYY-MM for spending
	reportYear   int    // --year YYYY for spending
	reportAsOf   string // --as-of YYYY-MM-DD for net-worth
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, remaining, err := parseArgs(args)
	if err != nil {
		return err
	}

	// Handle --create
	if opts.createDB != "" {
		return runCreateDB(opts, stdout)
	}

	// Handle --list-accounts
	if opts.listAccounts {
		return runListAccounts(opts, stdout)
	}

	// Handle --add-account
	if opts.addAccount {
		return runAddAccount(opts, stdout)
	}

	// Handle --add-transaction
	if opts.addTransaction {
		return runAddTransaction(opts, stdout)
	}

	// Handle --transfer
	if opts.transfer {
		return runTransfer(opts, stdout)
	}

	// Handle --post-scheduled
	if opts.postScheduled != "" {
		return runPostScheduled(opts, stdout)
	}

	// Handle --skip-scheduled
	if opts.skipScheduled != "" {
		return runSkipScheduled(opts, stdout)
	}

	// Handle --scheduled
	if opts.scheduled {
		return runScheduled(opts, stdout)
	}

	// Handle --report
	if opts.report {
		return runReport(opts, stdout)
	}

	// Handle --search
	if opts.searchTerm != "" {
		return runSearch(opts, stdout)
	}

	// Handle --transactions (check before --account since it uses --account as argument)
	if opts.transactions {
		return runTransactions(opts, stdout)
	}

	// Handle --account <name>
	if opts.accountName != "" {
		return runAccountDetails(opts, stdout)
	}

	// Handle --balance
	if opts.showBalance {
		return runBalance(opts, stdout)
	}

	// If remaining args include a file path, use it as the file
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		if opts.file == "" {
			opts.file = remaining[0]
		}
	}

	// Default to TUI mode
	return runTUI(opts)
}

// runTUI launches the interactive TUI mode.
func runTUI(opts *cliOptions) error {
	// If no file specified, use default location
	if opts.file == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		opts.file = filepath.Join(homeDir, "Documents", "TMoney", "default.tdb")
	}

	// Check if file exists, if not create it
	if _, err := os.Stat(opts.file); os.IsNotExist(err) {
		// Create the directory if needed
		dir := filepath.Dir(opts.file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Create new database
		database, err := db.Create(opts.file)
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
		defer database.Close()

		// Run TUI
		return tui.Run(database)
	}

	// Open existing database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Run TUI
	return tui.Run(database)
}

// parseArgs parses command-line arguments and returns options and remaining args.
func parseArgs(args []string) (*cliOptions, []string, error) {
	opts := &cliOptions{}
	var remaining []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch arg {
		case "-v", "--version":
			printVersion(os.Stdout)
			os.Exit(0)
		case "-h", "--help":
			printHelp(os.Stdout)
			os.Exit(0)
		case "-f", "--file":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--file requires a path argument")
			}
			i++
			opts.file = args[i]
		case "--list-accounts":
			opts.listAccounts = true
		case "--include-closed":
			opts.includeClosed = true
		case "--account":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--account requires an account name argument")
			}
			i++
			opts.accountName = args[i]
		case "--balance":
			opts.showBalance = true
		case "--transactions":
			opts.transactions = true
		case "--limit":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--limit requires a number argument")
			}
			i++
			limit, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, nil, fmt.Errorf("--limit requires a valid number: %w", err)
			}
			if limit < 1 {
				return nil, nil, fmt.Errorf("--limit must be a positive number")
			}
			opts.limit = limit
		case "--from":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--from requires an argument")
			}
			i++
			opts.fromDate = args[i]
			opts.fromAccount = args[i] // Also used for transfer source account
		case "--to":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--to requires an argument")
			}
			i++
			opts.toDate = args[i]
			opts.toAccount = args[i] // Also used for transfer destination account
		case "--create":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--create requires a path argument")
			}
			i++
			opts.createDB = args[i]
		case "--add-transaction":
			opts.addTransaction = true
		case "--transfer":
			opts.transfer = true
		case "--search":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--search requires a search term argument")
			}
			i++
			opts.searchTerm = args[i]
		case "--min":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--min requires an amount argument")
			}
			i++
			opts.minAmount = args[i]
		case "--max":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--max requires an amount argument")
			}
			i++
			opts.maxAmount = args[i]
		case "--scheduled":
			opts.scheduled = true
		case "--due":
			opts.scheduledDue = true
		case "--post-scheduled":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--post-scheduled requires a scheduled transaction ID argument")
			}
			i++
			opts.postScheduled = args[i]
		case "--skip-scheduled":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--skip-scheduled requires a scheduled transaction ID argument")
			}
			i++
			opts.skipScheduled = args[i]
		case "--amount":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--amount requires a value argument")
			}
			i++
			opts.txAmount = args[i]
		case "--payee":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--payee requires a name argument")
			}
			i++
			opts.txPayee = args[i]
		case "--category":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--category requires a name argument")
			}
			i++
			opts.txCategory = args[i]
		case "--date":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--date requires a date argument (YYYY-MM-DD)")
			}
			i++
			opts.txDate = args[i]
		case "--memo":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--memo requires a text argument")
			}
			i++
			opts.txMemo = args[i]
		case "--add-account":
			opts.addAccount = true
		case "--name":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--name requires a value argument")
			}
			i++
			opts.acctName = args[i]
		case "--type":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--type requires a value argument")
			}
			i++
			opts.acctType = args[i]
		case "--currency":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--currency requires a value argument")
			}
			i++
			opts.acctCurrency = args[i]
		case "--opening-balance":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--opening-balance requires a value argument")
			}
			i++
			opts.acctOpeningBal = args[i]
		case "--opening-date":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--opening-date requires a date argument (YYYY-MM-DD)")
			}
			i++
			opts.acctOpeningDate = args[i]
		case "--institution":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--institution requires a value argument")
			}
			i++
			opts.acctInstitution = args[i]
		case "--account-number":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--account-number requires a value argument")
			}
			i++
			opts.acctNumber = args[i]
		case "--notes":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--notes requires a text argument")
			}
			i++
			opts.acctNotes = args[i]
		case "--credit-limit":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--credit-limit requires a value argument")
			}
			i++
			opts.acctCreditLimit = args[i]
		case "--interest-rate":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--interest-rate requires a value argument")
			}
			i++
			opts.acctInterestRate = args[i]
		case "--report":
			opts.report = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.reportType = args[i]
			}
		case "--month":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--month requires a YYYY-MM argument")
			}
			i++
			opts.reportMonth = args[i]
		case "--year":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--year requires a YYYY argument")
			}
			i++
			year, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, nil, fmt.Errorf("--year requires a valid year: %w", err)
			}
			opts.reportYear = year
		case "--as-of":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--as-of requires a YYYY-MM-DD argument")
			}
			i++
			opts.reportAsOf = args[i]
		default:
			// Check for --flag=value formats
			if strings.HasPrefix(arg, "--file=") {
				opts.file = strings.TrimPrefix(arg, "--file=")
			} else if strings.HasPrefix(arg, "-f=") {
				opts.file = strings.TrimPrefix(arg, "-f=")
			} else if strings.HasPrefix(arg, "--create=") {
				opts.createDB = strings.TrimPrefix(arg, "--create=")
			} else if strings.HasPrefix(arg, "--account=") {
				opts.accountName = strings.TrimPrefix(arg, "--account=")
			} else if strings.HasPrefix(arg, "--limit=") {
				limitStr := strings.TrimPrefix(arg, "--limit=")
				limit, err := strconv.Atoi(limitStr)
				if err != nil {
					return nil, nil, fmt.Errorf("--limit requires a valid number: %w", err)
				}
				if limit < 1 {
					return nil, nil, fmt.Errorf("--limit must be a positive number")
				}
				opts.limit = limit
			} else if strings.HasPrefix(arg, "--from=") {
				val := strings.TrimPrefix(arg, "--from=")
				opts.fromDate = val
				opts.fromAccount = val
			} else if strings.HasPrefix(arg, "--to=") {
				val := strings.TrimPrefix(arg, "--to=")
				opts.toDate = val
				opts.toAccount = val
			} else if strings.HasPrefix(arg, "--amount=") {
				opts.txAmount = strings.TrimPrefix(arg, "--amount=")
			} else if strings.HasPrefix(arg, "--payee=") {
				opts.txPayee = strings.TrimPrefix(arg, "--payee=")
			} else if strings.HasPrefix(arg, "--category=") {
				opts.txCategory = strings.TrimPrefix(arg, "--category=")
			} else if strings.HasPrefix(arg, "--date=") {
				opts.txDate = strings.TrimPrefix(arg, "--date=")
			} else if strings.HasPrefix(arg, "--memo=") {
				opts.txMemo = strings.TrimPrefix(arg, "--memo=")
			} else if strings.HasPrefix(arg, "--name=") {
				opts.acctName = strings.TrimPrefix(arg, "--name=")
			} else if strings.HasPrefix(arg, "--type=") {
				opts.acctType = strings.TrimPrefix(arg, "--type=")
			} else if strings.HasPrefix(arg, "--currency=") {
				opts.acctCurrency = strings.TrimPrefix(arg, "--currency=")
			} else if strings.HasPrefix(arg, "--opening-balance=") {
				opts.acctOpeningBal = strings.TrimPrefix(arg, "--opening-balance=")
			} else if strings.HasPrefix(arg, "--opening-date=") {
				opts.acctOpeningDate = strings.TrimPrefix(arg, "--opening-date=")
			} else if strings.HasPrefix(arg, "--institution=") {
				opts.acctInstitution = strings.TrimPrefix(arg, "--institution=")
			} else if strings.HasPrefix(arg, "--account-number=") {
				opts.acctNumber = strings.TrimPrefix(arg, "--account-number=")
			} else if strings.HasPrefix(arg, "--notes=") {
				opts.acctNotes = strings.TrimPrefix(arg, "--notes=")
			} else if strings.HasPrefix(arg, "--credit-limit=") {
				opts.acctCreditLimit = strings.TrimPrefix(arg, "--credit-limit=")
			} else if strings.HasPrefix(arg, "--interest-rate=") {
				opts.acctInterestRate = strings.TrimPrefix(arg, "--interest-rate=")
			} else if strings.HasPrefix(arg, "--search=") {
				opts.searchTerm = strings.TrimPrefix(arg, "--search=")
			} else if strings.HasPrefix(arg, "--min=") {
				opts.minAmount = strings.TrimPrefix(arg, "--min=")
			} else if strings.HasPrefix(arg, "--max=") {
				opts.maxAmount = strings.TrimPrefix(arg, "--max=")
			} else if strings.HasPrefix(arg, "--post-scheduled=") {
				opts.postScheduled = strings.TrimPrefix(arg, "--post-scheduled=")
			} else if strings.HasPrefix(arg, "--skip-scheduled=") {
				opts.skipScheduled = strings.TrimPrefix(arg, "--skip-scheduled=")
			} else if strings.HasPrefix(arg, "--report=") {
				opts.report = true
				opts.reportType = strings.TrimPrefix(arg, "--report=")
			} else if strings.HasPrefix(arg, "--month=") {
				opts.reportMonth = strings.TrimPrefix(arg, "--month=")
			} else if strings.HasPrefix(arg, "--year=") {
				yearStr := strings.TrimPrefix(arg, "--year=")
				year, err := strconv.Atoi(yearStr)
				if err != nil {
					return nil, nil, fmt.Errorf("--year requires a valid year: %w", err)
				}
				opts.reportYear = year
			} else if strings.HasPrefix(arg, "--as-of=") {
				opts.reportAsOf = strings.TrimPrefix(arg, "--as-of=")
			} else {
				remaining = append(remaining, arg)
			}
		}
	}

	return opts, remaining, nil
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create service
	repo := repository.NewAccountRepository(database)
	svc := service.NewAccountService(repo, database)

	// List accounts (activeOnly = !includeClosed)
	accounts, err := svc.List(!opts.includeClosed)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get all balances
	balances, err := svc.GetAllBalances()
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create service
	repo := repository.NewAccountRepository(database)
	svc := service.NewAccountService(repo, database)

	// Get account by name
	account, err := svc.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Get balance
	balance, err := svc.GetBalance(account.ID)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create service
	repo := repository.NewAccountRepository(database)
	svc := service.NewAccountService(repo, database)

	// List accounts (active only)
	accounts, err := svc.List(true)
	if err != nil {
		return fmt.Errorf("failed to list accounts: %w", err)
	}

	// Get all balances
	balances, err := svc.GetAllBalances()
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Get account by name
	acctRepo := repository.NewAccountRepository(database)
	acctSvc := service.NewAccountService(acctRepo, database)
	account, err := acctSvc.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Create transaction service
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	txnSvc := service.NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

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
		transactions, err = txnSvc.ListByAccountAndDateRange(account.ID, startDate, endDate)
	} else {
		transactions, err = txnSvc.ListByAccount(account.ID)
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
	payees, _ := payeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categoryRepo := repository.NewCategoryRepository(database)
	categories, _ := categoryRepo.List()
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Get account by name
	acctRepo := repository.NewAccountRepository(database)
	acctSvc := service.NewAccountService(acctRepo, database)
	account, err := acctSvc.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Create transaction service
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	txnSvc := service.NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

	// Create payee service for auto-creation
	payeeSvc := service.NewPayeeService(payeeRepo, database)

	// Handle payee (auto-create if needed)
	var payeeID models.NullableID
	var payeeName string
	var payeeCreated bool
	if opts.txPayee != "" {
		payee, created, err := payeeSvc.GetOrCreate(opts.txPayee)
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
		categoryRepo := repository.NewCategoryRepository(database)
		// First try top-level category, then search all categories
		category, err := categoryRepo.GetByName(opts.txCategory, nil)
		if err != nil {
			// Try finding it as a subcategory (search all categories)
			categories, listErr := categoryRepo.List()
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
	if err := txnSvc.Create(txn); err != nil {
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Get source account by name
	acctRepo := repository.NewAccountRepository(database)
	acctSvc := service.NewAccountService(acctRepo, database)

	fromAcct, err := acctSvc.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	// Get destination account by name
	toAcct, err := acctSvc.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	// Create transaction service
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	txnSvc := service.NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

	// Create the transfer
	pair, err := txnSvc.CreateTransfer(fromAcct.ID, toAcct.ID, date, amount)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	// Set memo if provided
	if opts.txMemo != "" {
		err = txnSvc.UpdateTransfer(pair.FromTransaction.TransferID.ID, date, amount, opts.txMemo, models.TransactionStatusPending)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
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
		acctRepo := repository.NewAccountRepository(database)
		acctSvc := service.NewAccountService(acctRepo, database)
		account, err := acctSvc.GetByName(opts.accountName)
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

	// Create repositories
	txnRepo := repository.NewTransactionRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	acctRepo := repository.NewAccountRepository(database)

	// Search for transactions - we need to search by payee OR memo
	// Since the Search method uses AND logic, we'll do two searches and merge
	var transactions []*models.Transaction

	// Search by payee name
	payeeCriteria := criteria
	payeeCriteria.Memo = ""
	payeeResults, err := txnRepo.Search(payeeCriteria)
	if err != nil {
		return fmt.Errorf("failed to search transactions: %w", err)
	}

	// Search by memo
	memoCriteria := criteria
	memoCriteria.PayeeName = ""
	memoResults, err := txnRepo.Search(memoCriteria)
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

	payees, _ := payeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categories, _ := categoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	accounts, _ := acctRepo.List(false)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create account service
	acctRepo := repository.NewAccountRepository(database)
	acctSvc := service.NewAccountService(acctRepo, database)

	// Check if account name already exists
	if _, err := acctSvc.GetByName(opts.acctName); err == nil {
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
	if err := acctSvc.Create(account); err != nil {
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create scheduled transaction service
	stRepo := repository.NewScheduledTransactionRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	stSvc := service.NewScheduledTransactionService(stRepo, txnRepo, database)

	// Get scheduled transactions
	var scheduledTxns []*models.ScheduledTransaction
	if opts.scheduledDue {
		scheduledTxns, err = stSvc.ListDue()
	} else {
		scheduledTxns, err = stSvc.List()
	}
	if err != nil {
		return fmt.Errorf("failed to list scheduled transactions: %w", err)
	}

	// Filter by account if specified
	if opts.accountName != "" {
		acctRepo := repository.NewAccountRepository(database)
		acctSvc := service.NewAccountService(acctRepo, database)
		account, err := acctSvc.GetByName(opts.accountName)
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

	payeeRepo := repository.NewPayeeRepository(database)
	payees, _ := payeeRepo.List()
	for _, p := range payees {
		payeeNames[p.ID] = p.Name
	}

	categoryRepo := repository.NewCategoryRepository(database)
	categories, _ := categoryRepo.List()
	for _, c := range categories {
		categoryNames[c.ID] = c.Name
	}

	acctRepo := repository.NewAccountRepository(database)
	accounts, _ := acctRepo.List(false)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Parse the scheduled transaction ID
	stID, err := models.ParseID(opts.postScheduled)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	// Create scheduled transaction service
	stRepo := repository.NewScheduledTransactionRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	stSvc := service.NewScheduledTransactionService(stRepo, txnRepo, database)

	// Get the scheduled transaction first to show details
	st, err := stSvc.GetByID(stID)
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
		txn, err = stSvc.PostWithDate(stID, *date, amount)
	} else {
		txn, err = stSvc.Post(stID, amount)
	}
	if err != nil {
		return fmt.Errorf("failed to post scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := stSvc.GetByID(stID)

	// Get account info for currency
	acctRepo := repository.NewAccountRepository(database)
	account, _ := acctRepo.GetByID(st.AccountID)
	currency := "USD"
	accountName := "Unknown"
	if account != nil {
		currency = account.Currency
		accountName = account.Name
	}

	// Get payee name
	payeeName := "-"
	if st.HasPayee() {
		payeeRepo := repository.NewPayeeRepository(database)
		payee, err := payeeRepo.GetByID(st.PayeeID.ID)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Parse the scheduled transaction ID
	stID, err := models.ParseID(opts.skipScheduled)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	// Create scheduled transaction service
	stRepo := repository.NewScheduledTransactionRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	stSvc := service.NewScheduledTransactionService(stRepo, txnRepo, database)

	// Get the scheduled transaction first to show details
	st, err := stSvc.GetByID(stID)
	if err != nil {
		return fmt.Errorf("scheduled transaction not found: %w", err)
	}

	// Remember the old next date
	oldNextDate := st.NextDate

	// Skip the scheduled transaction
	err = stSvc.Skip(stID)
	if err != nil {
		return fmt.Errorf("failed to skip scheduled transaction: %w", err)
	}

	// Get updated scheduled transaction for next date
	stUpdated, _ := stSvc.GetByID(stID)

	// Get account info
	acctRepo := repository.NewAccountRepository(database)
	account, _ := acctRepo.GetByID(st.AccountID)
	accountName := "Unknown"
	if account != nil {
		accountName = account.Name
	}

	// Get payee name
	payeeName := "-"
	if st.HasPayee() {
		payeeRepo := repository.NewPayeeRepository(database)
		payee, err := payeeRepo.GetByID(st.PayeeID.ID)
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

// printScheduledTransactionsTable prints scheduled transactions in a formatted table.
func printScheduledTransactionsTable(w io.Writer, scheduledTxns []*models.ScheduledTransaction, dueOnly bool, accountNames map[models.ID]string, accountCurrencies map[models.ID]string, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
	if dueOnly {
		fmt.Fprintln(w, "DUE SCHEDULED TRANSACTIONS")
		fmt.Fprintln(w, "==========================")
	} else {
		fmt.Fprintln(w, "SCHEDULED TRANSACTIONS")
		fmt.Fprintln(w, "======================")
	}

	if len(scheduledTxns) == 0 {
		if dueOnly {
			fmt.Fprintln(w, "No scheduled transactions are due.")
		} else {
			fmt.Fprintln(w, "No scheduled transactions found.")
		}
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tAccount\tNext Date\tPayee\tAmount\tFrequency")
	fmt.Fprintln(tw, "--------\t-------\t---------\t-----\t------\t---------")

	for _, st := range scheduledTxns {
		// Truncate ID for display
		idStr := st.ID.String()
		if len(idStr) > 8 {
			idStr = idStr[:8]
		}

		account := accountNames[st.AccountID]
		if account == "" {
			account = "-"
		}

		currency := accountCurrencies[st.AccountID]
		if currency == "" {
			currency = "USD"
		}

		payee := "-"
		if st.HasPayee() {
			if name, ok := payeeNames[st.PayeeID.ID]; ok {
				payee = name
			}
		}

		amount := "~" // Variable amount indicator
		if st.HasAmount() {
			amount = formatMoney(st.Amount.Money, currency)
		}

		freq := st.Frequency.DisplayName()
		if st.OccurrencesRemaining.Valid {
			freq += fmt.Sprintf(" (%d left)", st.OccurrencesRemaining.Int64)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			idStr,
			account,
			st.NextDate.String(),
			payee,
			amount,
			freq,
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d scheduled transaction(s)\n", len(scheduledTxns))
}

// printAccountDetails prints detailed information for a single account.
func printAccountDetails(w io.Writer, account *models.Account, balance *service.AccountBalance) {
	fmt.Fprintf(w, "ACCOUNT: %s\n", account.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("ACCOUNT: ")+len(account.Name)))

	fmt.Fprintf(w, "Type:            %s\n", account.Type.DisplayName())
	fmt.Fprintf(w, "Currency:        %s\n", account.Currency)

	if account.Institution.Valid {
		fmt.Fprintf(w, "Institution:     %s\n", account.Institution.String)
	}

	if account.AccountNumber.Valid {
		// Mask account number for privacy
		num := account.AccountNumber.String
		if len(num) > 4 {
			num = "****" + num[len(num)-4:]
		}
		fmt.Fprintf(w, "Account Number:  %s\n", num)
	}

	fmt.Fprintf(w, "Opening Date:    %s\n", account.OpeningDate.String())
	fmt.Fprintf(w, "Opening Balance: %s\n", formatMoney(account.OpeningBalance, account.Currency))
	fmt.Fprintf(w, "Current Balance: %s\n", formatMoney(balance.CurrentBalance, account.Currency))
	fmt.Fprintf(w, "Cleared Balance: %s\n", formatMoney(balance.ClearedBalance, account.Currency))

	status := "Active"
	if !account.Active {
		status = "Closed"
	}
	fmt.Fprintf(w, "Status:          %s\n", status)

	// Type-specific details
	if account.CreditLimit.Valid {
		fmt.Fprintf(w, "Credit Limit:    %s\n", formatMoney(account.CreditLimit.Money, account.Currency))
	}
	if account.InterestRate.Valid {
		fmt.Fprintf(w, "Interest Rate:   %s%%\n", account.InterestRate.Money.String())
	}

	if account.Notes.Valid {
		fmt.Fprintf(w, "Notes:           %s\n", account.Notes.String)
	}
}

// printBalancesTable prints balances for all accounts with net worth.
func printBalancesTable(w io.Writer, accounts []*models.Account, balances map[models.ID]*service.AccountBalance) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "BALANCES")
	fmt.Fprintln(w, "========")

	var totalAssets, totalLiabilities models.Money

	for _, acct := range accounts {
		balance := models.MustNewMoney("0")
		if b, ok := balances[acct.ID]; ok {
			balance = b.CurrentBalance
		}

		fmt.Fprintf(w, "%-20s %s\n", acct.Name+":", formatMoney(balance, acct.Currency))

		// Track net worth
		if acct.Type.IsAssetType() {
			totalAssets = totalAssets.Add(balance)
		} else if acct.Type.IsLiabilityType() {
			totalLiabilities = totalLiabilities.Add(balance)
		}
	}

	fmt.Fprintln(w, "------------------------")

	// Net worth = assets - liabilities
	// For liabilities (credit cards, loans), the balance is typically negative
	// (representing what you owe), so we add them to get net worth
	netWorth := totalAssets.Add(totalLiabilities)
	fmt.Fprintf(w, "%-20s %s\n", "Net Worth:", formatMoney(netWorth, "USD"))
}

// printAccountsTable prints accounts in a formatted table.
func printAccountsTable(w io.Writer, accounts []*models.Account, balances map[models.ID]*service.AccountBalance) {
	if len(accounts) == 0 {
		fmt.Fprintln(w, "No accounts found.")
		return
	}

	fmt.Fprintln(w, "ACCOUNTS")
	fmt.Fprintln(w, "========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Name\tType\tBalance\tCurrency")
	fmt.Fprintln(tw, "----\t----\t-------\t--------")

	for _, acct := range accounts {
		balance := "N/A"
		if b, ok := balances[acct.ID]; ok {
			balance = formatMoney(b.CurrentBalance, acct.Currency)
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			acct.Name,
			acct.Type.DisplayName(),
			balance,
			acct.Currency,
		)
	}

	tw.Flush()
}

// printTransactionsTable prints transactions in a formatted table.
func printTransactionsTable(w io.Writer, account *models.Account, transactions []*models.Transaction, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
	fmt.Fprintf(w, "TRANSACTIONS: %s\n", account.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("TRANSACTIONS: ")+len(account.Name)))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Date\tPayee\tCategory\tAmount\tStatus")
	fmt.Fprintln(tw, "----\t-----\t--------\t------\t------")

	for _, txn := range transactions {
		payee := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				payee = name
			}
		}

		category := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				category = name
			}
		}

		// For transfers, show the transfer account
		if txn.IsTransfer() {
			payee = "[Transfer]"
			category = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			txn.Date.String(),
			payee,
			category,
			formatMoney(txn.Amount, account.Currency),
			txn.Status.DisplayName(),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d transaction(s)\n", len(transactions))
}

// printSearchResults prints search results in a formatted table.
func printSearchResults(w io.Writer, searchTerm string, transactions []*models.Transaction, accountNames map[models.ID]string, accountCurrencies map[models.ID]string, payeeNames map[models.ID]string, categoryNames map[models.ID]string) {
	fmt.Fprintf(w, "SEARCH RESULTS: %q\n", searchTerm)
	fmt.Fprintln(w, strings.Repeat("=", len("SEARCH RESULTS: ")+len(searchTerm)+2))

	if len(transactions) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Account\tDate\tPayee\tCategory\tAmount")
	fmt.Fprintln(tw, "-------\t----\t-----\t--------\t------")

	for _, txn := range transactions {
		account := accountNames[txn.AccountID]
		currency := accountCurrencies[txn.AccountID]
		if currency == "" {
			currency = "USD"
		}

		payee := "-"
		if txn.PayeeID.Valid {
			if name, ok := payeeNames[txn.PayeeID.ID]; ok {
				payee = name
			}
		}

		category := "-"
		if txn.CategoryID.Valid {
			if name, ok := categoryNames[txn.CategoryID.ID]; ok {
				category = name
			}
		}

		// For transfers, show transfer indicator
		if txn.IsTransfer() {
			payee = "[Transfer]"
			category = "-"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			account,
			txn.Date.String(),
			payee,
			category,
			formatMoney(txn.Amount, currency),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nFound %d transaction(s)\n", len(transactions))
}

// formatMoney formats a Money value with currency symbol.
// Always displays 2 decimal places for currencies.
func formatMoney(m models.Money, currency string) string {
	// Format with 2 decimal places
	value := fmt.Sprintf("%.2f", m.Float64())

	// Determine symbol and formatting
	var symbol string
	var format string
	switch currency {
	case "USD":
		symbol = "$"
		format = "symbol"
	case "EUR":
		symbol = "€"
		format = "symbol"
	case "GBP":
		symbol = "£"
		format = "symbol"
	default:
		return fmt.Sprintf("%s %s", currency, value)
	}

	if format == "symbol" {
		if m.IsNegative() {
			return fmt.Sprintf("-%s%s", symbol, strings.TrimPrefix(value, "-"))
		}
		return fmt.Sprintf("%s%s", symbol, value)
	}

	return value
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
	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create report service
	acctRepo := repository.NewAccountRepository(database)
	reportSvc := service.NewReportService(acctRepo, database)

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
		report, err = reportSvc.NetWorthAsOfIncludingClosed(asOf)
	} else {
		report, err = reportSvc.NetWorthAsOf(asOf)
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

	// Open database
	database, err := db.Open(opts.file)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Create report service
	acctRepo := repository.NewAccountRepository(database)
	reportSvc := service.NewReportService(acctRepo, database)

	// Generate report based on period type
	var report *models.SpendingReport

	if opts.reportMonth != "" {
		// Parse YYYY-MM format
		year, month, err := parseYearMonth(opts.reportMonth)
		if err != nil {
			return fmt.Errorf("invalid --month format: %w", err)
		}
		report, err = reportSvc.SpendingByCategoryMonth(year, month)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	} else if opts.reportYear != 0 {
		report, err = reportSvc.SpendingByCategoryYear(opts.reportYear)
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

		report, err = reportSvc.SpendingByCategoryDateRange(time.Time(startDate), time.Time(endDate))
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	}

	// Print report
	printSpendingReport(w, report)
	return nil
}

// parseYearMonth parses a YYYY-MM string into year and month integers.
func parseYearMonth(s string) (int, int, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected YYYY-MM format, got %q", s)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid year: %w", err)
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid month: %w", err)
	}

	if month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("month must be between 1 and 12, got %d", month)
	}

	return year, month, nil
}

// printNetWorthReport prints the net worth report.
func printNetWorthReport(w io.Writer, report *models.NetWorthReport) {
	fmt.Fprintln(w, "NET WORTH REPORT")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n\n", report.AsOfDate.Format("January 2, 2006"))

	// Assets section
	fmt.Fprintln(w, "ASSETS")
	fmt.Fprintln(w, "------")
	if len(report.Assets) == 0 {
		fmt.Fprintln(w, "  (No asset accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, acct := range report.Assets {
			fmt.Fprintf(tw, "  %s\t%s\n", acct.Name, formatMoney(acct.Balance, "USD"))
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Assets:\t\t%s\n\n", formatMoney(report.TotalAssets, "USD"))

	// Liabilities section
	fmt.Fprintln(w, "LIABILITIES")
	fmt.Fprintln(w, "-----------")
	if len(report.Liabilities) == 0 {
		fmt.Fprintln(w, "  (No liability accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, acct := range report.Liabilities {
			fmt.Fprintf(tw, "  %s\t%s\n", acct.Name, formatMoney(acct.Balance, "USD"))
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Liabilities:\t%s\n\n", formatMoney(report.TotalLiabilities, "USD"))

	// Net worth
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "NET WORTH:\t\t%s\n", formatMoney(report.NetWorth, "USD"))
}

// printSpendingReport prints the spending by category report.
func printSpendingReport(w io.Writer, report *models.SpendingReport) {
	fmt.Fprintln(w, "SPENDING BY CATEGORY")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Period: %s\n\n", report.Period)

	if len(report.Categories) == 0 {
		fmt.Fprintln(w, "No spending found for this period.")
		return
	}

	// Print category spending with visual bars
	maxBarWidth := 30
	maxAmount := models.ZeroMoney
	for _, cat := range report.Categories {
		if cat.Amount.Cmp(maxAmount) > 0 {
			maxAmount = cat.Amount
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Category\tAmount\t%\tBar")
	fmt.Fprintln(tw, "--------\t------\t-\t---")

	for _, cat := range report.Categories {
		// Calculate bar length
		barLen := 0
		if !maxAmount.IsZero() {
			barLen = int(cat.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
		}
		bar := strings.Repeat("█", barLen)

		fmt.Fprintf(tw, "%s\t%s\t%.1f%%\t%s\n",
			cat.Name,
			formatMoney(cat.Amount, "USD"),
			cat.Percentage,
			bar,
		)

		// Print subcategories with indentation
		for _, sub := range cat.Subcategories {
			subBarLen := 0
			if !maxAmount.IsZero() {
				subBarLen = int(sub.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
			}
			subBar := strings.Repeat("░", subBarLen)

			fmt.Fprintf(tw, "  %s\t%s\t%.1f%%\t%s\n",
				sub.Name,
				formatMoney(sub.Amount, "USD"),
				sub.Percentage,
				subBar,
			)
		}
	}
	tw.Flush()

	fmt.Fprintf(w, "\n------------------------\nTotal Spending:\t%s\n", formatMoney(report.TotalSpending, "USD"))
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "tmoney version %s\n", Version)
	fmt.Fprintf(w, "Build time: %s\n", BuildTime)
	fmt.Fprintf(w, "Git commit: %s\n", GitCommit)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `TMoney - Personal Finance Manager

Usage:
  tmoney [file.tdb]              Launch TUI with optional file
  tmoney [options]               Run CLI commands

Global Options:
  -f, --file <path>    Specify database file
  -h, --help           Show this help message
  -v, --version        Show version information

Database Commands:
  --create <path>      Create a new database file

Account Commands:
  --list-accounts      List all accounts
    --include-closed   Include closed accounts in listing
  --account <name>     Show details for a specific account
  --balance            Show balances for all accounts with net worth

  --add-account        Create a new account (requires --name, --type)
    --name <name>            Account name
    --type <type>            Account type (checking, savings, credit_card,
                             investment, cash, loan, asset)
    --currency <code>        Currency code (default: USD)
    --opening-balance <amt>  Opening balance (default: 0)
    --opening-date <date>    Opening date (YYYY-MM-DD, default: today)
    --institution <name>     Financial institution name
    --account-number <num>   Account number
    --notes <text>           Account notes
    --credit-limit <amt>     Credit limit (credit_card only)
    --interest-rate <rate>   Interest rate % (loan only)

Transaction Commands:
  --transactions       List transactions (requires --account)
    --account <name>   Account to show transactions for
    --limit <n>        Limit number of transactions shown
    --from <date>      Start date filter (YYYY-MM-DD)
    --to <date>        End date filter (YYYY-MM-DD)

  --add-transaction    Add a new transaction (requires --account, --amount)
    --account <name>   Account for the transaction
    --amount <value>   Transaction amount (negative for expenses)
    --payee <name>     Payee name (auto-created if new)
    --category <name>  Category name
    --date <date>      Transaction date (YYYY-MM-DD, default: today)
    --memo <text>      Transaction memo/note

  --transfer           Create a transfer between accounts
    --from <account>   Source account name
    --to <account>     Destination account name
    --amount <value>   Transfer amount (must be positive)
    --date <date>      Transfer date (YYYY-MM-DD, default: today)
    --memo <text>      Transfer memo/note

  --search <term>      Search transactions by payee name or memo
    --account <name>   Filter by account
    --from <date>      Start date filter (YYYY-MM-DD)
    --to <date>        End date filter (YYYY-MM-DD)
    --category <name>  Filter by category
    --min <amount>     Minimum amount filter
    --max <amount>     Maximum amount filter

Scheduled Transaction Commands:
  --scheduled          List all scheduled transactions
    --due              Show only due scheduled transactions
    --account <name>   Filter by account

  --post-scheduled <id>  Post a scheduled transaction (create real transaction)
    --amount <value>     Override amount (for variable amount schedules)
    --date <date>        Override date (YYYY-MM-DD, default: scheduled date)

  --skip-scheduled <id>  Skip a scheduled transaction (advance to next date)

Report Commands:
  --report net-worth     Generate net worth report
    --as-of <date>       Report as of specific date (YYYY-MM-DD, default: today)
    --include-closed     Include closed accounts in report

  --report spending      Generate spending by category report
    --month <YYYY-MM>    Report for a specific month
    --year <YYYY>        Report for a specific year
    --from <date>        Start date for custom range (YYYY-MM-DD)
    --to <date>          End date for custom range (YYYY-MM-DD)

For more information, visit: https://github.com/haskovec/tmoney`)
}
