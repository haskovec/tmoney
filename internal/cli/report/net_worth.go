package report

import (
	"fmt"
	"io"
	"time"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	reportdom "github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reportNetWorthOptions are the inputs to `tmoney report net-worth`.
type reportNetWorthOptions struct {
	file          string
	asOf          string
	includeClosed bool
}

// newReportNetWorthCmd registers `tmoney report net-worth`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newReportNetWorthCmd() *cobra.Command {
	opts := &reportNetWorthOptions{}
	cmd := &cobra.Command{
		Use:   "net-worth",
		Short: "Show a net-worth report (assets vs. liabilities)",
		Long: "Generate a net-worth report summarizing total assets, " +
			"total liabilities, and net worth as of a given date. By " +
			"default closed accounts are excluded; pass `--include-closed` " +
			"to include them.",
		Example: "  tmoney report net-worth\n" +
			"  tmoney report net-worth --as-of 2024-06-30\n" +
			"  tmoney report net-worth --include-closed",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReportNetWorth(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.asOf, "as-of", "", "Valuation date (YYYY-MM-DD); defaults to today")
	cmd.Flags().BoolVar(&opts.includeClosed, "include-closed", false, "Include closed accounts in the report")
	return cmd
}

// runReportNetWorth generates and displays the net-worth report.
func runReportNetWorth(opts *reportNetWorthOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var asOf time.Time
	if opts.asOf != "" {
		d, err := types.ParseDate(opts.asOf)
		if err != nil {
			return fmt.Errorf("invalid --as-of date: %w", err)
		}
		asOf = time.Time(d)
	} else {
		asOf = time.Now()
	}

	var rpt *reportdom.NetWorth
	if opts.includeClosed {
		rpt, err = svc.Report.NetWorthAsOfIncludingClosed(asOf)
	} else {
		rpt, err = svc.Report.NetWorthAsOf(asOf)
	}
	if err != nil {
		return fmt.Errorf("failed to generate net worth report: %w", err)
	}

	printNetWorthReport(w, rpt)
	return nil
}
