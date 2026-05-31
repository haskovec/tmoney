package security

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	securitydom "github.com/haskovec/tmoney/internal/security"
)

// printSecuritiesTable prints securities in a formatted table.
func printSecuritiesTable(w io.Writer, securities []*securitydom.Security) {
	if len(securities) == 0 {
		fmt.Fprintln(w, "No securities found.")
		return
	}

	fmt.Fprintln(w, "SECURITIES")
	fmt.Fprintln(w, "==========")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Ticker\tName\tType\tAsset Class\tCurrency")
	fmt.Fprintln(tw, "------\t----\t----\t-----------\t--------")

	for _, sec := range securities {
		hidden := ""
		if sec.Hidden {
			hidden = " [hidden]"
		}

		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\n",
			sec.Ticker,
			hidden,
			sec.Name,
			sec.SecurityType.DisplayName(),
			sec.AssetClass.DisplayName(),
			sec.Currency,
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nShowing %d security(ies)\n", len(securities))
}

// printSecurityDetails prints detailed information for a single security.
func printSecurityDetails(w io.Writer, sec *securitydom.Security) {
	fmt.Fprintf(w, "SECURITY: %s\n", sec.Ticker)
	fmt.Fprintln(w, strings.Repeat("=", len("SECURITY: ")+len(sec.Ticker)))

	fmt.Fprintf(w, "Ticker:      %s\n", sec.Ticker)
	fmt.Fprintf(w, "Name:        %s\n", sec.Name)
	fmt.Fprintf(w, "Type:        %s\n", sec.SecurityType.DisplayName())
	fmt.Fprintf(w, "Asset Class: %s\n", sec.AssetClass.DisplayName())
	fmt.Fprintf(w, "Currency:    %s\n", sec.Currency)

	if sec.Exchange.Valid {
		fmt.Fprintf(w, "Exchange:    %s\n", sec.Exchange.String)
	}

	if sec.Hidden {
		fmt.Fprintf(w, "Status:      Hidden\n")
	} else {
		fmt.Fprintf(w, "Status:      Active\n")
	}
}
