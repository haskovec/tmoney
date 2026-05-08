package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// runMergeSecurity executes the --merge-security command: apply a merger/acquisition.
func runMergeSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--merge-security requires --file to specify a database")
	}
	if opts.mergeSource == "" {
		return fmt.Errorf("--merge-security requires --source to specify the source security ticker")
	}
	if opts.mergeTarget == "" {
		return fmt.Errorf("--merge-security requires --target to specify the target security ticker")
	}
	if opts.exchangeRatio == "" {
		return fmt.Errorf("--merge-security requires --exchange-ratio to specify the exchange ratio")
	}

	ratio, err := strconv.ParseFloat(opts.exchangeRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --exchange-ratio: %w", err)
	}

	var cashPerShare float64
	if opts.cashPerShare != "" {
		cashPerShare, err = strconv.ParseFloat(opts.cashPerShare, 64)
		if err != nil {
			return fmt.Errorf("invalid --cash-per-share: %w", err)
		}
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

	params := investment.MergerParams{
		ExchangeRatio: ratio,
		CashPerShare:  cashPerShare,
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sourceSec, err := svc.Security.GetByTicker(opts.mergeSource, "")
	if err != nil {
		return fmt.Errorf("source security %q not found", opts.mergeSource)
	}
	if sourceSec.Hidden {
		return fmt.Errorf("source security %q is hidden; unhide it first to apply corporate actions", opts.mergeSource)
	}

	targetSec, err := svc.Security.GetByTicker(opts.mergeTarget, "")
	if err != nil {
		return fmt.Errorf("target security %q not found", opts.mergeTarget)
	}
	if targetSec.Hidden {
		return fmt.Errorf("target security %q is hidden; unhide it first to apply corporate actions", opts.mergeTarget)
	}

	action, err := svc.CorporateAction.Merger(sourceSec.ID, targetSec.ID, date, params)
	if err != nil {
		return fmt.Errorf("failed to apply merger: %w", err)
	}

	fmt.Fprintln(w, "Merger applied successfully!")
	fmt.Fprintf(w, "  Source:   %s (%s)\n", sourceSec.Ticker, sourceSec.Name)
	fmt.Fprintf(w, "  Target:   %s (%s)\n", targetSec.Ticker, targetSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Exchange Ratio: %s\n", opts.exchangeRatio)
	if params.HasCashConsideration() {
		fmt.Fprintf(w, "  Cash/Share: $%.2f\n", cashPerShare)
	}
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}
