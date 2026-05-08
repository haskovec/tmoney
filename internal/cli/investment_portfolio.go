package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// runPortfolio executes the --portfolio command: show investment portfolio for an account.
func runPortfolio(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--portfolio requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--portfolio requires --account to specify an investment account")
	}

	// Parse optional as-of date (defaults to today)
	var asOf types.Date
	if opts.reportAsOf != "" {
		var err error
		asOf, err = types.ParseDate(opts.reportAsOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
	} else {
		asOf = types.Today()
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	valuation, err := svc.Investment.GetAccountValuation(acct.ID, asOf)
	if err != nil {
		return fmt.Errorf("failed to get portfolio valuation: %w", err)
	}

	// Build security lookup for display
	securityMap := make(map[types.ID]*security.Security)
	for _, h := range valuation.Holdings {
		sec, secErr := svc.Security.GetByID(h.SecurityID)
		if secErr == nil {
			securityMap[h.SecurityID] = sec
		}
	}

	if opts.showLots && acct.TrackLots {
		printPortfolioWithLots(w, acct, valuation, securityMap, svc, asOf)
	} else {
		printPortfolioSummary(w, acct, valuation, securityMap)
	}

	return nil
}
