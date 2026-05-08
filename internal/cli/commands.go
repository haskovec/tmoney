package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/report"
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
