package report

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
)

// printNetWorthReport prints the net worth report.
func printNetWorthReport(w io.Writer, rpt *reportdom.NetWorth) {
	fmt.Fprintln(w, "NET WORTH REPORT")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n\n", rpt.AsOfDate.Format("January 2, 2006"))

	// Assets section
	fmt.Fprintln(w, "ASSETS")
	fmt.Fprintln(w, "------")
	if len(rpt.Assets) == 0 {
		fmt.Fprintln(w, "  (No asset accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, ab := range rpt.Assets {
			balStr := cmdutil.FormatMoney(ab.Balance, "USD")
			if ab.EstimatedValue {
				balStr = "~" + balStr
			}
			fmt.Fprintf(tw, "  %s\t%s\n", ab.Name, balStr)
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Assets:\t\t%s\n\n", cmdutil.FormatMoney(rpt.TotalAssets, "USD"))

	// Liabilities section
	fmt.Fprintln(w, "LIABILITIES")
	fmt.Fprintln(w, "-----------")
	if len(rpt.Liabilities) == 0 {
		fmt.Fprintln(w, "  (No liability accounts)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, ab := range rpt.Liabilities {
			balStr := cmdutil.FormatMoney(ab.Balance, "USD")
			if ab.EstimatedValue {
				balStr = "~" + balStr
			}
			fmt.Fprintf(tw, "  %s\t%s\n", ab.Name, balStr)
		}
		tw.Flush()
	}
	fmt.Fprintf(w, "\nTotal Liabilities:\t%s\n\n", cmdutil.FormatMoney(rpt.TotalLiabilities, "USD"))

	// Net worth
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "NET WORTH:\t\t%s\n", cmdutil.FormatMoney(rpt.NetWorth, "USD"))
}

// printSpendingReport prints the spending by category report.
func printSpendingReport(w io.Writer, rpt *reportdom.Spending) {
	fmt.Fprintln(w, "SPENDING BY CATEGORY")
	fmt.Fprintln(w, "====================")
	fmt.Fprintf(w, "Period: %s\n\n", rpt.Period)

	if len(rpt.Categories) == 0 {
		fmt.Fprintln(w, "No spending found for this period.")
		return
	}

	// Print category spending with visual bars
	maxBarWidth := 30
	maxAmount := types.ZeroMoney
	for _, cs := range rpt.Categories {
		if cs.Amount.Cmp(maxAmount) > 0 {
			maxAmount = cs.Amount
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Category\tAmount\t%\tBar")
	fmt.Fprintln(tw, "--------\t------\t-\t---")

	for _, cs := range rpt.Categories {
		// Calculate bar length
		barLen := 0
		if !maxAmount.IsZero() {
			barLen = int(cs.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
		}
		bar := strings.Repeat("█", barLen)

		fmt.Fprintf(tw, "%s\t%s\t%.1f%%\t%s\n",
			cs.Name,
			cmdutil.FormatMoney(cs.Amount, "USD"),
			cs.Percentage,
			bar,
		)

		// Print subcategories with indentation
		for _, sub := range cs.Subcategories {
			subBarLen := 0
			if !maxAmount.IsZero() {
				subBarLen = int(sub.Amount.Float64() / maxAmount.Float64() * float64(maxBarWidth))
			}
			subBar := strings.Repeat("░", subBarLen)

			fmt.Fprintf(tw, "  %s\t%s\t%.1f%%\t%s\n",
				sub.Name,
				cmdutil.FormatMoney(sub.Amount, "USD"),
				sub.Percentage,
				subBar,
			)
		}
	}
	tw.Flush()

	fmt.Fprintf(w, "\n------------------------\nTotal Spending:\t%s\n", cmdutil.FormatMoney(rpt.TotalSpending, "USD"))
}
