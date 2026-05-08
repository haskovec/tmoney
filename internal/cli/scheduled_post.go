package cli

import (
	"fmt"
	"io"

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
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	database, svc, err := openServices(opts.file)
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

	fmt.Fprintln(w, "Scheduled transaction posted successfully!")
	fmt.Fprintf(w, "  Account:     %s\n", accountName)
	if payeeName != "-" {
		fmt.Fprintf(w, "  Payee:       %s\n", payeeName)
	}
	fmt.Fprintf(w, "  Amount:      %s\n", formatMoney(txn.Amount, currency))
	fmt.Fprintf(w, "  Date:        %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Frequency:   %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Previous:    %s\n", oldNextDate.String())
	if stUpdated != nil && !stUpdated.IsCompleted() {
		fmt.Fprintf(w, "  Next:        %s\n", stUpdated.NextDate.String())
	} else {
		fmt.Fprintln(w, "  Status:      Completed (no more occurrences)")
	}

	autoBackupAfterModification(opts.file)
	return nil
}
