package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

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

// holdingsBarWidth is the width of the bar column in the holdings report.
const holdingsBarWidth = 20

// printHoldingsReport prints the holdings report: one table per currency,
// each with a TOTAL line, then the notes for estimated values and for
// accounts that could not be valued.
func printHoldingsReport(w io.Writer, rpt *reportdom.Holdings) {
	fmt.Fprintln(w, "HOLDINGS REPORT")
	fmt.Fprintln(w, "===============")
	fmt.Fprintf(w, "As of: %s\n", rpt.AsOfDate.Time().Format("January 2, 2006"))
	fmt.Fprintln(w)

	if len(rpt.Sections) == 0 {
		fmt.Fprintln(w, "No investment accounts.")
		return
	}

	estimated := false
	for i, sec := range rpt.Sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		printHoldingsSection(w, sec)
		estimated = estimated || sec.Estimated
	}
	if estimated || len(rpt.Failed) > 0 {
		fmt.Fprintln(w)
	}
	if estimated {
		fmt.Fprintln(w, "~ No price on file: the value is the cost basis.")
	}
	if len(rpt.Failed) > 0 {
		fmt.Fprintln(w, "Percentages leave out accounts that could not be valued.")
	}
}

// printHoldingsSection prints one currency's rows and its TOTAL line.
func printHoldingsSection(w io.Writer, sec reportdom.HoldingsSection) {
	largest := types.ZeroMoney
	if len(sec.Rows) > 0 {
		largest = sec.Rows[0].Value
	}
	// tabwriter pads every cell it aligns, so a row whose last cells are
	// empty would end in spaces; the buffer lets them be cut off.
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Security\tName\tShares\tPrice\tValue\t% Total\tCost Basis\tGain\t")
	for _, row := range sec.Rows {
		label, shares, price, gain := row.Label, "", "", ""
		switch {
		case row.Cash:
		case row.Estimated:
			label = "~" + label
			shares, price, gain = row.Shares.String(), "N/A", "N/A"
		default:
			shares = row.Shares.String()
			price = cmdutil.FormatMoney(row.Price, sec.Currency)
			gain = cmdutil.FormatMoney(row.Gain, sec.Currency)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.1f%%\t%s\t%s\t%s\n",
			label, row.Name, shares, price,
			cmdutil.FormatMoney(row.Value, sec.Currency),
			row.Percent,
			cmdutil.FormatMoney(row.CostBasis, sec.Currency),
			gain,
			strings.TrimRight(reportdom.Bar(row.Value, largest, holdingsBarWidth), " "),
		)
	}
	total := fmt.Sprintf("TOTAL (%s)", sec.Currency)
	if sec.Available {
		fmt.Fprintf(tw, "%s\t\t\t\t%s\t100.0%%\t%s\t%s\t\n", total,
			formatTotal(sec.Value, sec.Currency, true, sec.Estimated),
			cmdutil.FormatMoney(sec.CostBasis, sec.Currency),
			cmdutil.FormatMoney(sec.Gain, sec.Currency))
	} else {
		fmt.Fprintf(tw, "%s\t\t\t\t%s\t\t\t\t\n", total, formatTotal(sec.Value, sec.Currency, false, false))
	}
	tw.Flush()
	for line := range strings.Lines(buf.String()) {
		fmt.Fprintln(w, strings.TrimRight(line, " \n"))
	}
}

// printHoldingSplit prints how one security splits across the accounts, one
// table per currency it is held in. It returns false, and prints nothing,
// when no row holds the security.
func printHoldingSplit(w io.Writer, rpt *reportdom.Holdings, securityID types.ID, ticker, name string) bool {
	type found struct {
		sec reportdom.HoldingsSection
		row reportdom.HoldingRow
	}
	var rows []found
	for _, sec := range rpt.Sections {
		for _, row := range sec.Rows {
			if !row.Cash && row.SecurityID == securityID {
				rows = append(rows, found{sec, row})
			}
		}
	}
	if len(rows) == 0 {
		return false
	}

	heading := "HOLDING: " + cmdutil.SecurityDisplay(ticker, name)
	fmt.Fprintln(w, heading)
	fmt.Fprintln(w, strings.Repeat("=", utf8.RuneCountInString(heading)))
	fmt.Fprintf(w, "As of: %s\n", rpt.AsOfDate.Time().Format("January 2, 2006"))
	ref := cmdutil.SecurityRef(ticker, name)
	for _, f := range rows {
		fmt.Fprintln(w)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Account\tShares\tValue\t% of Holding")
		for _, a := range f.row.Accounts {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f%%\n", a.Name, a.Shares.String(),
				cmdutil.FormatMoney(a.Value, f.sec.Currency), a.Percent)
		}
		fmt.Fprintf(tw, "Total\t%s\t%s\t100.0%%\n", f.row.Shares.String(),
			cmdutil.FormatMoney(f.row.Value, f.sec.Currency))
		tw.Flush()
		fmt.Fprintln(w)
		if f.sec.Available {
			fmt.Fprintf(w, "%s is %.1f%% of all holdings (%s).\n", ref, f.row.Percent,
				formatTotal(f.sec.Value, f.sec.Currency, true, f.sec.Estimated))
		} else {
			fmt.Fprintf(w, "%s is %.1f%% of the holdings that could be valued.\n", ref, f.row.Percent)
		}
	}
	return true
}

// holdingsErrors returns one error naming every account that could not be
// valued, or nil, as netWorthErrors does for net worth.
func holdingsErrors(rpt *reportdom.Holdings) error {
	if len(rpt.Failed) == 0 {
		return nil
	}
	failed := make([]string, len(rpt.Failed))
	for i, f := range rpt.Failed {
		failed[i] = fmt.Sprintf("%s: %v", f.Name, f.Err)
	}
	return fmt.Errorf("could not value %d account(s): %s", len(failed), strings.Join(failed, "; "))
}
