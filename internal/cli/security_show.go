package cli

import (
	"fmt"
	"io"
)

// runSecurityDetail shows detailed information for a specific security.
func runSecurityDetail(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--security requires --file to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.securityTicker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.securityTicker)
	}

	printSecurityDetails(w, sec)

	return nil
}
