package cli

import (
	"fmt"
	"io"
)

// runUnhideSecurity unhides a security.
func runUnhideSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--unhide-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.unhideSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.unhideSecurity)
	}

	if err := svc.Security.Unhide(sec.ID); err != nil {
		return fmt.Errorf("failed to unhide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) unhidden successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}
