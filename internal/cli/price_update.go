package cli

import (
	"errors"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/spf13/cobra"
)

// registerPriceProviders registers the network-backed price providers on
// a freshly opened Services. Tests override this to point providers at a
// local httptest server with a fixed clock.
var registerPriceProviders = func(svc *app.Services) {
	svc.Price.ProviderRegistry().Register(price.NewYahooProvider())
}

// priceUpdateOptions are the inputs to `tmoney price update`.
type priceUpdateOptions struct {
	file     string
	provider string
	tickers  []string
}

// newPriceUpdateCmd registers `tmoney price update [tickers...]`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Positional arguments restrict the
// refresh to specific tickers; with none, every visible security with a
// ticker is refreshed.
func newPriceUpdateCmd() *cobra.Command {
	opts := &priceUpdateOptions{}
	cmd := &cobra.Command{
		Use:   "update [tickers...]",
		Short: "Refresh prices for securities from a provider",
		Long: "Fetch the latest closed-session price from a provider for " +
			"each visible security with a ticker, or only the supplied " +
			"tickers. Prices are upserted with source = api. Re-running " +
			"the same day is a no-op.",
		Example: "  tmoney price update\n" +
			"  tmoney price update AAPL MSFT\n" +
			"  tmoney price update --provider yahoo",
		Args:         cobra.ArbitraryArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.tickers = args
			return runPriceUpdate(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Price provider name (default: yahoo)")
	return cmd
}

// runPriceUpdate fetches the latest closed-session price from a provider
// for each visible security with a ticker (or the explicit ticker filter),
// printing per-ticker outcomes and a summary.
func runPriceUpdate(opts *priceUpdateOptions, w io.Writer) error {
	if opts.file == "" {
		return errors.New("--file is required to specify a database")
	}

	providerName := opts.provider
	if providerName == "" {
		providerName = "yahoo"
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	registerPriceProviders(svc)

	result, err := svc.Price.RefreshPrices(providerName, opts.tickers)
	if err != nil {
		return err
	}

	printRefreshResult(w, result)
	autoBackupAfterModification(opts.file)

	if result.HasFailures() {
		return errors.New("one or more tickers failed to update")
	}
	return nil
}

// printRefreshResult writes a per-ticker table followed by an aggregate
// summary line describing the run outcome.
func printRefreshResult(w io.Writer, result *price.RefreshResult) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TICKER\tSTATUS\tDATE\tPRICE\tNOTE")
	for _, e := range result.Entries {
		ticker := e.Ticker
		if ticker == "" {
			ticker = "-"
		}
		date := "-"
		if !e.Date.IsZero() {
			date = e.Date.String()
		}
		priceStr := "-"
		if !e.Price.IsZero() {
			priceStr = e.Price.String()
		}
		note := e.Note
		if e.Error != nil {
			note = e.Error.Error()
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ticker, displayOutcome(e.Outcome), date, priceStr, note)
	}
	_ = tw.Flush()

	counts := result.CountByOutcome()
	fmt.Fprintf(w, "\n%d updated, %d up-to-date, %d skipped, %d failed\n",
		counts[price.OutcomeUpdated],
		counts[price.OutcomeUpToDate],
		counts[price.OutcomeSkippedHidden]+counts[price.OutcomeSkippedNoTicker]+counts[price.OutcomeSkippedCurrency],
		counts[price.OutcomeFailed],
	)
}

func displayOutcome(o price.RefreshOutcome) string {
	switch o {
	case price.OutcomeUpdated:
		return "updated"
	case price.OutcomeUpToDate:
		return "up-to-date"
	case price.OutcomeSkippedHidden:
		return "skipped (hidden)"
	case price.OutcomeSkippedNoTicker:
		return "skipped (no ticker)"
	case price.OutcomeSkippedCurrency:
		return "skipped (currency)"
	case price.OutcomeFailed:
		return "failed"
	default:
		return string(o)
	}
}
