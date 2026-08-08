package scheduled

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledPostOptions are the inputs to `tmoney scheduled post`.
type scheduledPostOptions struct {
	file   string
	id     string
	amount string
	date   string
}

// newScheduledPostCmd registers `tmoney scheduled post <id>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command. `--amount` and `--date` are optional
// per-occurrence overrides.
func newScheduledPostCmd() *cobra.Command {
	opts := &scheduledPostOptions{}
	cmd := &cobra.Command{
		Use:   "post <id>",
		Short: "Post a scheduled transaction",
		Long: "Post a scheduled transaction by creating a real transaction " +
			"from it and advancing the schedule to its next occurrence. " +
			"`--amount` overrides a variable schedule's posted amount; " +
			"`--date` overrides the posted date.",
		Example: "  tmoney scheduled post <id> --file personal.tdb\n" +
			"  tmoney scheduled post <id> --amount -150.00 --file personal.tdb\n" +
			"  tmoney scheduled post <id> --date 2024-03-20 --file personal.tdb",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.id = args[0]
			return runScheduledPost(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Override the posted amount (e.g. -150.00)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Override the posted date (YYYY-MM-DD)")
	return cmd
}

// runScheduledPost posts a scheduled transaction (creates a real transaction).
func runScheduledPost(opts *scheduledPostOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	stID, err := types.ParseID(opts.id)
	if err != nil {
		return fmt.Errorf("invalid scheduled transaction ID: %w", err)
	}

	st, err := svc.Scheduled.GetByID(stID)
	if err != nil {
		return fmt.Errorf("scheduled transaction not found: %w", err)
	}

	oldNextDate := st.NextDate

	var amount *types.Money
	if opts.amount != "" {
		amt, err := types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		amount = &amt
	}

	var date *types.Date
	if opts.date != "" {
		d, err := types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
		date = &d
	}

	var txn *transaction.Transaction
	if date != nil {
		txn, err = svc.Scheduled.PostWithDate(stID, *date, amount)
	} else {
		txn, err = svc.Scheduled.Post(stID, amount)
	}
	if err != nil {
		return fmt.Errorf("failed to post scheduled transaction: %w", err)
	}

	stUpdated, _ := svc.Scheduled.GetByID(stID)

	acct, _ := svc.AccountRepo.GetByID(st.AccountID)
	currency := "USD"
	accountName := "Unknown"
	if acct != nil {
		currency = acct.Currency
		accountName = acct.Name
	}

	payeeName := "-"
	if st.HasPayee() {
		py, err := svc.PayeeRepo.GetByID(st.PayeeID.ID)
		if err == nil {
			payeeName = py.Name
		}
	}

	// A posted occurrence does not always produce a regular-ledger row. An
	// investment-to-investment transfer writes BOTH its legs to
	// investment_transactions, so the post path correctly reports no
	// transaction — there is none to report. The occurrence still happened, so
	// fall back to what the schedule was asked to post rather than dereferencing
	// a nil result.
	postedAmount := types.ZeroMoney
	if st.Amount.Valid {
		postedAmount = st.Amount.Money
	}
	if amount != nil {
		postedAmount = *amount
		// A transfer schedule stores its amount as the signed effect on the
		// SOURCE account, and posting takes the magnitude of whatever override is
		// given. Normalise so the summary reads the way the schedule and the
		// register do, whichever sign the user typed.
		if st.IsTransfer() {
			postedAmount = amount.Abs().Neg()
		}
	}
	postedDate := oldNextDate
	if date != nil {
		postedDate = *date
	}
	if txn != nil {
		postedDate = txn.Date
		// For a transfer, the returned row is whichever leg landed on the REGULAR
		// ledger, which is the destination when the source is an investment
		// account — so its sign belongs to a different account than the one named
		// above. The schedule's own signed amount is the honest figure here.
		if !st.IsTransfer() {
			postedAmount = txn.Amount
		}
	}

	fmt.Fprintln(w, "Scheduled transaction posted successfully!")
	fmt.Fprintf(w, "  Account:     %s\n", accountName)
	if payeeName != "-" {
		fmt.Fprintf(w, "  Payee:       %s\n", payeeName)
	}
	fmt.Fprintf(w, "  Amount:      %s\n", cmdutil.FormatMoney(postedAmount, currency))
	fmt.Fprintf(w, "  Date:        %s\n", postedDate.String())
	// State the direction rather than leaving it to be inferred from a sign.
	if st.IsTransfer() {
		destName := "Unknown"
		if dest, derr := svc.AccountRepo.GetByID(st.TransferAccountID.ID); derr == nil && dest != nil {
			destName = dest.Name
		}
		fmt.Fprintf(w, "  Transfer to: %s\n", destName)
	}
	fmt.Fprintf(w, "  Frequency:   %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Previous:    %s\n", oldNextDate.String())
	if stUpdated != nil && !stUpdated.IsCompleted() {
		fmt.Fprintf(w, "  Next:        %s\n", stUpdated.NextDate.String())
	} else {
		fmt.Fprintln(w, "  Status:      Completed (no more occurrences)")
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}
