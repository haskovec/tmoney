package cli

import (
	"fmt"
	"io"
)

// runHideSecurity hides a security.
func runHideSecurity(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--hide-security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.hideSecurity, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.hideSecurity)
	}

	if err := svc.Security.Hide(sec.ID); err != nil {
		return fmt.Errorf("failed to hide security: %w", err)
	}

	fmt.Fprintf(w, "Security %s (%s) hidden successfully.\n", sec.Ticker, sec.Name)

	autoBackupAfterModification(opts.file)
	return nil
}
