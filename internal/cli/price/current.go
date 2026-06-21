package price

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceCurrentOptions are the inputs to `tmoney price current`.
type priceCurrentOptions struct {
	file   string
	ticker string
	isin   string
	name   string
}

// newPriceCurrentCmd registers `tmoney price current [ticker]`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. Identify the security by a positional
// ticker, or by `--isin` / `--name` for a security that has no ticker.
func newPriceCurrentCmd() *cobra.Command {
	opts := &priceCurrentOptions{}
	cmd := &cobra.Command{
		Use:          "current [ticker]",
		Short:        "Show the most recent price for a security",
		Long:         "Show the most recent price recorded for a security. Identify it by a positional ticker, or by `--isin` / `--name` (exact).",
		Example:      "  tmoney price current AAPL\n  tmoney price current --isin US0378331005",
		Args:         cobra.RangeArgs(0, 1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if len(args) > 0 {
				opts.ticker = args[0]
			}
			return runPriceCurrent(opts, cmd.OutOrStdout())
		},
	}
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	return cmd
}

// runPriceCurrent shows the most recent price for a security.
func runPriceCurrent(opts *priceCurrentOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
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

	asOf := types.Today()
	p, err := svc.Price.GetCurrentPrice(sec.ID, asOf)
	if err != nil {
		return fmt.Errorf("no price found for %s", cmdutil.SecurityRef(sec.Ticker, sec.Name))
	}

	fmt.Fprintf(w, "CURRENT PRICE: %s\n", cmdutil.SecurityRef(sec.Ticker, sec.Name))
	if sec.Ticker != "" {
		fmt.Fprintf(w, "Ticker:  %s\n", sec.Ticker)
	}
	fmt.Fprintf(w, "Name:    %s\n", sec.Name)
	if sec.ISIN != "" {
		fmt.Fprintf(w, "ISIN:    %s\n", sec.ISIN)
	}
	fmt.Fprintf(w, "Date:    %s\n", p.Date.String())
	fmt.Fprintf(w, "Price:   %s\n", cmdutil.FormatMoney(p.Price, sec.Currency))
	fmt.Fprintf(w, "Source:  %s\n", p.Source.DisplayName())

	return nil
}
