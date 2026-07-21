package investment

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentListOptions are the inputs to `tmoney investment list`.
type investmentListOptions struct {
	file     string
	account  string
	ticker   string
	txnType  string
	fromDate string
	toDate   string
	limit    int
	showIDs  bool
}

// newInvestmentListCmd registers `tmoney investment list`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--account` is required.
func newInvestmentListCmd() *cobra.Command {
	opts := &investmentListOptions{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List investment transactions for an account",
		Long: "List the investment register for an account, optionally filtered " +
			"by security, transaction type, or date range. Pass --show-ids to " +
			"print each transaction's UUID for scripting `investment edit`.",
		Example: "  tmoney investment list --account Brokerage\n" +
			"  tmoney investment list --account Brokerage --ticker AAPL --show-ids",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			if opts.limit < 0 {
				return fmt.Errorf("--limit must be a non-negative number")
			}
			return runInvestmentList(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.account, "account", "", "Investment account name (required)")
	cmd.Flags().StringVar(&opts.ticker, "ticker", "", "Filter by security ticker")
	cmd.Flags().StringVar(&opts.txnType, "type", "", "Filter by transaction type (buy, sell, dividend, reinvest_dividend, fee, fee_liquidation, deposit, withdrawal, interest, transfer_cash, transfer_shares, exchange)")
	cmd.Flags().StringVar(&opts.fromDate, "from", "", "Earliest date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&opts.toDate, "to", "", "Latest date (YYYY-MM-DD)")
	cmd.Flags().IntVar(&opts.limit, "limit", 0, "Maximum number of transactions to display (0 = no limit)")
	cmd.Flags().BoolVar(&opts.showIDs, "show-ids", false, "Prefix each row with the transaction's UUID")
	_ = cmd.MarkFlagRequired("account")
	return cmd
}

// runInvestmentList lists investment transactions for an account.
func runInvestmentList(opts *investmentListOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	filter := investmentdom.TransactionFilter{}

	if opts.txnType != "" {
		txnType := investmentdom.TransactionType(opts.txnType)
		if !txnType.IsValid() {
			return fmt.Errorf("invalid --type %q", opts.txnType)
		}
		filter.Type = &txnType
	}
	if opts.fromDate != "" {
		from, err := types.ParseDate(opts.fromDate)
		if err != nil {
			return fmt.Errorf("invalid --from date: %w", err)
		}
		filter.FromDate = &from
	}
	if opts.toDate != "" {
		to, err := types.ParseDate(opts.toDate)
		if err != nil {
			return fmt.Errorf("invalid --to date: %w", err)
		}
		filter.ToDate = &to
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	acct, err := svc.Account.GetByName(opts.account)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.account)
	}

	if opts.ticker != "" {
		sec, err := svc.Security.Resolve(opts.ticker, "", "")
		if err != nil {
			return err
		}
		filter.SecurityID = &sec.ID
	}

	txns, err := svc.InvestmentRepo.ListByAccount(acct.ID, filter)
	if err != nil {
		return fmt.Errorf("failed to list investment transactions: %w", err)
	}

	if opts.limit > 0 && len(txns) > opts.limit {
		txns = txns[:opts.limit]
	}

	securities, err := svc.SecurityRepo.List(security.Filter{})
	if err != nil {
		return fmt.Errorf("failed to list securities: %w", err)
	}
	tickers := make(map[types.ID]string, len(securities))
	for _, sec := range securities {
		tickers[sec.ID] = securityLabel(sec)
	}

	fmt.Fprintf(w, "INVESTMENT TRANSACTIONS: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("INVESTMENT TRANSACTIONS: ")+len(acct.Name)))

	if len(txns) == 0 {
		fmt.Fprintln(w, "No transactions found.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if opts.showIDs {
		fmt.Fprintln(tw, "ID\tDate\tType\tSecurity\tShares\tPrice\tAmount\tStatus")
	} else {
		fmt.Fprintln(tw, "Date\tType\tSecurity\tShares\tPrice\tAmount\tStatus")
	}
	for _, txn := range txns {
		secLabel := ""
		if txn.SecurityID.Valid {
			secLabel = tickers[txn.SecurityID.ID]
		}
		shares := ""
		if txn.Shares.Valid {
			shares = txn.Shares.Quantity.String()
		}
		price := ""
		if txn.PricePerShare.Valid {
			price = cmdutil.FormatMoney(txn.PricePerShare.Money, acct.Currency)
		}
		row := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s",
			txn.Date.String(),
			txn.Type.DisplayName(),
			secLabel,
			shares,
			price,
			cmdutil.FormatMoney(txn.TotalAmount, acct.Currency),
			txn.Status,
		)
		if opts.showIDs {
			row = txn.ID.String() + "\t" + row
		}
		fmt.Fprintln(tw, row)
	}
	return tw.Flush()
}

// securityLabel returns the ticker when present, else the security name.
func securityLabel(sec *security.Security) string {
	if sec.Ticker != "" {
		return sec.Ticker
	}
	return sec.Name
}
