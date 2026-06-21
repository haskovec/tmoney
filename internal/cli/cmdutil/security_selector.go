package cmdutil

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AddSecuritySelectorFlags registers the --isin and --name flags used to
// identify a security that may have no ticker. They are alternatives to the
// ticker (supplied as a positional argument or a --ticker flag, depending on
// the command). Resolution is done by the security service's Resolve method,
// which requires exactly one selector and reports ambiguity.
func AddSecuritySelectorFlags(cmd *cobra.Command, isin, name *string) {
	cmd.Flags().StringVar(isin, "isin", "", "Identify the security by ISIN instead of ticker")
	cmd.Flags().StringVar(name, "name", "", "Identify the security by exact name instead of ticker")
}

// SecurityRef returns a short identifier for messages: the ticker if present,
// otherwise the name. Tickerless securities would otherwise print a blank %q.
func SecurityRef(ticker, name string) string {
	if ticker != "" {
		return ticker
	}
	return name
}

// SecurityDisplay returns "TICKER (Name)" when a ticker is present, otherwise
// just the name. Used for confirmation output that would otherwise read
// " (Name)" for a tickerless security.
func SecurityDisplay(ticker, name string) string {
	if ticker != "" {
		return fmt.Sprintf("%s (%s)", ticker, name)
	}
	return name
}
