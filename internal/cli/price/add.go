package price

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	pricedom "github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// priceAddOptions are the inputs to `tmoney price add`.
type priceAddOptions struct {
	file   string
	ticker string
	date   string
	value  string
}

// newPriceAddCmd registers `tmoney price add`. The database file is
// taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--ticker`, `--date`, and `--price` are required.
func newPriceAddCmd() *cobra.Command {
	opts := &priceAddOptions{}
	cmd := &cobra.Command{
		Use:          "add",
		Short:        "Add a manual price for a security",
		Long:         "Record a price for a security on a specific date. The source is set to manual.",
		Example:      "  tmoney price add --ticker AAPL --date 2024-01-15 --price 150.00",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runPriceAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Ticker symbol (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Price date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&opts.value, "price", "", "Price value (required)")
	_ = cmd.MarkFlagRequired("ticker")
	_ = cmd.MarkFlagRequired("date")
	_ = cmd.MarkFlagRequired("price")
	return cmd
}

// runPriceAdd adds a price for a security.
func runPriceAdd(opts *priceAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	priceDate, err := types.ParseDate(opts.date)
	if err != nil {
		return fmt.Errorf("invalid --date: %w", err)
	}

	priceAmount, err := types.NewMoney(opts.value)
	if err != nil {
		return fmt.Errorf("invalid --price: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	sec, err := svc.Security.GetByTicker(opts.ticker, "")
	if err != nil {
		return fmt.Errorf("security %q not found", opts.ticker)
	}

	p := pricedom.NewPrice(sec.ID, priceDate, priceAmount, pricedom.SourceManual)
	if err := svc.Price.AddPrice(p); err != nil {
		return fmt.Errorf("failed to add price: %w", err)
	}

	fmt.Fprintf(w, "Price added for %s on %s: %s\n", sec.Ticker, priceDate.String(), cmdutil.FormatMoney(priceAmount, sec.Currency))

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
