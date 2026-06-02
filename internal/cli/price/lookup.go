package price

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceLookupOptions are the inputs to `tmoney price lookup`.
type priceLookupOptions struct {
	file     string
	ticker   string
	date     string
	provider string
}

// newPriceLookupCmd registers `tmoney price lookup`. It fetches and prints a
// provider's closing price on or before a date without recording anything.
func newPriceLookupCmd() *cobra.Command {
	opts := &priceLookupOptions{}
	cmd := &cobra.Command{
		Use:   "lookup",
		Short: "Fetch and print a provider's closing price for a date (no store)",
		Long: "Fetch the closing price for a security on or before a date from a " +
			"provider and print it, without recording anything. A weekend or " +
			"holiday date resolves to the prior trading day. Use it to sanity-check " +
			"a value before recording it with `price add --fetch`.",
		Example: "  tmoney price lookup --ticker AAPL --date 2024-01-15\n" +
			"  tmoney price lookup --ticker GBTC --date 2024-07-31 --provider yahoo",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runPriceLookup(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "As-of date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Price provider name (default: yahoo)")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("date")
	return cmd
}

// runPriceLookup fetches and prints a provider quote for a date.
func runPriceLookup(opts *priceLookupOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	date, err := types.ParseDate(opts.date)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	providerName := opts.provider
	if providerName == "" {
		providerName = "yahoo"
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	registerPriceProviders(svc)
	provider, err := svc.Price.ProviderRegistry().Get(providerName)
	if err != nil {
		return err
	}

	quote, err := provider.FetchQuoteOn(opts.ticker, date)
	if err != nil {
		return fmt.Errorf("lookup failed for %s: %w", opts.ticker, err)
	}

	fmt.Fprintf(w, "%s closed at %s on %s (%s, provider %s)\n",
		opts.ticker, cmdutil.FormatMoney(quote.Price, quote.Currency), quote.Date.String(), quote.Currency, providerName)
	return nil
}
