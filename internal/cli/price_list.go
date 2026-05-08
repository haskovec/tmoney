package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceListOptions are the inputs to `tmoney price list`.
type priceListOptions struct {
	file     string
	ticker   string
	fromDate string
	toDate   string
}

// newPriceListCmd registers `tmoney price list <ticker>`. The database
// file is taken from the persistent `--file` / `-f` flag inherited
// from the root command. Optional `--from` and `--to` filters limit
// the date range of returned prices.
func newPriceListCmd() *cobra.Command {
	opts := &priceListOptions{}
	cmd := &cobra.Command{
		Use:          "list <ticker>",
		Short:        "List recorded prices for a security",
		Long:         "List the price history for a security identified by ticker, optionally filtered by date.",
		Example:      "  tmoney price list AAPL\n  tmoney price list AAPL --from 2024-01-01 --to 2024-06-30",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runPriceList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.fromDate, "from", "", "Earliest date YYYY-MM-DD (inclusive)")
	cmd.Flags().StringVar(&opts.toDate, "to", "", "Latest date YYYY-MM-DD (inclusive)")
	return cmd
}

// runPriceList lists prices for a security ticker.
func runPriceList(opts *priceListOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}

	var from, to *types.Date
	if opts.fromDate != "" {
		d, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		from = &d
	}
	if opts.toDate != "" {
		d, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		to = &d
	}

	prices, err := svc.Price.GetPriceHistory(sec.ID, from, to)
	if err != nil {
		return fmt.Errorf("failed to get prices: %w", err)
	}

	printPricesTable(w, sec.Ticker, prices)

	return nil
}
