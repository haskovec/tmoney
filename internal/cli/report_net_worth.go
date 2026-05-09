package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

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
