package price

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/investment"
	pricedom "github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/spf13/cobra"
)

// priceCleanupOptions are the inputs to `tmoney price cleanup`.
type priceCleanupOptions struct {
	file     string
	ticker   string
	isin     string
	name     string
	confirm  bool
	refetch  bool
	provider string
}

// cleanupCandidate pairs an income-only price with the security it belongs to.
type cleanupCandidate struct {
	sec  *security.Security
	item investment.IncomeOnlyPrice
}

// newPriceCleanupCmd registers `tmoney price cleanup`.
func newPriceCleanupCmd() *cobra.Command {
	opts := &priceCleanupOptions{}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove legacy reinvest/fee-liquidation prices",
		Long: "Find transaction-sourced prices that are justified only by a reinvested " +
			"dividend or fee liquidation (no buy or sell on that date). Their per-share " +
			"value is total_amount÷rounded-shares, unreliable for tiny income events, and " +
			"the current policy no longer creates them. Prints a dry-run plan by default; " +
			"pass --confirm to apply. With --refetch, tickered securities are replaced by " +
			"the provider's close for that date instead of deleted (tickerless securities " +
			"and fetch failures are left in place). Limit to one security with --ticker / " +
			"--isin / --name. Run `tmoney db backup` first.",
		Example: "  tmoney price cleanup\n" +
			"  tmoney price cleanup --confirm\n" +
			"  tmoney price cleanup --refetch --confirm\n" +
			"  tmoney price cleanup --ticker VSIAX --refetch --confirm",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runPriceCleanup(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Limit to one security by ticker (or use --isin / --name; default: all securities)")
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Apply the cleanup (default: dry-run preview only)")
	cmd.Flags().BoolVar(&opts.refetch, "refetch", false, "Replace tickered prices with the provider's close for that date instead of deleting")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Price provider for --refetch (default: yahoo)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runPriceCleanup gathers the income-only transaction prices and either prints
// the plan (dry-run) or applies it (--confirm), deleting or refetching each.
func runPriceCleanup(opts *priceCleanupOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Resolve the security set: one (when a selector is given) or all.
	var secs []*security.Security
	if opts.ticker != "" || opts.isin != "" || opts.name != "" {
		sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
		if err != nil {
			return err
		}
		secs = []*security.Security{sec}
	} else {
		secs, err = svc.Security.List(security.Filter{}) // include hidden — bad prices corrupt their valuation too
		if err != nil {
			return fmt.Errorf("failed to list securities: %w", err)
		}
	}

	var candidates []cleanupCandidate
	for _, sec := range secs {
		items, err := svc.Investment.ListIncomeOnlyTransactionPrices(sec.ID)
		if err != nil {
			return err
		}
		for _, it := range items {
			candidates = append(candidates, cleanupCandidate{sec: sec, item: it})
		}
	}

	if len(candidates) == 0 {
		fmt.Fprintln(w, "No income-only transaction prices found. Nothing to clean up.")
		return nil
	}

	if !opts.confirm {
		printCleanupPlan(w, candidates, opts.refetch)
		return nil
	}

	return applyCleanup(w, svc, opts, candidates)
}

// printCleanupPlan writes the dry-run table describing what --confirm would do.
func printCleanupPlan(w io.Writer, candidates []cleanupCandidate, refetch bool) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TICKER\tDATE\tPRICE\tPLANNED ACTION")
	for _, c := range candidates {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			cmdutil.SecurityRef(c.sec.Ticker, c.sec.Name),
			c.item.Price.Date.String(), cmdutil.FormatMoney(c.item.Price.Price, c.sec.Currency),
			plannedAction(c, refetch))
	}
	_ = tw.Flush()

	fmt.Fprintf(w, "\n%d candidate(s). Re-run with --confirm to apply", len(candidates))
	if refetch {
		fmt.Fprint(w, " (--refetch replaces tickered prices with the provider close).")
	} else {
		fmt.Fprint(w, " (deletes them; add --refetch to replace tickered prices with the provider close instead).")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Tip: run `tmoney db backup` first.")
}

// plannedAction describes, for the dry-run, what would happen to one candidate.
func plannedAction(c cleanupCandidate, refetch bool) string {
	if refetch {
		if c.sec.Ticker == "" {
			return "keep (no ticker to refetch)"
		}
		return "refetch provider close"
	}
	if c.item.Fallback != nil {
		return fmt.Sprintf("delete (falls back to %s @ %s)", cmdutil.FormatMoney(c.item.Fallback.Price, c.sec.Currency), c.item.Fallback.Date.String())
	}
	return "delete (no prior price — leaves a gap)"
}

// applyCleanup executes the cleanup for every candidate and prints the outcome.
func applyCleanup(w io.Writer, svc *app.Services, opts *priceCleanupOptions, candidates []cleanupCandidate) error {
	var provider pricedom.Provider
	if opts.refetch {
		providerName := opts.provider
		if providerName == "" {
			providerName = "yahoo"
		}
		registerPriceProviders(svc)
		p, err := svc.Price.ProviderRegistry().Get(providerName)
		if err != nil {
			return err
		}
		provider = p
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TICKER\tDATE\tOLD\tACTION\tDETAIL")
	var deleted, refetched, kept int
	for _, c := range candidates {
		ref := cmdutil.SecurityRef(c.sec.Ticker, c.sec.Name)
		date := c.item.Price.Date
		old := cmdutil.FormatMoney(c.item.Price.Price, c.sec.Currency)

		if opts.refetch {
			action, detail, err := refetchOne(svc, provider, c)
			if err != nil {
				return err
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ref, date.String(), old, action, detail)
			switch action {
			case "refetched":
				refetched++
			default:
				kept++
			}
			continue
		}

		if err := svc.Price.DeletePrice(c.item.Price.ID); err != nil {
			return fmt.Errorf("delete %s @ %s: %w", ref, date.String(), err)
		}
		detail := "no prior price (gap)"
		if c.item.Fallback != nil {
			detail = fmt.Sprintf("falls back to %s @ %s", cmdutil.FormatMoney(c.item.Fallback.Price, c.sec.Currency), c.item.Fallback.Date.String())
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", ref, date.String(), old, "deleted", detail)
		deleted++
	}
	_ = tw.Flush()

	fmt.Fprintf(w, "\n%d deleted, %d refetched, %d kept\n", deleted, refetched, kept)
	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// refetchOne replaces a single candidate's price with the provider's close for
// its date, or keeps it if the security is tickerless, the fetch fails, or the
// currency disagrees. It returns a short action ("refetched"/"kept") and detail.
func refetchOne(svc *app.Services, provider pricedom.Provider, c cleanupCandidate) (action, detail string, err error) {
	if c.sec.Ticker == "" {
		return "kept", "no ticker to refetch", nil
	}
	quote, ferr := provider.FetchQuoteOn(c.sec.Ticker, c.item.Price.Date)
	if ferr != nil {
		return "kept", "fetch failed: " + ferr.Error(), nil
	}
	if quote.Currency != "" && !strings.EqualFold(quote.Currency, c.sec.Currency) {
		return "kept", fmt.Sprintf("currency mismatch (%s vs %s)", quote.Currency, c.sec.Currency), nil
	}
	// Update the existing row in place (price + source) rather than delete+add.
	// UpdatePrice upserts by (security, date), so it is atomic, keeps the row,
	// and avoids DuckDB's "Failed to delete all rows from index" error, which
	// can strike an individual row on a DELETE.
	updated := pricedom.NewPrice(c.sec.ID, c.item.Price.Date, quote.Price, pricedom.SourceAPI)
	if upErr := svc.Price.UpdatePrice(updated); upErr != nil {
		return "", "", fmt.Errorf("update %s @ %s: %w", c.sec.Ticker, c.item.Price.Date.String(), upErr)
	}
	return "refetched", cmdutil.FormatMoney(quote.Price, c.sec.Currency), nil
}
