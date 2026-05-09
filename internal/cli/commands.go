package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
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
