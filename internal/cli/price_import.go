package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/price"
)

// runImportPrices imports prices from a CSV file.
func runImportPrices(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--import-prices requires --file to specify a database")
	}

	file, err := os.Open(opts.importPrices)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()

	// Parse the CSV
	parseResult, err := imexport.ParsePriceCSV(file)
	if err != nil {
		return fmt.Errorf("failed to parse price CSV: %w", err)
	}

	// Report parse errors
	if parseResult.HasErrors() {
		for _, e := range parseResult.Errors {
			fmt.Fprintf(w, "  Warning: %s\n", e.Error())
		}
	}

	if len(parseResult.Records) == 0 {
		fmt.Fprintln(w, "No valid price records found in CSV file.")
		return nil
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Resolve tickers to security IDs and build price objects
	var prices []*price.Price
	tickerErrors := 0
	hiddenSkipped := 0
	for _, rec := range parseResult.Records {
		sec, secErr := svc.Security.GetByTicker(rec.Ticker, "")
		if secErr != nil {
			fmt.Fprintf(w, "  Warning: line %d: unknown ticker %q\n", rec.SourceLine, rec.Ticker)
			tickerErrors++
			continue
		}
		if sec.Hidden {
			fmt.Fprintf(w, "  Warning: line %d: skipping hidden security %q\n", rec.SourceLine, rec.Ticker)
			hiddenSkipped++
			continue
		}

		p := price.NewPrice(sec.ID, rec.Date, rec.Price, price.SourceImport)
		prices = append(prices, p)
	}

	if len(prices) == 0 {
		fmt.Fprintln(w, "No prices to import after resolving tickers.")
		return nil
	}

	// Import prices
	result, err := svc.Price.BulkImport(prices, opts.overwrite)
	if err != nil {
		return fmt.Errorf("failed to import prices: %w", err)
	}

	// Display summary
	fmt.Fprintf(w, "IMPORT COMPLETE: %s\n", filepath.Base(opts.importPrices))
	fmt.Fprintf(w, "  Total rows:     %d\n", result.Total+tickerErrors+len(parseResult.Errors))
	fmt.Fprintf(w, "  Imported:       %d\n", result.Imported)
	fmt.Fprintf(w, "  Skipped:        %d\n", result.Skipped)
	if tickerErrors > 0 {
		fmt.Fprintf(w, "  Unknown ticker: %d\n", tickerErrors)
	}
	if len(parseResult.Errors) > 0 {
		fmt.Fprintf(w, "  Parse errors:   %d\n", len(parseResult.Errors))
	}

	autoBackupAfterModification(opts.file)
	return nil
}
