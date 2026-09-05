package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentTransferOptions are the inputs to `tmoney investment transfer`.
type investmentTransferOptions struct {
	file        string
	fromAccount string
	toAccount   string
	ticker      string
	isin        string
	name        string
	shares      string
	date        string
	memo        string
	lot         string
}

// newInvestmentTransferCmd registers `tmoney investment transfer`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--from`, `--to`, and `--shares` are
// required; identify the security with `--ticker`, `--isin`, or `--name`.
func newInvestmentTransferCmd() *cobra.Command {
	opts := &investmentTransferOptions{}
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Transfer shares between investment accounts",
		Long: "Transfer shares of a security from one investment account " +
			"to another. No cash changes hands; the share count moves to " +
			"the destination account. For lot-tracked source accounts, " +
			"pass --lot to allocate against a specific open lot.",
		Example: "  tmoney investment transfer --from \"Source IRA\" --to \"Dest 401k\" --ticker AAPL --shares 5\n" +
			"  tmoney investment transfer --from Brokerage --to RolloverIRA --ticker VTI --shares 100 --date 2025-04-15",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runInvestmentTransfer(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.fromAccount, "from", "", "Source investment account name (required)")
	cmd.Flags().StringVar(&opts.toAccount, "to", "", "Destination investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Security ticker (or use --isin / --name)")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "Number of shares to transfer (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transaction date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	cmd.Flags().StringVar(&opts.lot, "lot", "", "Lot ID to allocate against (lot-tracked source accounts)")
	cmdutil.AddSecuritySelectorFlags(cmd, &opts.isin, &opts.name)
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("shares")
	return cmd
}

// runInvestmentTransfer executes `tmoney investment transfer`: move
// shares of a security from one investment account to another.
func runInvestmentTransfer(opts *investmentTransferOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	shares, err := types.NewQuantity(opts.shares)
	if err != nil {
		return fmt.Errorf("invalid --shares: %w", err)
	}

	var date types.Date
	if opts.date != "" {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	fromAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	toAcct, err := svc.Account.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	sec, err := svc.Security.Resolve(opts.ticker, opts.isin, opts.name)
	if err != nil {
		return err
	}
	if sec.Hidden {
		return fmt.Errorf("security %q is hidden; unhide it first to create transactions", cmdutil.SecurityRef(sec.Ticker, sec.Name))
	}

	var lotAllocations []investmentdom.SellLotAllocation
	if opts.lot != "" {
		lotID, err := types.ParseID(opts.lot)
		if err != nil {
			return fmt.Errorf("invalid --lot: %w", err)
		}
		lotAllocations = []investmentdom.SellLotAllocation{
			{LotID: lotID, Shares: shares},
		}
	}

	if _, err := svc.Investment.TransferShares(fromAcct.ID, toAcct.ID, sec.ID, date, shares, opts.memo, lotAllocations); err != nil {
		return fmt.Errorf("failed to transfer shares: %w", err)
	}

	fmt.Fprintln(w, "Share transfer created successfully!")
	fmt.Fprintf(w, "  From:     %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:       %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Security: %s\n", cmdutil.SecurityDisplay(sec.Ticker, sec.Name))
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Shares:   %s\n", shares.String())

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
