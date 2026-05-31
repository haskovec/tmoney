package price

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	pricedom "github.com/haskovec/tmoney/internal/price"
)

// printPricesTable prints prices in a formatted table.
func printPricesTable(w io.Writer, ticker string, prices []*pricedom.Price) {
	if len(prices) == 0 {
		fmt.Fprintf(w, "No prices found for %s.\n", ticker)
		return
	}

	fmt.Fprintf(w, "PRICES: %s\n", ticker)
	fmt.Fprintln(w, strings.Repeat("=", len("PRICES: ")+len(ticker)))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Date\tPrice\tSource")
	fmt.Fprintln(tw, "----\t-----\t------")

	for _, p := range prices {
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			p.Date.String(),
			fmt.Sprintf("%.2f", p.Price.Float64()),
			p.Source.DisplayName(),
		)
	}

	tw.Flush()

	fmt.Fprintf(w, "\nTotal: %d price(s)\n", len(prices))
}
