package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentSplitLotOptions are the inputs to `tmoney investment split-lot`.
type investmentSplitLotOptions struct {
	file  string
	lot   string
	ratio string
}

// newInvestmentSplitLotCmd registers `tmoney investment split-lot`. It applies a
// split ratio to a SINGLE lot, as a repair for a lot entered after a global
// split had already run. Unlike `investment split`, it records no corporate
// action and touches no other lot.
func newInvestmentSplitLotCmd() *cobra.Command {
	opts := &investmentSplitLotOptions{}
	cmd := &cobra.Command{
		Use:   "split-lot",
		Short: "Apply a split ratio to a single lot (repair)",
		Long: "Apply a stock split (e.g. 2:1) or reverse split (e.g. 1:2) to a " +
			"SINGLE lot, identified by its lot ID. This is a repair for a lot " +
			"that was entered after a security-wide split had already been " +
			"applied, so the global split never scaled it. It scales the lot's " +
			"shares, original shares, and per-share cost by the ratio and " +
			"recomputes the account's position from its lots. It records no " +
			"corporate-action history and leaves every other lot untouched. The " +
			"lot must not have been sold against. Find lot IDs with " +
			"`investment portfolio --account NAME --show-lots`.",
		Example:      "  tmoney investment split-lot --lot 019e9fea-463f-75bc-9044-cd6f10bb53f0 --ratio 2:1",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentSplitLot(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.lot, "lot", "", "Lot ID to split (required)")
	cmd.Flags().StringVar(&opts.ratio, "ratio", "", "Split ratio N:D, e.g. 2:1 forward or 1:2 reverse (required)")
	_ = cmd.MarkFlagRequired("lot")
	_ = cmd.MarkFlagRequired("ratio")
	return cmd
}

// runInvestmentSplitLot executes `tmoney investment split-lot`.
func runInvestmentSplitLot(opts *investmentSplitLotOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	lotID, err := types.ParseID(opts.lot)
	if err != nil {
		return fmt.Errorf("invalid --lot: %w", err)
	}

	params, err := investmentdom.ParseSplitRatio(opts.ratio)
	if err != nil {
		return fmt.Errorf("invalid --ratio: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	lot, err := svc.CorporateAction.SplitLot(lotID, *params)
	if err != nil {
		return fmt.Errorf("failed to split lot: %w", err)
	}

	fmt.Fprintln(w, "Lot split applied successfully!")
	fmt.Fprintf(w, "  Lot:        %s\n", lot.ID.String())
	fmt.Fprintf(w, "  Ratio:      %s\n", params.RatioString())
	fmt.Fprintf(w, "  Shares:     %s\n", lot.Shares.String())
	fmt.Fprintf(w, "  Cost/share: %s\n", lot.CostPerShare.String())

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
