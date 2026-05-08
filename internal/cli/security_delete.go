package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/security"
)

// runDeleteSecurity deletes a security.
func runDeleteSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--delete-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.deleteSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.deleteSecurity)
	}

	if err := svc.Security.Delete(sec.ID); err != nil {
		if depErr, ok := err.(*security.HasDependentsError); ok {
			return fmt.Errorf("%s\nUse --hide-security %s instead", depErr.Error(), sec.Ticker)
		}
		return fmt.Errorf("failed to delete security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) deleted successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}
