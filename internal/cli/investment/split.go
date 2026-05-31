package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentSplitOptions are the inputs to `tmoney investment split`.
type investmentSplitOptions struct {
	file   string
	ticker string
	ratio  string
	date   string
}

// newInvestmentSplitCmd registers `tmoney investment split`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--ticker` and `--ratio` are
// required.
func newInvestmentSplitCmd() *cobra.Command {
	opts := &investmentSplitOptions{}
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Apply a stock split or reverse split to a security",
		Long: "Apply a stock split (e.g. 4:1) or reverse split (e.g. 1:10) " +
			"to a security. All open positions and (if lot tracking is " +
			"enabled) lots are adjusted by the supplied ratio.",
		Example: "  tmoney investment split --ticker AAPL --ratio 4:1\n" +
			"  tmoney investment split --ticker AAPL --ratio 1:10 --date 2025-01-15",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentSplit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (required)")
	cmd.Flags().StringVar(&opts.ratio, "ratio", "", "Split ratio N:D, e.g. 4:1 forward or 1:10 reverse (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Effective date YYYY-MM-DD (default today)")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("ratio")
	return cmd
}

// runInvestmentSplit executes `tmoney investment split`: apply a stock
// split or reverse split to a security.
func runInvestmentSplit(opts *investmentSplitOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	params, err := investmentdom.ParseSplitRatio(opts.ratio)
	if err != nil {
		return fmt.Errorf("invalid --ratio: %w", err)
	}

	var date types.Date
	if opts.date != "" {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to apply corporate actions", opts.ticker)
	}

	action, err := svc.CorporateAction.Split(sec.ID, date, *params)
	if err != nil {
		return fmt.Errorf("failed to apply stock split: %w", err)
	}

	fmt.Fprintln(w, "Stock split applied successfully!")
	fmt.Fprintf(w, "  Security: %s (%s)\n", sec.Ticker, sec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Ratio:    %s\n", params.RatioString())
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
