package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// scheduledSkipOptions are the inputs to `tmoney scheduled skip`.
type scheduledSkipOptions struct {
	file string
	id   string
}

// newScheduledSkipCmd registers `tmoney scheduled skip <id>`. The
// database file is taken from the persistent `--file` / `-f` flag
// inherited from the root command.
func newScheduledSkipCmd() *cobra.Command {
	opts := &scheduledSkipOptions{}
	cmd := &cobra.Command{
		Use:   "skip <id>",
		Short: "Skip a scheduled transaction's next occurrence",
		Long: "Advance a scheduled transaction to its next occurrence " +
			"without creating a real transaction for the current one.",
		Example:      "  tmoney scheduled skip <id> --file personal.tdb",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.id = args[0]
			return runScheduledSkip(opts, cmd.OutOrStdout())
		},
	}
	return cmd
}

// runScheduledSkip skips a scheduled transaction (advances to next date without posting).
func runScheduledSkip(opts *scheduledSkipOptions, w io.Writer) error {
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

	if err := svc.Scheduled.Skip(stID); err != nil {
		return fmt.Errorf("failed to skip scheduled transaction: %w", err)
	}

	stUpdated, _ := svc.Scheduled.GetByID(stID)

	acct, _ := svc.AccountRepo.GetByID(st.AccountID)
	accountName := "Unknown"
	if acct != nil {
		accountName = acct.Name
	}

	payeeName := "-"
	if st.HasPayee() {
		py, err := svc.PayeeRepo.GetByID(st.PayeeID.ID)
		if err == nil {
			payeeName = py.Name
		}
	}

	fmt.Fprintln(w, "Scheduled transaction skipped!")
	fmt.Fprintf(w, "  Account:     %s\n", accountName)
	if payeeName != "-" {
		fmt.Fprintf(w, "  Payee:       %s\n", payeeName)
	}
	fmt.Fprintf(w, "  Frequency:   %s\n", st.Frequency.DisplayName())
	fmt.Fprintf(w, "  Skipped:     %s\n", oldNextDate.String())
	if stUpdated != nil && !stUpdated.IsCompleted() {
		fmt.Fprintf(w, "  Next:        %s\n", stUpdated.NextDate.String())
	} else {
		fmt.Fprintln(w, "  Status:      Completed (no more occurrences)")
	}

	autoBackupAfterModification(opts.file)
	return nil
}
