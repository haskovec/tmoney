package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/spf13/cobra"
)

// priceImportOptions are the inputs to `tmoney price import`.
type priceImportOptions struct {
	file      string
	csvPath   string
	overwrite bool
}

// newPriceImportCmd registers `tmoney price import <file>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--overwrite` causes existing
// prices on matching dates to be replaced instead of skipped.
func newPriceImportCmd() *cobra.Command {
	opts := &priceImportOptions{}
	cmd := &cobra.Command{
		Use:          "import <file>",
		Short:        "Import prices from a CSV file",
		Long:         "Bulk-import prices from a CSV file. The CSV must have Date, Ticker, and Price columns. Existing prices on matching dates are skipped unless --overwrite is set.",
		Example:      "  tmoney price import prices.csv\n  tmoney price import prices.csv --overwrite",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.csvPath = args[0]
			return runPriceImport(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&opts.overwrite, "overwrite", false, "Overwrite existing prices on matching dates")
	return cmd
}

// runPriceImport imports prices from a CSV file.
func runPriceImport(opts *priceImportOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	file, err := os.Open(opts.csvPath)
	if err != nil {
		return fmt.Errorf("failed to open import file: %w", err)
	}
	defer file.Close()

	parseResult, err := imexport.ParsePriceCSV(file)
	if err != nil {
		return fmt.Errorf("failed to parse price CSV: %w", err)
	}

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

	result, err := svc.Price.BulkImport(prices, opts.overwrite)
	if err != nil {
		return fmt.Errorf("failed to import prices: %w", err)
	}

	fmt.Fprintf(w, "IMPORT COMPLETE: %s\n", filepath.Base(opts.csvPath))
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
