package cli

import (
	"fmt"
	"io"
	"strconv"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// runSpinOff executes the --spin-off command: apply a corporate spin-off.
func runSpinOff(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--spin-off requires --file to specify a database")
	}
	if opts.spinOffParent == "" {
		return fmt.Errorf("--spin-off requires --parent to specify the parent security ticker")
	}
	if opts.spinOffChild == "" {
		return fmt.Errorf("--spin-off requires --spinoff to specify the spin-off security ticker")
	}
	if opts.shareRatio == "" {
		return fmt.Errorf("--spin-off requires --share-ratio to specify the share ratio")
	}
	if opts.parentAllocation == "" {
		return fmt.Errorf("--spin-off requires --parent-allocation to specify the parent cost basis allocation percentage")
	}
	if opts.spinOffPrice == "" {
		return fmt.Errorf("--spin-off requires --spin-off-price to specify the spin-off security price")
	}

	shareRatio, err := strconv.ParseFloat(opts.shareRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --share-ratio: %w", err)
	}

	parentAlloc, err := strconv.ParseFloat(opts.parentAllocation, 64)
	if err != nil {
		return fmt.Errorf("invalid --parent-allocation: %w", err)
	}

	spinPrice, err := types.NewMoney(opts.spinOffPrice)
	if err != nil {
		return fmt.Errorf("invalid --spin-off-price: %w", err)
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

	params := investment.SpinOffParams{
		ShareRatio:          shareRatio,
		ParentAllocationPct: parentAlloc,
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	parentSec, err := svc.Security.GetByTicker(opts.spinOffParent, "")
	if err != nil {
		return fmt.Errorf("parent security %q not found", opts.spinOffParent)
	}
	if parentSec.Hidden {
		return fmt.Errorf("parent security %q is hidden; unhide it first to apply corporate actions", opts.spinOffParent)
	}

	childSec, err := svc.Security.GetByTicker(opts.spinOffChild, "")
	if err != nil {
		return fmt.Errorf("spin-off security %q not found", opts.spinOffChild)
	}
	if childSec.Hidden {
		return fmt.Errorf("spin-off security %q is hidden; unhide it first to apply corporate actions", opts.spinOffChild)
	}

	action, err := svc.CorporateAction.SpinOff(parentSec.ID, childSec.ID, date, params, spinPrice)
	if err != nil {
		return fmt.Errorf("failed to apply spin-off: %w", err)
	}

	fmt.Fprintln(w, "Spin-off applied successfully!")
	fmt.Fprintf(w, "  Parent:   %s (%s)\n", parentSec.Ticker, parentSec.Name)
	fmt.Fprintf(w, "  Spin-off: %s (%s)\n", childSec.Ticker, childSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Share Ratio: %s\n", opts.shareRatio)
	fmt.Fprintf(w, "  Parent Allocation: %s%%\n", opts.parentAllocation)
	fmt.Fprintf(w, "  Spin-off Price: %s\n", formatMoney(spinPrice, "USD"))
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	autoBackupAfterModification(opts.file)
	return nil
}
