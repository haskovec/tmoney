package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
)

// runListSecurities lists securities from the database.
func runListSecurities(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--list-securities requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	filter := security.Filter{}
	if !opts.includeHidden {
		excludeHidden := true
		filter.ExcludeHidden = &excludeHidden
	}
	if opts.acctType != "" {
		secType, err := security.ParseType(opts.acctType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		filter.SecurityType = &secType
	}
	if opts.secAssetClass != "" {
		ac, err := security.ParseAssetClass(opts.secAssetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		filter.AssetClass = &ac
	}

	securities, err := svc.Security.List(filter)
	if err != nil {
		return fmt.Errorf("failed to list securities: %w", err)
	}

	printSecuritiesTable(w, securities)

	return nil
}
