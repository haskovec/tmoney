package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
)

// runAddSecurity creates a new security.
func runAddSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-security requires --file to specify a database")
	}
	if opts.secTicker == "" {
		return fmt.Errorf("--add-security requires --ticker to specify a ticker symbol")
	}
	if opts.acctName == "" {
		return fmt.Errorf("--add-security requires --name to specify a security name")
	}
	if opts.acctType == "" {
		return fmt.Errorf("--add-security requires --type to specify a security type (stock, etf, mutual_fund, other)")
	}

	secType, err := security.ParseType(opts.acctType)
	if err != nil {
		return fmt.Errorf("invalid --type: %w", err)
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec := security.NewSecurity(opts.secTicker, opts.acctName, secType)

	if opts.secAssetClass != "" {
		ac, err := security.ParseAssetClass(opts.secAssetClass)
		if err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
		sec.AssetClass = ac
	}

	if opts.acctCurrency != "" {
		sec.Currency = opts.acctCurrency
	}

	if opts.secExchange != "" {
		sec.SetExchange(opts.secExchange)
	}

	if err := svc.Security.Create(sec); err != nil {
		return fmt.Errorf("failed to create security: %w", err)
	}

	fmt.Fprintln(w, "Security created successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)
	if sec.Exchange.Valid {
		fmt.Fprintf(w, "  Exchange:    %s\n", sec.Exchange.String)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
