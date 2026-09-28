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

// printNetWorthReport prints the net worth report. Each row is in its own
// currency, and the totals come one per currency: money in different
// currencies is never added. A row that could not be valued prints "error",
// and its currency's total prints "not available" rather than a sum that
// leaves the row out.
func printNetWorthReport(w io.Writer, rpt *reportdom.NetWorth) {
	fmt.Fprintln(w, "NET WORTH REPORT")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n", rpt.AsOfDate.Format("January 2, 2006"))
	if rpt.InvestmentAsOfApproximate {
		fmt.Fprintln(w, "Investment accounts show current cash and shares, priced as of this date.")
	}
	fmt.Fprintln(w)

	// Assets section
	fmt.Fprintln(w, "ASSETS")
	fmt.Fprintln(w, "------")
	printNetWorthRows(w, rpt.Assets, "  (No asset accounts)")
	fmt.Fprintln(w)
	for _, t := range rpt.Totals {
		fmt.Fprintf(w, "Total Assets (%s):\t%s\n", t.Currency, formatTotal(t.Assets, t.Currency, t.AssetsAvailable, t.Estimated))
	}
	fmt.Fprintln(w)

	// Liabilities section. Liability balances are stored signed (negative =
	// owed); under the LIABILITIES heading they render the raw signed balance,
	// so a debt shows negative and a credit / paid-ahead card shows positive.
	fmt.Fprintln(w, "LIABILITIES")
	fmt.Fprintln(w, "-----------")
	printNetWorthRows(w, rpt.Liabilities, "  (No liability accounts)")
	fmt.Fprintln(w)
	for _, t := range rpt.Totals {
		fmt.Fprintf(w, "Total Liabilities (%s):\t%s\n", t.Currency, formatTotal(t.Liabilities, t.Currency, t.LiabilitiesAvailable, false))
	}
	fmt.Fprintln(w)

	// Net worth
	fmt.Fprintln(w, "========================")
	if len(rpt.Totals) == 0 {
		fmt.Fprintln(w, "NET WORTH:\t(no accounts)")
	}
	for _, t := range rpt.Totals {
		fmt.Fprintf(w, "NET WORTH (%s):\t%s\n", t.Currency, formatTotal(t.NetWorth, t.Currency, t.Available, t.Estimated))
	}
}

// printNetWorthRows prints one line per account, or empty when there are none.
func printNetWorthRows(w io.Writer, rows []reportdom.AccountBalance, empty string) {
	if len(rows) == 0 {
		fmt.Fprintln(w, empty)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, ab := range rows {
		balStr := "error"
		if ab.Err == nil {
			balStr = cmdutil.FormatMoney(ab.Balance, ab.Currency)
			if ab.EstimatedValue {
				balStr = "~" + balStr
			}
		}
		fmt.Fprintf(tw, "  %s\t%s\n", ab.Name, balStr)
	}
	tw.Flush()
}

// formatTotal renders one currency's total, or "not available" when a row
// it would include could not be valued.
func formatTotal(m types.Money, currency string, available, estimated bool) string {
	if !available {
		return "not available"
	}
	s := cmdutil.FormatMoney(m, currency)
	if estimated {
		s = "~" + s
	}
	return s
}

// netWorthErrors returns one error naming every account that could not be
// valued, or nil. The command prints the report first and then returns it, so
// it exits non-zero and the reasons reach stderr.
func netWorthErrors(rpt *reportdom.NetWorth) error {
	var failed []string
	for _, rows := range [][]reportdom.AccountBalance{rpt.Assets, rpt.Liabilities} {
		for _, ab := range rows {
			if ab.Err != nil {
				failed = append(failed, fmt.Sprintf("%s: %v", ab.Name, ab.Err))
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return fmt.Errorf("could not value %d account(s): %s", len(failed), strings.Join(failed, "; "))
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
