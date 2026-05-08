package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runListPrices lists prices for a security ticker.
func runListPrices(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--prices requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--prices requires --ticker to specify a security")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.secTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.secTicker)
	}

	var from, to *types.Date
	if opts.fromDate != "" {
		d, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		from = &d
	}
	if opts.toDate != "" {
		d, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		to = &d
	}

	prices, err := svc.Price.GetPriceHistory(sec.ID, from, to)
	if err != nil {
		return fmt.Errorf("failed to get prices: %w", err)
	}

	printPricesTable(w, sec.Ticker, prices)

	return nil
}
