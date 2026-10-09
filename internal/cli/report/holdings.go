package report

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/spf13/cobra"
)

// reportHoldingsOptions are the inputs to `tmoney report holdings`.
type reportHoldingsOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// selected reports whether a security selector was given.
func (o *reportHoldingsOptions) selected() bool {
	return o.ticker != "" || o.isin != "" || o.name != ""
}

// newReportHoldingsCmd registers `tmoney report holdings`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the root
// command.
func newReportHoldingsCmd() *cobra.Command {
	opts := &reportHoldingsOptions{}
	cmd := &cobra.Command{
		Use:   "holdings",
		Short: "Show the holdings of all investment accounts added up",
		Long: "Generate a holdings report: each security held in the active " +
			"investment accounts, added up across the accounts, with its " +
			"percentage of the total. One Cash row holds the accounts' " +
			"uninvested cash, so the total is the investment value in net " +
			"worth. Pass a security with `--ticker`, `--isin` or `--name` to " +
			"show how it splits across the accounts.",
		Example: "  tmoney report holdings\n" +
			"  tmoney report holdings --ticker ACME\n" +
			"  tmoney report holdings --name \"Cedar 2045 Target Fund\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReportHoldings(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Show how one security splits across the accounts (or use --isin / --name)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runReportHoldings generates and displays the holdings report, or the split
// of one security when a selector is given.
func runReportHoldings(opts *reportHoldingsOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	if opts.selected() {
		sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
		if err != nil {
			return err
		}
		rpt, err := svc.Report.Holdings()
		if err != nil {
			return fmt.Errorf("failed to generate holdings report: %w", err)
		}
		if !printHoldingSplit(w, rpt, sec.ID, sec.Ticker, sec.Name) {
			return fmt.Errorf("%s is not held in an active investment account",
				cmdutil.SecurityRef(sec.Ticker, sec.Name))
		}
		return holdingsErrors(rpt)
	}

	rpt, err := svc.Report.Holdings()
	if err != nil {
		return fmt.Errorf("failed to generate holdings report: %w", err)
	}
	printHoldingsReport(w, rpt)
	return holdingsErrors(rpt)
}
