package cli

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transferAddOptions are the inputs to `tmoney transfer add`.
type transferAddOptions struct {
	file        string
	fromAccount string
	toAccount   string
	amount      string
	date        string
	memo        string
}

// newTransferAddCmd registers `tmoney transfer add`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--from`, `--to`, and `--amount` are required.
func newTransferAddCmd() *cobra.Command {
	opts := &transferAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a transfer between two accounts",
		Long: "Create a transfer between two accounts. " +
			"`--from`, `--to`, and `--amount` are required; `--amount` must be positive.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runTransferAdd(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.fromAccount, "from", "", "Source account name (required)")
	cmd.Flags().StringVar(&opts.toAccount, "to", "", "Destination account name (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "Transfer amount; must be positive (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "Transfer date YYYY-MM-DD (default today)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "Free-form memo")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runTransferAdd creates a transfer between two accounts.
func runTransferAdd(opts *transferAddOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--file is required to specify a database")
	}

	// Parse amount
	amount, err := types.NewMoney(opts.amount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}

	// Amount must be positive for transfers
	if !amount.IsPositive() {
		return fmt.Errorf("--amount must be positive for transfers")
	}

	// Parse date (default to today)
	var date types.Date
	if opts.date != "" {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		date = types.Today()
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get source account by name
	fromAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("source account %q not found", opts.fromAccount)
	}

	// Get destination account by name
	toAcct, err := svc.Account.GetByName(opts.toAccount)
	if err != nil {
		return fmt.Errorf("destination account %q not found", opts.toAccount)
	}

	// Create the transfer
	pair, err := svc.Transaction.CreateTransfer(fromAcct.ID, toAcct.ID, date, amount)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	// Set memo if provided
	if opts.memo != "" {
		err = svc.Transaction.UpdateTransfer(pair.FromTransaction.TransferID.ID, date, amount, opts.memo, transaction.StatusUncleared)
		if err != nil {
			return fmt.Errorf("failed to set memo on transfer: %w", err)
		}
	}

	// Print confirmation
	fmt.Fprintln(w, "Transfer created successfully!")
	fmt.Fprintf(w, "  From:   %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:     %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Date:   %s\n", date.String())
	fmt.Fprintf(w, "  Amount: %s\n", formatMoney(amount, fromAcct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:   %s\n", opts.memo)
	}

	autoBackupAfterModification(opts.file)
	return nil
}
