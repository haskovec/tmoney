package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/report"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// reportSpendingOptions are the inputs to `tmoney report spending`.
type reportSpendingOptions struct {
	file     string
	month    string
	year     int
	fromDate string
	toDate   string
}

// newReportSpendingCmd registers `tmoney report spending`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Exactly one period flag must be
// provided: `--month`, `--year`, or `--from` (with optional `--to`).
func newReportSpendingCmd() *cobra.Command {
	opts := &reportSpendingOptions{}
	cmd := &cobra.Command{
		Use:   "spending",
		Short: "Show a spending-by-category report",
		Long: "Generate a spending-by-category report for a given " +
			"period. The period must be specified with `--month YYYY-MM`, " +
			"`--year YYYY`, or a `--from`/`--to` date range.",
		Example: "  tmoney report spending --month 2024-03\n" +
			"  tmoney report spending --year 2024\n" +
			"  tmoney report spending --from 2024-01-01 --to 2024-06-30",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runReportSpending(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.month, "month", "", "Period as YYYY-MM (e.g. 2024-03)")
	cmd.Flags().IntVar(&opts.year, "year", 0, "Period as YYYY (e.g. 2024)")
	cmd.Flags().StringVar(&opts.fromDate, "from", "", "Start of custom date range (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.toDate, "to", "", "End of custom date range (YYYY-MM-DD); defaults to today")
	return cmd
}

// runReportSpending generates and displays the spending-by-category report.
func runReportSpending(opts *reportSpendingOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}
	if opts.month == "" && opts.year == 0 && opts.fromDate == "" {
		return fmt.Errorf("report spending requires --month YYYY-MM, --year YYYY, or --from/--to date range")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	var rpt *report.Spending
	switch {
	case opts.month != "":
		year, month, err := parseYearMonth(opts.month)
		if err != nil {
			return fmt.Errorf("invalid --month format: %w", err)
		}
		rpt, err = svc.Report.SpendingByCategoryMonth(year, month)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	case opts.year != 0:
		rpt, err = svc.Report.SpendingByCategoryYear(opts.year)
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	default:
		startDate, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}

		var endDate types.Date
		if opts.toDate != "" {
			endDate, err = types.ParseDate(opts.toDate)
			if err != nil {
				return fmt.Errorf("invalid --to date: %w", err)
			}
		} else {
			endDate = types.Today()
		}

		rpt, err = svc.Report.SpendingByCategoryDateRange(time.Time(startDate), time.Time(endDate))
		if err != nil {
			return fmt.Errorf("failed to generate spending report: %w", err)
		}
	}

	printSpendingReport(w, rpt)
	return nil
}

// parseYearMonth parses a YYYY-MM string into year and month integers.
func parseYearMonth(s string) (int, int, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected YYYY-MM format, got %q", s)
	}

	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid year: %w", err)
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid month: %w", err)
	}

	if month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("month must be between 1 and 12, got %d", month)
	}

	return year, month, nil
}
