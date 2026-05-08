package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
)

// runEditSecurity edits an existing security.
func runEditSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--edit-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.editSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.editSecurity)
	}

	// Apply changes
	if opts.secTicker != "" {
		sec.Ticker = opts.secTicker
	}
	if opts.acctName != "" {
		sec.Name = opts.acctName
	}
	if opts.acctType != "" {
		secType, err := security.ParseType(opts.acctType)
		if err != nil {
			return fmt.Errorf("invalid --type: %w", err)
		}
		sec.SecurityType = secType
	}
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

	if err := svc.Security.Update(sec); err != nil {
		return fmt.Errorf("failed to update security: %w", err)
	}

	fmt.Fprintln(w, "Security updated successfully!")
	fmt.Fprintf(w, "  Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "  Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "  Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "  Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "  Currency:    %s\n", sec.Currency)

	autoBackupAfterModification(opts.file)
	return nil
}
