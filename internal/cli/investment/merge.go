package investment

import (
	"fmt"
	"io"
	"strconv"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentMergeOptions are the inputs to `tmoney investment merge`.
type investmentMergeOptions struct {
	file          string
	source        string
	target        string
	exchangeRatio string
	cashPerShare  string
	date          string
}

// newInvestmentMergeCmd registers `tmoney investment merge`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--source`, `--target`, and
// `--exchange-ratio` are required.
func newInvestmentMergeCmd() *cobra.Command {
	opts := &investmentMergeOptions{}
	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Apply a merger or acquisition between two securities",
		Long: "Apply a merger/acquisition: every share of the source " +
			"security is exchanged for `--exchange-ratio` shares of the " +
			"target security, optionally with cash consideration via " +
			"`--cash-per-share`.",
		Example: "  tmoney investment merge --source AAPL --target GOOG --exchange-ratio 0.5\n" +
			"  tmoney investment merge --source AAPL --target GOOG --exchange-ratio 0.5 --cash-per-share 10.50",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentMerge(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.source, "source", "", "Source security ticker (required)")
	cmd.Flags().StringVar(&opts.target, "target", "", "Target security ticker (required)")
	cmd.Flags().StringVar(&opts.exchangeRatio, "exchange-ratio", "", "Shares of target received per source share (required)")
	cmd.Flags().StringVar(&opts.cashPerShare, "cash-per-share", "", "Optional cash consideration per source share")
	cmd.Flags().StringVar(&opts.date, "date", "", "Effective date YYYY-MM-DD (default today)")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("exchange-ratio")
	return cmd
}

// runInvestmentMerge executes `tmoney investment merge`: apply a merger or
// acquisition between two securities.
func runInvestmentMerge(opts *investmentMergeOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	ratio, err := strconv.ParseFloat(opts.exchangeRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --exchange-ratio: %w", err)
	}

	var cashPerShare float64
	if opts.cashPerShare != "" {
		cashPerShare, err = strconv.ParseFloat(opts.cashPerShare, 64)
		if err != nil {
			return fmt.Errorf("invalid --cash-per-share: %w", err)
		}
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

	params := investmentdom.MergerParams{
		ExchangeRatio: ratio,
		CashPerShare:  cashPerShare,
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sourceSec, err := svc.Security.GetByTicker(opts.source, "")
	if err != nil {
		return fmt.Errorf("source security %q not found", opts.source)
	}
	if sourceSec.Hidden {
		return fmt.Errorf("source security %q is hidden; unhide it first to apply corporate actions", opts.source)
	}

	targetSec, err := svc.Security.GetByTicker(opts.target, "")
	if err != nil {
		return fmt.Errorf("target security %q not found", opts.target)
	}
	if targetSec.Hidden {
		return fmt.Errorf("target security %q is hidden; unhide it first to apply corporate actions", opts.target)
	}

	action, err := svc.CorporateAction.Merger(sourceSec.ID, targetSec.ID, date, params)
	if err != nil {
		return fmt.Errorf("failed to apply merger: %w", err)
	}

	fmt.Fprintln(w, "Merger applied successfully!")
	fmt.Fprintf(w, "  Source:   %s (%s)\n", sourceSec.Ticker, sourceSec.Name)
	fmt.Fprintf(w, "  Target:   %s (%s)\n", targetSec.Ticker, targetSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Exchange Ratio: %s\n", opts.exchangeRatio)
	if params.HasCashConsideration() {
		fmt.Fprintf(w, "  Cash/Share: $%.2f\n", cashPerShare)
	}
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
