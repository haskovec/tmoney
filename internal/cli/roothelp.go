package cli

import (
	"fmt"
	"io"
)

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
  --account <name>     Show details for a specific account
  --balance            Show balances for all accounts with net worth

Transaction Commands:
  --transactions       List transactions (requires --account)
    --account <name>   Account to show transactions for
    --limit <n>        Limit number of transactions shown
    --from <date>      Start date filter (YYYY-MM-DD)
    --to <date>        End date filter (YYYY-MM-DD)
    --status <status>  Filter by status (uncleared, cleared, reconciled, void)

  --void <txn-id>      Void a transaction (sets amount to 0, status to void)

  transaction add      Add a new transaction (requires --account, --amount)
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

  --add-scheduled      Create a new scheduled transaction
    --account <name>         Account for the scheduled transaction
    --frequency <freq>       Frequency (daily, weekly, fortnightly, semimonthly, monthly, quarterly, yearly)
    --amount <value>         Transaction amount (omit for variable amount)
    --payee <name>           Payee name
    --category <name>        Category name
    --date <date>            Start date (YYYY-MM-DD, default: today)
    --memo <text>            Memo/note
    --day <1-31|-1>          Day of month (monthly/quarterly; -1 for last day)
    --occurrences <n>        Number of times to repeat
    --end-date <date>        End date for the schedule (YYYY-MM-DD)
    --auto-post              Automatically post when due
    --lead-days <0|3|7>      Days before due date to auto-post (requires --auto-post)

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

Reconciliation Commands:
  --start-reconcile      Start reconciliation (requires --account, --statement-date,
                         --statement-balance)
    --account <name>           Account to reconcile
    --statement-date <date>    Bank statement date (YYYY-MM-DD)
    --statement-balance <amt>  Bank statement ending balance

  --mark-reconciled <id>...  Mark transactions for reconciliation (one or more IDs)

  --finish-reconcile     Complete reconciliation (requires --account)
    --account <name>     Account to finish reconciling
    --force              Complete even with non-zero difference

  --reconcile-status     Show reconciliation status (requires --account)
    --account <name>     Account to check status for

Import Commands:
  --import <file>        Import transactions from a file (dry-run by default)
    --account <name>     Target account for imported transactions
    --format <fmt>       Override format detection (csv, qif, or ofx)
    --confirm            Execute the import (default is dry-run preview)
    --skip-duplicates    Skip matched/duplicate transactions
    --update-duplicates  Update existing matched transactions

Export Commands:
  --export <file>        Export transactions to a file
    --format <fmt>       Override format detection (csv or qif)
    --account <name>     Export only a specific account
    --from <date>        Start date filter (YYYY-MM-DD)
    --to <date>          End date filter (YYYY-MM-DD)

Backup/Restore Commands:
  --backup               Create a manual backup of the database
  --restore <file>       Restore database from a backup file

Security Commands:
  --edit-security <tkr>  Edit a security by ticker
    --ticker <ticker>    New ticker symbol
    --name <name>        New security name
    --type <type>        New security type
    --asset-class <cls>  New asset class
    --currency <code>    New currency code
    --exchange <name>    New exchange name

  --delete-security <tkr> Delete a security (fails if history exists)
  (Use 'tmoney security hide <tkr>' or 'tmoney security unhide <tkr>'.)

For more information, visit: https://github.com/haskovec/tmoney`)
}
