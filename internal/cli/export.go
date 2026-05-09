package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/types"
)

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
