package price

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	pricedom "github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceAddOptions are the inputs to `tmoney price add`.
type priceAddOptions struct {
	file     string
	ticker   string
	isin     string
	name     string
	date     string
	value    string
	fetch    bool
	provider string
}

// newPriceAddCmd registers `tmoney price add`. The database file is
// taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--date` is required, plus either `--price` or `--fetch`;
// identify the security with `--ticker`, `--isin`, or `--name`.
func newPriceAddCmd() *cobra.Command {
	opts := &priceAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a price for a security",
		Long: "Record a price for a security on a specific date (source = manual). " +
			"Pass --fetch to look the price up from a provider for --date instead of " +
			"supplying --price (stored with source = api).",
		Example: "  tmoney price add --ticker AAPL --date 2024-01-15 --price 150.00\n" +
			"  tmoney price add --ticker AAPL --date 2024-01-15 --fetch",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runPriceAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (or use --isin / --name)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Price date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&opts.value, "price", "", "Price value (omit when using --fetch)")
	cmd.Flags().BoolVar(&opts.fetch, "fetch", false, "Fetch the price for --date from a provider instead of passing --price")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Price provider name when using --fetch (default: yahoo)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	_ = cmd.MarkFlagRequired("date")
	return cmd
}

// runPriceAdd adds a price for a security.
func runPriceAdd(opts *priceAddOptions, w io.Writer) error {
	if opts.fetch && opts.value != "" {
		return fmt.Errorf("pass either --price or --fetch, not both")
	}
	if !opts.fetch && opts.value == "" {
		return fmt.Errorf("provide --price, or use --fetch to fetch it from a provider")
	}

	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	priceDate, err := types.ParseDate(opts.date)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	// Parse a manual --price up front so a malformed value fails before we
	// open the database. The fetch path resolves its amount from the provider.
	manualAmount := types.ZeroMoney
	if !opts.fetch {
		manualAmount, err = types.NewMoney(opts.value)
		if err != nil {
			return fmt.Errorf("invalid --price: %w", err)
		}
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

	priceAmount := manualAmount
	source := pricedom.SourceManual
	if opts.fetch {
		if sec.Ticker == "" {
			return fmt.Errorf("cannot --fetch a price for a security with no ticker; enter --price manually")
		}
		providerName := opts.provider
		if providerName == "" {
			providerName = "yahoo"
		}
		registerPriceProviders(svc)
		provider, err := svc.Price.ProviderRegistry().Get(providerName)
		if err != nil {
			return err
		}
		quote, err := provider.FetchQuoteOn(sec.Ticker, priceDate)
		if err != nil {
			return fmt.Errorf("fetch failed for %s: %w", sec.Ticker, err)
		}
		if quote.Currency != "" && !strings.EqualFold(quote.Currency, sec.Currency) {
			return fmt.Errorf("provider currency %s does not match %s currency %s", quote.Currency, sec.Ticker, sec.Currency)
		}
		priceAmount = quote.Price
		source = pricedom.SourceAPI
	}

	p := pricedom.NewPrice(sec.ID, priceDate, priceAmount, source)
	if err := svc.Price.AddPrice(p); err != nil {
		return fmt.Errorf("failed to add price: %w", err)
	}

	fmt.Fprintf(w, "Price added for %s on %s: %s (source %s)\n", cmdutil.SecurityRef(sec.Ticker, sec.Name), priceDate.String(), cmdutil.FormatMoney(priceAmount, sec.Currency), source)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
