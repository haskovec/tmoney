package transfer

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	xfer "github.com/haskovec/tmoney/internal/transfer"
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
	category    string
}

// newTransferAddCmd registers `tmoney transfer add`. The database file
// is taken from the persistent `--file` / `-f` flag inherited from the
// root command. `--from`, `--to`, and `--amount` are required.
func newTransferAddCmd() *cobra.Command {
	opts := &transferAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Create a transfer between two accounts",
		Long: "Create a transfer between two accounts. The command dispatches by the " +
			"(from, to) account types so any combination works: bank↔bank, bank↔investment, " +
			"and investment↔investment (including HSA on either leg). " +
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
	cmd.Flags().StringVar(&opts.category, "category", "", "Optional existing non-system category to label the transfer (e.g. \"Bills:Credit Card\"); not supported for investment-to-investment transfers")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// runTransferAdd creates a transfer between two accounts. Every (from, to)
// combination goes through the one transfer service, which derives each leg's
// sign from its side and each leg's table from its own account type.
func runTransferAdd(opts *transferAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	amount, err := types.NewMoney(opts.amount)
	if err != nil {
		return fmt.Errorf("invalid --amount: %w", err)
	}
	if !amount.IsPositive() {
		return fmt.Errorf("--amount must be positive for transfers")
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

	// Reject an unstorable category before resolving the name, so the error
	// names the real limitation rather than an incidental "category not found".
	//
	// This CALLS the domain predicate (Kind.StoresCategory) rather than
	// restating the rule, so there is one implementation. transfer.Service
	// refuses independently with *transfer.CategoryNotSupportedError.
	if strings.TrimSpace(opts.category) != "" &&
		!xfer.ClassifyKind(fromAcct.Type, toAcct.Type).StoresCategory() {
		return fmt.Errorf("--category is not supported for investment-to-investment transfers")
	}

	categoryID, err := resolveTransferCategory(svc, opts.category)
	if err != nil {
		return err
	}

	// One call for every (From, To) combination. The 60-line dispatchTransferAdd
	// switch that used to be here -- four service methods, three result shapes,
	// and an argument flip for DepositFromAccount's reversed parameter order --
	// is gone.
	result, err := svc.Transfer.Create(xfer.Spec{
		FromAccountID: fromAcct.ID,
		ToAccountID:   toAcct.ID,
		Date:          date,
		Amount:        amount,
		Memo:          opts.memo,
		CategoryID:    categoryID,
	})
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	fmt.Fprintln(w, "Transfer created successfully!")
	fmt.Fprintf(w, "  Transfer ID:           %s\n", result.TransferID)
	fmt.Fprintf(w, "  From transaction ID:   %s\n", result.From.RowID)
	fmt.Fprintf(w, "  To transaction ID:     %s\n", result.To.RowID)
	fmt.Fprintf(w, "  From:                  %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:                    %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Date:                  %s\n", date.String())
	fmt.Fprintf(w, "  Amount:                %s\n", cmdutil.FormatMoney(amount, fromAcct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:                  %s\n", opts.memo)
	}
	if categoryID.Valid {
		fmt.Fprintf(w, "  Category:              %s\n", strings.TrimSpace(opts.category))
	}

	cmdutil.AutoBackupAfterModification(database)
	return nil
}
