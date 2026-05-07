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
	transactions  bool   // --transactions to list transactions
	limit         int    // --limit <n> to limit results
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

	// Add scheduled transaction options
	addScheduled  bool   // --add-scheduled flag
	stFrequency   string // --frequency <daily|weekly|biweekly|monthly|quarterly|yearly>
	stDay         string // --day <1-31 or -1 for last day>
	stOccurrences string // --occurrences <n>
	stEndDate     string // --end-date <YYYY-MM-DD>
	autoPost      bool   // --auto-post flag
	leadDays      string // --lead-days <0|3|7>

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

	// Void transaction
	voidTxn string // --void <txn-id>

	// Status filter
	txStatus string // --status <uncleared|cleared|reconciled|void>

	// Report options
	report      bool   // --report flag
	reportType  string // net-worth or spending
	reportMonth string // --month YYYY-MM for spending
	reportYear  int    // --year YYYY for spending
	reportAsOf  string // --as-of YYYY-MM-DD for net-worth

	// Reconciliation options
	startReconcile   bool     // --start-reconcile flag
	markReconciled   []string // --mark-reconciled <txn-id>... (remaining args)
	finishReconcile  bool     // --finish-reconcile flag
	reconcileStatus  bool     // --reconcile-status flag
	reconcileForce   bool     // --force flag (for finish with non-zero diff)
	statementDate    string   // --statement-date <YYYY-MM-DD>
	statementBalance string   // --statement-balance <amount>

	// Import options
	importFile       string // --import <file>
	confirm          bool   // --confirm flag (execute import instead of dry-run)
	skipDuplicates   bool   // --skip-duplicates flag
	updateDuplicates bool   // --update-duplicates flag
	formatOverride   string // --format <csv|qif|ofx>
	sourceAccount    string // --source-account <name> picks one account out of a multi-account CSV

	// Export options
	exportFile string // --export <file>

	// Link transfers options
	linkTransfers   bool // --link-transfers flag
	maxDateDiffDays int  // --max-days <n> (default 5)

	// Security management options
	listSecurities bool   // --list-securities flag
	securityTicker string // --security <ticker> to show details
	addSecurity    bool   // --add-security flag
	editSecurity   string // --edit-security <ticker>
	hideSecurity   string // --hide-security <ticker>
	unhideSecurity string // --unhide-security <ticker>
	deleteSecurity string // --delete-security <ticker>
	secTicker      string // --ticker <ticker> (for add/edit)
	secAssetClass  string // --asset-class <class>
	secExchange    string // --exchange <exchange>
	includeHidden  bool   // --include-hidden flag

	// Price management options
	listPrices    bool     // --prices flag
	addPrice      bool     // --add-price flag
	currentPrice  bool     // --current-price flag
	priceValue    string   // --price <value>
	importPrices  string   // --import-prices <file>
	overwrite     bool     // --overwrite flag
	updatePrices  bool     // --update-prices flag
	updateTickers []string // optional positional tickers after --update-prices
	provider      string   // --provider <name> for --update-prices

	// Investment transaction options
	buy            bool   // --buy flag
	sell           bool   // --sell flag
	dividend       bool   // --dividend flag
	reinvest       bool   // --reinvest flag
	investmentFee  bool   // --investment-fee flag
	investDeposit  bool   // --invest-deposit flag
	investWithdraw bool   // --invest-withdraw flag
	transferShares bool   // --transfer-shares flag
	shares         string // --shares <quantity>
	commission     string // --commission <value>
	pricePerShare  string // --price-per-share <value>
	lot            string // --lot <lot-id>

	// Portfolio options
	portfolio bool // --portfolio flag
	showLots  bool // --show-lots flag

	// Corporate action options
	split            bool   // --split flag
	splitRatio       string // --ratio <N:D> (for split)
	mergeSecurity    bool   // --merge-security flag
	mergeSource      string // --source <ticker>
	mergeTarget      string // --target <ticker>
	exchangeRatio    string // --exchange-ratio <ratio>
	cashPerShare     string // --cash-per-share <amount>
	spinOff          bool   // --spin-off flag
	spinOffParent    string // --parent <ticker>
	spinOffChild     string // --spinoff <ticker>
	shareRatio       string // --share-ratio <ratio>
	parentAllocation string // --parent-allocation <percent>
	spinOffPrice     string // --spin-off-price <price>
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
		case "--add-scheduled":
			opts.addScheduled = true
		case "--frequency":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--frequency requires a value argument")
			}
			i++
			opts.stFrequency = args[i]
		case "--day":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--day requires a value argument")
			}
			i++
			opts.stDay = args[i]
		case "--occurrences":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--occurrences requires a value argument")
			}
			i++
			opts.stOccurrences = args[i]
		case "--end-date":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--end-date requires a date argument (YYYY-MM-DD)")
			}
			i++
			opts.stEndDate = args[i]
		case "--auto-post":
			opts.autoPost = true
		case "--lead-days":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--lead-days requires a value argument (0, 3, or 7)")
			}
			i++
			opts.leadDays = args[i]
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
		case "--start-reconcile":
			opts.startReconcile = true
		case "--finish-reconcile":
			opts.finishReconcile = true
		case "--reconcile-status":
			opts.reconcileStatus = true
		case "--force":
			opts.reconcileForce = true
		case "--statement-date":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--statement-date requires a date argument (YYYY-MM-DD)")
			}
			i++
			opts.statementDate = args[i]
		case "--statement-balance":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--statement-balance requires an amount argument")
			}
			i++
			opts.statementBalance = args[i]
		case "--mark-reconciled":
			// Collect all following non-flag arguments as transaction IDs
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.markReconciled = append(opts.markReconciled, args[i])
			}
			if len(opts.markReconciled) == 0 {
				return nil, nil, fmt.Errorf("--mark-reconciled requires at least one transaction ID")
			}
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
		case "--link-transfers":
			opts.linkTransfers = true
		case "--max-days":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--max-days requires a number argument")
			}
			i++
			n, perr := strconv.Atoi(args[i])
			if perr != nil || n < 0 {
				return nil, nil, fmt.Errorf("--max-days must be a non-negative integer, got %q", args[i])
			}
			opts.maxDateDiffDays = n
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
		case "--list-securities":
			opts.listSecurities = true
		case "--security":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--security requires a ticker argument")
			}
			i++
			opts.securityTicker = args[i]
		case "--add-security":
			opts.addSecurity = true
		case "--edit-security":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--edit-security requires a ticker argument")
			}
			i++
			opts.editSecurity = args[i]
		case "--hide-security":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--hide-security requires a ticker argument")
			}
			i++
			opts.hideSecurity = args[i]
		case "--unhide-security":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--unhide-security requires a ticker argument")
			}
			i++
			opts.unhideSecurity = args[i]
		case "--delete-security":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--delete-security requires a ticker argument")
			}
			i++
			opts.deleteSecurity = args[i]
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
		case "--include-hidden":
			opts.includeHidden = true
		case "--prices":
			opts.listPrices = true
		case "--add-price":
			opts.addPrice = true
		case "--current-price":
			opts.currentPrice = true
		case "--price":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--price requires a value argument")
			}
			i++
			opts.priceValue = args[i]
		case "--import-prices":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--import-prices requires a file path argument")
			}
			i++
			opts.importPrices = args[i]
		case "--overwrite":
			opts.overwrite = true
		case "--update-prices":
			opts.updatePrices = true
			// Collect any following non-flag arguments as ticker filters.
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				opts.updateTickers = append(opts.updateTickers, args[i])
			}
		case "--provider":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--provider requires a name argument")
			}
			i++
			opts.provider = args[i]
		case "--buy":
			opts.buy = true
		case "--sell":
			opts.sell = true
		case "--dividend":
			opts.dividend = true
		case "--reinvest":
			opts.reinvest = true
		case "--investment-fee":
			opts.investmentFee = true
		case "--invest-deposit":
			opts.investDeposit = true
		case "--invest-withdraw":
			opts.investWithdraw = true
		case "--transfer-shares":
			opts.transferShares = true
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
		case "--split":
			opts.split = true
		case "--ratio":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--ratio requires a value argument (e.g. 4:1)")
			}
			i++
			opts.splitRatio = args[i]
		case "--merge-security":
			opts.mergeSecurity = true
		case "--source":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--source requires a ticker argument")
			}
			i++
			opts.mergeSource = args[i]
		case "--target":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--target requires a ticker argument")
			}
			i++
			opts.mergeTarget = args[i]
		case "--exchange-ratio":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--exchange-ratio requires a value argument")
			}
			i++
			opts.exchangeRatio = args[i]
		case "--cash-per-share":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--cash-per-share requires a value argument")
			}
			i++
			opts.cashPerShare = args[i]
		case "--spin-off":
			opts.spinOff = true
		case "--parent":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--parent requires a ticker argument")
			}
			i++
			opts.spinOffParent = args[i]
		case "--spinoff":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--spinoff requires a ticker argument")
			}
			i++
			opts.spinOffChild = args[i]
		case "--share-ratio":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--share-ratio requires a value argument")
			}
			i++
			opts.shareRatio = args[i]
		case "--parent-allocation":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--parent-allocation requires a percentage value")
			}
			i++
			opts.parentAllocation = args[i]
		case "--spin-off-price":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--spin-off-price requires a value argument")
			}
			i++
			opts.spinOffPrice = args[i]
		default:
			// Check for --flag=value formats
			if after, ok := strings.CutPrefix(arg, "--file="); ok {
				opts.file = after
			} else if after, ok := strings.CutPrefix(arg, "-f="); ok {
				opts.file = after
			} else if after, ok := strings.CutPrefix(arg, "--account="); ok {
				opts.accountName = after
			} else if after, ok := strings.CutPrefix(arg, "--limit="); ok {
				limitStr := after
				limit, err := strconv.Atoi(limitStr)
				if err != nil {
					return nil, nil, fmt.Errorf("--limit requires a valid number: %w", err)
				}
				if limit < 1 {
					return nil, nil, fmt.Errorf("--limit must be a positive number")
				}
				opts.limit = limit
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
			} else if after, ok := strings.CutPrefix(arg, "--void="); ok {
				opts.voidTxn = after
			} else if after, ok := strings.CutPrefix(arg, "--status="); ok {
				opts.txStatus = after
			} else if after, ok := strings.CutPrefix(arg, "--search="); ok {
				opts.searchTerm = after
			} else if after, ok := strings.CutPrefix(arg, "--min="); ok {
				opts.minAmount = after
			} else if after, ok := strings.CutPrefix(arg, "--max="); ok {
				opts.maxAmount = after
			} else if after, ok := strings.CutPrefix(arg, "--post-scheduled="); ok {
				opts.postScheduled = after
			} else if after, ok := strings.CutPrefix(arg, "--skip-scheduled="); ok {
				opts.skipScheduled = after
			} else if after, ok := strings.CutPrefix(arg, "--frequency="); ok {
				opts.stFrequency = after
			} else if after, ok := strings.CutPrefix(arg, "--day="); ok {
				opts.stDay = after
			} else if after, ok := strings.CutPrefix(arg, "--occurrences="); ok {
				opts.stOccurrences = after
			} else if after, ok := strings.CutPrefix(arg, "--end-date="); ok {
				opts.stEndDate = after
			} else if after, ok := strings.CutPrefix(arg, "--lead-days="); ok {
				opts.leadDays = after
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
			} else if after, ok := strings.CutPrefix(arg, "--statement-date="); ok {
				opts.statementDate = after
			} else if after, ok := strings.CutPrefix(arg, "--statement-balance="); ok {
				opts.statementBalance = after
			} else if after, ok := strings.CutPrefix(arg, "--import="); ok {
				opts.importFile = after
			} else if after, ok := strings.CutPrefix(arg, "--export="); ok {
				opts.exportFile = after
			} else if after, ok := strings.CutPrefix(arg, "--format="); ok {
				opts.formatOverride = after
			} else if after, ok := strings.CutPrefix(arg, "--security="); ok {
				opts.securityTicker = after
			} else if after, ok := strings.CutPrefix(arg, "--edit-security="); ok {
				opts.editSecurity = after
			} else if after, ok := strings.CutPrefix(arg, "--hide-security="); ok {
				opts.hideSecurity = after
			} else if after, ok := strings.CutPrefix(arg, "--unhide-security="); ok {
				opts.unhideSecurity = after
			} else if after, ok := strings.CutPrefix(arg, "--delete-security="); ok {
				opts.deleteSecurity = after
			} else if after, ok := strings.CutPrefix(arg, "--ticker="); ok {
				opts.secTicker = after
			} else if after, ok := strings.CutPrefix(arg, "--asset-class="); ok {
				opts.secAssetClass = after
			} else if after, ok := strings.CutPrefix(arg, "--exchange="); ok {
				opts.secExchange = after
			} else if after, ok := strings.CutPrefix(arg, "--price="); ok {
				opts.priceValue = after
			} else if after, ok := strings.CutPrefix(arg, "--import-prices="); ok {
				opts.importPrices = after
			} else if after, ok := strings.CutPrefix(arg, "--provider="); ok {
				opts.provider = after
			} else if after, ok := strings.CutPrefix(arg, "--shares="); ok {
				opts.shares = after
			} else if after, ok := strings.CutPrefix(arg, "--commission="); ok {
				opts.commission = after
			} else if after, ok := strings.CutPrefix(arg, "--price-per-share="); ok {
				opts.pricePerShare = after
			} else if after, ok := strings.CutPrefix(arg, "--lot="); ok {
				opts.lot = after
			} else if after, ok := strings.CutPrefix(arg, "--ratio="); ok {
				opts.splitRatio = after
			} else if after, ok := strings.CutPrefix(arg, "--source="); ok {
				opts.mergeSource = after
			} else if after, ok := strings.CutPrefix(arg, "--target="); ok {
				opts.mergeTarget = after
			} else if after, ok := strings.CutPrefix(arg, "--exchange-ratio="); ok {
				opts.exchangeRatio = after
			} else if after, ok := strings.CutPrefix(arg, "--cash-per-share="); ok {
				opts.cashPerShare = after
			} else if after, ok := strings.CutPrefix(arg, "--parent="); ok {
				opts.spinOffParent = after
			} else if after, ok := strings.CutPrefix(arg, "--spinoff="); ok {
				opts.spinOffChild = after
			} else if after, ok := strings.CutPrefix(arg, "--share-ratio="); ok {
				opts.shareRatio = after
			} else if after, ok := strings.CutPrefix(arg, "--parent-allocation="); ok {
				opts.parentAllocation = after
			} else if after, ok := strings.CutPrefix(arg, "--spin-off-price="); ok {
				opts.spinOffPrice = after
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
