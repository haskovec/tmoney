package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
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

	// Default to TUI mode (placeholder for now)
	fmt.Fprintln(stdout, "TMoney - Personal Finance Manager")
	fmt.Fprintln(stdout, "TUI mode not yet implemented.")
	fmt.Fprintln(stdout, "Use --help for available options.")

	return nil
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
		case "--create":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--create requires a path argument")
			}
			i++
			opts.createDB = args[i]
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

For more information, visit: https://github.com/haskovec/tmoney`)
}
