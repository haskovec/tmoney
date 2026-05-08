package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// runAddPrice adds a price for a security.
func runAddPrice(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-price requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--add-price requires --ticker to specify a security")
	}
	if opts.txDate == "" {
		return fmt.Errorf("--add-price requires --date to specify a price date")
	}
	if opts.priceValue == "" {
		return fmt.Errorf("--add-price requires --price to specify a price value")
	}

	priceDate, err := types.ParseDate(opts.txDate)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	priceAmount, err := types.NewMoney(opts.priceValue)
	if err != nil {
		return fmt.Errorf("invalid --price: %w", err)
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

	p := price.NewPrice(sec.ID, priceDate, priceAmount, price.SourceManual)
	if err := svc.Price.AddPrice(p); err != nil {
		return fmt.Errorf("failed to add price: %w", err)
	}

	fmt.Fprintf(w, "Price added for %s on %s: %s\n", sec.Ticker, priceDate.String(), formatMoney(priceAmount, sec.Currency))

	autoBackupAfterModification(opts.file)
	return nil
}
