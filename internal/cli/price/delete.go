package price

import (
	"errors"
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceDeleteOptions are the inputs to `tmoney price delete`.
type priceDeleteOptions struct {
	file   string
	ticker string
	isin   string
	name   string
	date   string
}

// newPriceDeleteCmd registers `tmoney price delete`. It removes the price for a
// security on an exact date — the CLI counterpart to deleting a price row in
// the TUI prices view (both call price.Service.DeletePrice). Identify the
// security with `--ticker`, `--isin`, or `--name`; `--date` is required.
func newPriceDeleteCmd() *cobra.Command {
	opts := &priceDeleteOptions{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a price for a security on a date",
		Long: "Remove the recorded price for a security on a specific date. " +
			"Identify the security with --ticker, --isin, or --name. This is the " +
			"CLI equivalent of deleting a price in the TUI prices view.",
		Example: "  tmoney price delete --ticker AAPL --date 2024-01-15\n" +
			"  tmoney price delete --name \"MFS Mid Cap Value CT\" --date 2024-01-15",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runPriceDelete(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (or use --isin / --name)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Price date YYYY-MM-DD (required)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	_ = cmd.MarkFlagRequired("date")
	return cmd
}

// runPriceDelete deletes the price for a security on an exact date.
func runPriceDelete(opts *priceDeleteOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	priceDate, err := types.ParseDate(opts.date)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
	if err != nil {
		return err
	}

	p, err := svc.Price.GetBySecurityAndDate(sec.ID, priceDate)
	if err != nil {
		var notFound *dberrors.NotFoundError
		if errors.As(err, &notFound) {
			return fmt.Errorf("no price recorded for %s on %s", cmdutil.SecurityRef(sec.Ticker, sec.Name), priceDate.String())
		}
		return fmt.Errorf("failed to look up price: %w", err)
	}

	if err := svc.Price.DeletePrice(p.ID); err != nil {
		return fmt.Errorf("failed to delete price: %w", err)
	}

	fmt.Fprintf(w, "Price deleted for %s on %s: %s (source %s)\n",
		cmdutil.SecurityRef(sec.Ticker, sec.Name), priceDate.String(),
		cmdutil.FormatMoney(p.Price, sec.Currency), p.Source)

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
