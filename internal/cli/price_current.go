package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
)

// runCurrentPrice shows the most recent price for a security.
func runCurrentPrice(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--current-price requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--current-price requires --ticker to specify a security")
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

	asOf := types.Today()
	p, err := svc.Price.GetCurrentPrice(sec.ID, asOf)
	if err != nil {
		return fmt.Errorf("no price found for %s", sec.Ticker)
	}

	fmt.Fprintf(w, "CURRENT PRICE: %s\n", sec.Ticker)
	fmt.Fprintf(w, "Ticker:  %s\n", sec.Ticker)
	fmt.Fprintf(w, "Name:    %s\n", sec.Name)
	fmt.Fprintf(w, "Date:    %s\n", p.Date.String())
	fmt.Fprintf(w, "Price:   %s\n", formatMoney(p.Price, sec.Currency))
	fmt.Fprintf(w, "Source:  %s\n", p.Source.DisplayName())

	return nil
}
