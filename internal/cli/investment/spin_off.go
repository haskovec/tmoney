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

// investmentSpinOffOptions are the inputs to `tmoney investment spin-off`.
type investmentSpinOffOptions struct {
	file             string
	parent           string
	child            string
	shareRatio       string
	parentAllocation string
	spinOffPrice     string
	date             string
}

// newInvestmentSpinOffCmd registers `tmoney investment spin-off`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--parent`, `--spinoff`,
// `--share-ratio`, `--parent-allocation`, and `--spin-off-price` are
// required.
func newInvestmentSpinOffCmd() *cobra.Command {
	opts := &investmentSpinOffOptions{}
	cmd := &cobra.Command{
		Use:   "spin-off",
		Short: "Apply a corporate spin-off splitting a parent security into a new child",
		Long: "Apply a corporate spin-off: a parent security distributes a " +
			"new child security to existing holders at the supplied share " +
			"ratio, with the parent's cost basis split between the two by " +
			"the parent-allocation percentage.",
		Example: "  tmoney investment spin-off --parent AAPL --spinoff GOOG \\\n" +
			"    --share-ratio 0.5 --parent-allocation 80 --spin-off-price 25",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentSpinOff(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.parent, "parent", "", "Parent security ticker (required)")
	cmd.Flags().StringVar(&opts.child, "spinoff", "", "Spin-off (child) security ticker (required)")
	cmd.Flags().StringVar(&opts.shareRatio, "share-ratio", "", "Shares of child received per parent share (required)")
	cmd.Flags().StringVar(&opts.parentAllocation, "parent-allocation", "", "Percentage of parent cost basis retained by the parent (required)")
	cmd.Flags().StringVar(&opts.spinOffPrice, "spin-off-price", "", "Price per share of the spin-off security (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Effective date YYYY-MM-DD (default today)")
	_ = cmd.MarkFlagRequired("parent")
	_ = cmd.MarkFlagRequired("spinoff")
	_ = cmd.MarkFlagRequired("share-ratio")
	_ = cmd.MarkFlagRequired("parent-allocation")
	_ = cmd.MarkFlagRequired("spin-off-price")
	return cmd
}

// runInvestmentSpinOff executes `tmoney investment spin-off`: apply a
// corporate spin-off.
func runInvestmentSpinOff(opts *investmentSpinOffOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	shareRatio, err := strconv.ParseFloat(opts.shareRatio, 64)
	if err != nil {
		return fmt.Errorf("invalid --share-ratio: %w", err)
	}

	parentAlloc, err := strconv.ParseFloat(opts.parentAllocation, 64)
	if err != nil {
		return fmt.Errorf("invalid --parent-allocation: %w", err)
	}

	spinPrice, err := types.NewMoney(opts.spinOffPrice)
	if err != nil {
		return fmt.Errorf("invalid --spin-off-price: %w", err)
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

	params := investmentdom.SpinOffParams{
		ShareRatio:          shareRatio,
		ParentAllocationPct: parentAlloc,
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	parentSec, err := svc.Security.GetByTicker(opts.parent, "")
	if err != nil {
		return fmt.Errorf("parent security %q not found", opts.parent)
	}
	if parentSec.Hidden {
		return fmt.Errorf("parent security %q is hidden; unhide it first to apply corporate actions", opts.parent)
	}

	childSec, err := svc.Security.GetByTicker(opts.child, "")
	if err != nil {
		return fmt.Errorf("spin-off security %q not found", opts.child)
	}
	if childSec.Hidden {
		return fmt.Errorf("spin-off security %q is hidden; unhide it first to apply corporate actions", opts.child)
	}

	action, err := svc.CorporateAction.SpinOff(parentSec.ID, childSec.ID, date, params, spinPrice)
	if err != nil {
		return fmt.Errorf("failed to apply spin-off: %w", err)
	}

	fmt.Fprintln(w, "Spin-off applied successfully!")
	fmt.Fprintf(w, "  Parent:   %s (%s)\n", parentSec.Ticker, parentSec.Name)
	fmt.Fprintf(w, "  Spin-off: %s (%s)\n", childSec.Ticker, childSec.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Share Ratio: %s\n", opts.shareRatio)
	fmt.Fprintf(w, "  Parent Allocation: %s%%\n", opts.parentAllocation)
	fmt.Fprintf(w, "  Spin-off Price: %s\n", cmdutil.FormatMoney(spinPrice, "USD"))
	fmt.Fprintf(w, "  Action ID: %s\n", action.ID.String())

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
