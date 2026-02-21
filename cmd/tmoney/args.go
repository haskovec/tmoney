package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// Void transaction
	voidTxn string // --void <txn-id>

	// Status filter
	txStatus string // --status <uncleared|cleared|reconciled|void>

	// Report options
	report       bool   // --report flag
	reportType   string // net-worth or spending
	reportMonth  string // --month YYYY-MM for spending
	reportYear   int    // --year YYYY for spending
	reportAsOf   string // --as-of YYYY-MM-DD for net-worth
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
		case "--void":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--void requires a transaction ID argument")
			}
			i++
			opts.voidTxn = args[i]
		case "--status":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--status requires a status value (uncleared, cleared, reconciled, void)")
			}
			i++
			opts.txStatus = args[i]
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
			} else if strings.HasPrefix(arg, "--void=") {
				opts.voidTxn = strings.TrimPrefix(arg, "--void=")
			} else if strings.HasPrefix(arg, "--status=") {
				opts.txStatus = strings.TrimPrefix(arg, "--status=")
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
