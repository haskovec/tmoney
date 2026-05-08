package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceCurrentOptions are the inputs to `tmoney price current`.
type priceCurrentOptions struct {
	file   string
	ticker string
}

// newPriceCurrentCmd registers `tmoney price current <ticker>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newPriceCurrentCmd() *cobra.Command {
	opts := &priceCurrentOptions{}
	cmd := &cobra.Command{
		Use:          "current <ticker>",
		Short:        "Show the most recent price for a security",
		Long:         "Show the most recent price recorded for a security identified by ticker.",
		Example:      "  tmoney price current AAPL",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.ticker = args[0]
			return runPriceCurrent(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runPriceCurrent shows the most recent price for a security.
func runPriceCurrent(opts *priceCurrentOptions, w io.Writer) error {
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

	asOf := types.Today()
	p, err := svc.Price.GetCurrentPrice(sec.ID, asOf)
	if err != nil {
		return fmt.Errorf("no price found for %s", sec.Ticker)
	}

	fmt.Fprintf(w, "CURRENT PRICE: %s\n", sec.Ticker)
	fmt.Fprintf(w, "Ticker:  %s\n", sec.Ticker)
	fmt.Fprintf(w, "Name:    %s\n", sec.Name)
	fmt.Fprintf(w, "Date:    %s\n", p.Date.String())
	fmt.Fprintf(w, "Price:   %s\n", formatMoney(p.Price, sec.Currency))
	fmt.Fprintf(w, "Source:  %s\n", p.Source.DisplayName())

	return nil
}
