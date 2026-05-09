package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CLI option flags
type cliOptions struct {
	file          string
	includeClosed bool
	accountName   string // --account <name> to show details
	fromDate      string // --from <YYYY-MM-DD> start date filter
	toDate        string // --to <YYYY-MM-DD> end date filter

	// Transaction options shared by legacy verbs (--transfer, --void,
	// --add-scheduled, --post-scheduled, --search, etc.). Retired as
	// each verb migrates to Cobra.
	txAmount   string // --amount <value>
	txPayee    string // --payee <name>
	txCategory string // --category <name>
	txDate     string // --date <YYYY-MM-DD>
	txMemo     string // --memo <text>

	// Transfer options (residual: fromAccount/toAccount still used by
	// --transfer-shares; the standalone --transfer flag has been
	// migrated to `tmoney transfer add`.)
	fromAccount string // --from <account> for transfers
	toAccount   string // --to <account> for transfers

	// Add account / shared --name+--type/etc. options
	acctName         string // --name <name>
	acctType         string // --type <type>
	acctCurrency     string // --currency <code>
	acctOpeningBal   string // --opening-balance <value>
	acctOpeningDate  string // --opening-date <YYYY-MM-DD>
	acctInstitution  string // --institution <name>
	acctNumber       string // --account-number <number>
	acctNotes        string // --notes <text>
	acctCreditLimit  string // --credit-limit <value>
	acctInterestRate string // --interest-rate <value>

	// Status filter
	txStatus string // --status <uncleared|cleared|reconciled|void>

	// Report options
	report      bool   // --report flag
	reportType  string // net-worth or spending
	reportMonth string // --month YYYY-MM for spending
	reportYear  int    // --year YYYY for spending
	reportAsOf  string // --as-of YYYY-MM-DD for net-worth

	// Import options
	importFile       string // --import <file>
	confirm          bool   // --confirm flag (execute import instead of dry-run)
	skipDuplicates   bool   // --skip-duplicates flag
	updateDuplicates bool   // --update-duplicates flag
	formatOverride   string // --format <csv|qif|ofx>
	sourceAccount    string // --source-account <name> picks one account out of a multi-account CSV

	// Export options
	exportFile string // --export <file>

	// Security management options
	secTicker     string // --ticker <ticker> (for add/edit)
	secAssetClass string // --asset-class <class>
	secExchange   string // --exchange <exchange>

	// Price management options
	priceValue string // --price <value>

	// Investment transaction options
	shares        string // --shares <quantity>
	commission    string // --commission <value>
	pricePerShare string // --price-per-share <value>
	lot           string // --lot <lot-id>

	// Portfolio options
	portfolio bool // --portfolio flag
	showLots  bool // --show-lots flag

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
		case "--include-closed":
			opts.includeClosed = true
		case "--account":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--account requires an account name argument")
			}
			i++
			opts.accountName = args[i]
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
		case "--import":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--import requires a file path argument")
			}
			i++
			opts.importFile = args[i]
		case "--export":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--export requires a file path argument")
			}
			i++
			opts.exportFile = args[i]
		case "--confirm":
			opts.confirm = true
		case "--skip-duplicates":
			opts.skipDuplicates = true
		case "--update-duplicates":
			opts.updateDuplicates = true
		case "--format":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--format requires a value argument (csv, qif, or ofx)")
			}
			i++
			opts.formatOverride = args[i]
		case "--source-account":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--source-account requires a name argument")
			}
			i++
			opts.sourceAccount = args[i]
		case "--ticker":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--ticker requires a value argument")
			}
			i++
			opts.secTicker = args[i]
		case "--asset-class":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--asset-class requires a value argument")
			}
			i++
			opts.secAssetClass = args[i]
		case "--exchange":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--exchange requires a value argument")
			}
			i++
			opts.secExchange = args[i]
		case "--price":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--price requires a value argument")
			}
			i++
			opts.priceValue = args[i]
		case "--shares":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--shares requires a value argument")
			}
			i++
			opts.shares = args[i]
		case "--commission":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--commission requires a value argument")
			}
			i++
			opts.commission = args[i]
		case "--price-per-share":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--price-per-share requires a value argument")
			}
			i++
			opts.pricePerShare = args[i]
		case "--lot":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--lot requires a lot ID argument")
			}
			i++
			opts.lot = args[i]
		case "--portfolio":
			opts.portfolio = true
		case "--show-lots":
			opts.showLots = true
		default:
			// Check for --flag=value formats
			if after, ok := strings.CutPrefix(arg, "--file="); ok {
				opts.file = after
			} else if after, ok := strings.CutPrefix(arg, "-f="); ok {
				opts.file = after
			} else if after, ok := strings.CutPrefix(arg, "--account="); ok {
				opts.accountName = after
			} else if after, ok := strings.CutPrefix(arg, "--from="); ok {
				val := after
				opts.fromDate = val
				opts.fromAccount = val
			} else if after, ok := strings.CutPrefix(arg, "--to="); ok {
				val := after
				opts.toDate = val
				opts.toAccount = val
			} else if after, ok := strings.CutPrefix(arg, "--amount="); ok {
				opts.txAmount = after
			} else if after, ok := strings.CutPrefix(arg, "--payee="); ok {
				opts.txPayee = after
			} else if after, ok := strings.CutPrefix(arg, "--category="); ok {
				opts.txCategory = after
			} else if after, ok := strings.CutPrefix(arg, "--date="); ok {
				opts.txDate = after
			} else if after, ok := strings.CutPrefix(arg, "--memo="); ok {
				opts.txMemo = after
			} else if after, ok := strings.CutPrefix(arg, "--name="); ok {
				opts.acctName = after
			} else if after, ok := strings.CutPrefix(arg, "--type="); ok {
				opts.acctType = after
			} else if after, ok := strings.CutPrefix(arg, "--currency="); ok {
				opts.acctCurrency = after
			} else if after, ok := strings.CutPrefix(arg, "--opening-balance="); ok {
				opts.acctOpeningBal = after
			} else if after, ok := strings.CutPrefix(arg, "--opening-date="); ok {
				opts.acctOpeningDate = after
			} else if after, ok := strings.CutPrefix(arg, "--institution="); ok {
				opts.acctInstitution = after
			} else if after, ok := strings.CutPrefix(arg, "--account-number="); ok {
				opts.acctNumber = after
			} else if after, ok := strings.CutPrefix(arg, "--notes="); ok {
				opts.acctNotes = after
			} else if after, ok := strings.CutPrefix(arg, "--credit-limit="); ok {
				opts.acctCreditLimit = after
			} else if after, ok := strings.CutPrefix(arg, "--interest-rate="); ok {
				opts.acctInterestRate = after
			} else if after, ok := strings.CutPrefix(arg, "--status="); ok {
				opts.txStatus = after
			} else if strings.HasPrefix(arg, "--report=") {
				opts.report = true
				opts.reportType = strings.TrimPrefix(arg, "--report=")
			} else if after, ok := strings.CutPrefix(arg, "--month="); ok {
				opts.reportMonth = after
			} else if after, ok := strings.CutPrefix(arg, "--year="); ok {
				yearStr := after
				year, err := strconv.Atoi(yearStr)
				if err != nil {
					return nil, nil, fmt.Errorf("--year requires a valid year: %w", err)
				}
				opts.reportYear = year
			} else if after, ok := strings.CutPrefix(arg, "--as-of="); ok {
				opts.reportAsOf = after
			} else if after, ok := strings.CutPrefix(arg, "--import="); ok {
				opts.importFile = after
			} else if after, ok := strings.CutPrefix(arg, "--export="); ok {
				opts.exportFile = after
			} else if after, ok := strings.CutPrefix(arg, "--format="); ok {
				opts.formatOverride = after
			} else if after, ok := strings.CutPrefix(arg, "--ticker="); ok {
				opts.secTicker = after
			} else if after, ok := strings.CutPrefix(arg, "--asset-class="); ok {
				opts.secAssetClass = after
			} else if after, ok := strings.CutPrefix(arg, "--exchange="); ok {
				opts.secExchange = after
			} else if after, ok := strings.CutPrefix(arg, "--price="); ok {
				opts.priceValue = after
			} else if after, ok := strings.CutPrefix(arg, "--shares="); ok {
				opts.shares = after
			} else if after, ok := strings.CutPrefix(arg, "--commission="); ok {
				opts.commission = after
			} else if after, ok := strings.CutPrefix(arg, "--price-per-share="); ok {
				opts.pricePerShare = after
			} else if after, ok := strings.CutPrefix(arg, "--lot="); ok {
				opts.lot = after
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
