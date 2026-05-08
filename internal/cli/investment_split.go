package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// runSplit executes the --split command: apply a stock split or reverse split.
func runSplit(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--split requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--split requires --ticker to specify a security")
	}
	if opts.splitRatio == "" {
		return fmt.Errorf("--split requires --ratio to specify the split ratio (e.g. 4:1)")
	}

	params, err := investment.ParseSplitRatio(opts.splitRatio)
	if err != nil {
		return fmt.Errorf("invalid --ratio: %w", err)
	}

	var date types.Date
	if opts.txDate != "" {
		date, err = types.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
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
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to apply corporate actions", opts.secTicker)
	}

	action, err := svc.CorporateAction.Split(sec.ID, date, *params)
	if err != nil {
		return fmt.Errorf("failed to apply stock split: %w", err)
	}

	fmt.Fprintln(w, "Stock split applied successfully!")
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Ratio:    %s\n", params.RatioString())
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}
