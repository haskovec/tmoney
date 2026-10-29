package report

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reportAllocationOptions are the inputs to `tmoney report allocation`.
type reportAllocationOptions struct {
	file       string
	assetClass string
}

// newReportAllocationCmd registers `tmoney report allocation`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from the
// root command.
func newReportAllocationCmd() *cobra.Command {
	opts := &reportAllocationOptions{}
	cmd := &cobra.Command{
		Use:   "allocation",
		Short: "Show the holdings of all investment accounts by asset class",
		Long: "Generate an asset allocation report: the holdings of the active " +
			"investment accounts, grouped by the asset class of each security, " +
			"with each class's percentage of the total. The Cash class holds the " +
			"accounts' uninvested cash and each security of class cash. A " +
			"holding with no price is left out, and a note names it. Pass " +
			"`--asset-class` to show the holdings in one class.",
		Example: "  tmoney report allocation\n" +
			"  tmoney report allocation --asset-class mid_cap_stock",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReportAllocation(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.assetClass, "asset-class", "", "Show the holdings in one asset class (e.g. large_cap_stock)")
	return cmd
}

// runReportAllocation generates and displays the allocation report, or the
// holdings of one class when --asset-class is given.
func runReportAllocation(opts *reportAllocationOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	// Parse the class first: a bad class fails before every account is
	// valued.
	var class security.AssetClass
	if opts.assetClass != "" {
		var err error
		if class, err = security.ParseAssetClass(opts.assetClass); err != nil {
			return fmt.Errorf("invalid --asset-class: %w", err)
		}
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()
	return reportAllocation(w, svc, class)
}

// reportAllocation is runReportAllocation on open services. class is "" for
// the full report.
func reportAllocation(w io.Writer, svc *app.Services, class security.AssetClass) error {
	rpt, err := svc.Report.Allocation()
	if err != nil {
		return fmt.Errorf("failed to generate allocation report: %w", err)
	}
	if class == "" {
		printAllocationReport(w, rpt)
	} else if !printAllocationClass(w, rpt, class) {
		return notInClassError(rpt, class)
	}
	return failedAccountsError(rpt.Failed)
}

// allocationBarWidth is the width of the bar column in the allocation report.
const allocationBarWidth = 20

// printAllocationReport prints the allocation report: one table per
// currency, each with its TOTAL line and its notes.
func printAllocationReport(w io.Writer, rpt *reportdom.Allocation) {
	fmt.Fprintln(w, "ASSET ALLOCATION")
	fmt.Fprintln(w, "================")
	fmt.Fprintf(w, "As of: %s\n", rpt.AsOfDate.Time().Format("January 2, 2006"))
	fmt.Fprintln(w)

	if len(rpt.Sections) == 0 {
		if rpt.AccountCount == 0 {
			fmt.Fprintln(w, "No investment accounts.")
		} else {
			fmt.Fprintln(w, "No holdings in the investment accounts.")
		}
		return
	}
	for i, sec := range rpt.Sections {
		if i > 0 {
			fmt.Fprintln(w)
		}
		printAllocationSection(w, sec)
	}
}

// printAllocationSection prints one currency's classes, its TOTAL line, and
// its notes.
func printAllocationSection(w io.Writer, sec reportdom.AllocationSection) {
	money := func(m types.Money) string { return cmdutil.FormatMoney(m, sec.Currency) }
	shown := sec.Shown()
	largest := types.ZeroMoney
	for _, c := range shown {
		if c.Value.Cmp(largest) > 0 {
			largest = c.Value
		}
	}
	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Asset Class\tHoldings\tValue\t% Total\t")
	for _, c := range shown {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%.1f%%\t%s\n",
			c.Class.DisplayName(), c.Count(), money(c.Value), c.Percent,
			strings.TrimRight(reportdom.Bar(c.Value, largest, allocationBarWidth), " "))
	}
	total := fmt.Sprintf("TOTAL (%s)", sec.Currency)
	if sec.Available {
		// The classes are 0% of a total that is zero or less (negative
		// cash), so the total is not 100% of them either.
		pct := ""
		if sec.Value.IsPositive() {
			pct = "100.0%"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t\n", total, sec.Count(), money(sec.Value), pct)
	} else {
		fmt.Fprintf(tw, "%s\t\t%s\t\t\n", total, formatTotal(sec.Value, sec.Currency, false, false))
	}
	tw.Flush()
	printTrimmed(w, buf.String())

	var notes []string
	if !sec.Available {
		notes = append(notes, "Percentages leave out accounts that could not be valued.")
	}
	if note := reportdom.NoPriceNote(sec.NoPriceNames()); note != "" {
		notes = append(notes, note)
	}
	if c, ok := sec.Class(security.AssetClassUnclassified); ok {
		if note := reportdom.UnclassifiedNote(c.Count()); note != "" {
			notes = append(notes, note+" Set one with: tmoney security edit <ticker> --asset-class <class>")
		}
	}
	if len(notes) > 0 {
		fmt.Fprintln(w)
		for _, n := range notes {
			fmt.Fprintln(w, n)
		}
	}
}

// printAllocationClass prints the holdings in one asset class, one table per
// currency that has them. It returns false, and prints nothing, when no
// section has a holding in the class.
func printAllocationClass(w io.Writer, rpt *reportdom.Allocation, ac security.AssetClass) bool {
	type found struct {
		sec   reportdom.AllocationSection
		class reportdom.ClassRow
	}
	var blocks []found
	for _, sec := range rpt.Sections {
		if c, ok := sec.Class(ac); ok {
			blocks = append(blocks, found{sec, c})
		}
	}
	if len(blocks) == 0 {
		return false
	}

	heading := "ASSET CLASS: " + ac.DisplayName()
	fmt.Fprintln(w, heading)
	fmt.Fprintln(w, strings.Repeat("=", utf8.RuneCountInString(heading)))
	fmt.Fprintf(w, "As of: %s\n", rpt.AsOfDate.Time().Format("January 2, 2006"))
	for _, b := range blocks {
		fmt.Fprintln(w)
		money := func(m types.Money) string { return cmdutil.FormatMoney(m, b.sec.Currency) }
		var buf bytes.Buffer
		tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Security\tName\tValue\t% of Class\t% Total")
		for _, h := range b.class.Holdings {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%.1f%%\t%.1f%%\n",
				h.Label, h.Name, money(h.Value), h.PercentOfClass, h.PercentOfTotal)
		}
		for _, row := range b.class.NoPrice {
			fmt.Fprintf(tw, "%s\t%s\tno price\t\t\n", row.Label, row.Name)
		}
		if b.sec.Available {
			pct := ""
			if b.class.Value.IsPositive() {
				pct = "100.0%"
			}
			fmt.Fprintf(tw, "Total\t\t%s\t%s\t%.1f%%\n", money(b.class.Value), pct, b.class.Percent)
		} else {
			fmt.Fprintf(tw, "Total\t\t%s\t\t\n", formatTotal(b.class.Value, b.sec.Currency, false, false))
		}
		tw.Flush()
		printTrimmed(w, buf.String())

		var notes []string
		if !b.sec.Available {
			notes = append(notes, "Percentages leave out accounts that could not be valued.")
		}
		labels := make([]string, len(b.class.NoPrice))
		for i, row := range b.class.NoPrice {
			labels[i] = row.Label
		}
		if note := reportdom.NoPriceNote(labels); note != "" {
			notes = append(notes, note)
		}
		if len(notes) > 0 {
			fmt.Fprintln(w)
			for _, n := range notes {
				fmt.Fprintln(w, n)
			}
		}
	}
	return true
}

// notInClassError is the error for a class that has no holding (C12). When
// an account could not be valued the class can have holdings in it, so the
// error names the failed accounts.
func notInClassError(rpt *reportdom.Allocation, ac security.AssetClass) error {
	if err := failedAccountsError(rpt.Failed); err != nil {
		return fmt.Errorf("no holdings in %s in the accounts that could be valued: %w", ac.DisplayName(), err)
	}
	return fmt.Errorf("no holdings in %s", ac.DisplayName())
}

// printTrimmed prints tabwriter output with the spaces at the end of each
// line cut off: tabwriter pads every cell it aligns, so a line whose last
// cells are empty would end in spaces.
func printTrimmed(w io.Writer, s string) {
	for line := range strings.Lines(s) {
		fmt.Fprintln(w, strings.TrimRight(line, " \n"))
	}
}
