package transfer

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
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
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

// transferAddResult is the format-agnostic result of any dispatched
// transfer create: the shared transfer_id and the two leg transaction IDs,
// laid out as "from" and "to" matching the user's --from / --to flags.
type transferAddResult struct {
	TransferID types.ID
	FromTxnID  types.ID
	ToTxnID    types.ID
}

// runTransferAdd creates a transfer between two accounts. The (from, to)
// account types pick one of four service methods (see
// transaction.ChooseTransferDispatch).
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

	result, err := dispatchTransferAdd(svc, fromAcct, toAcct, date, amount, opts.memo)
	if err != nil {
		return fmt.Errorf("failed to create transfer: %w", err)
	}

	fmt.Fprintln(w, "Transfer created successfully!")
	fmt.Fprintf(w, "  Transfer ID:           %s\n", result.TransferID)
	fmt.Fprintf(w, "  From transaction ID:   %s\n", result.FromTxnID)
	fmt.Fprintf(w, "  To transaction ID:     %s\n", result.ToTxnID)
	fmt.Fprintf(w, "  From:                  %s\n", fromAcct.Name)
	fmt.Fprintf(w, "  To:                    %s\n", toAcct.Name)
	fmt.Fprintf(w, "  Date:                  %s\n", date.String())
	fmt.Fprintf(w, "  Amount:                %s\n", cmdutil.FormatMoney(amount, fromAcct.Currency))
	if opts.memo != "" {
		fmt.Fprintf(w, "  Memo:                  %s\n", opts.memo)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// dispatchTransferAdd picks the right service method for the
// (from.Type, to.Type) combination and returns the leg IDs in
// caller-supplied (from, to) order.
func dispatchTransferAdd(svc *app.Services, from, to *account.Account, date types.Date, amount types.Money, memo string) (*transferAddResult, error) {
	switch transaction.ChooseTransferDispatch(from.Type, to.Type) {
	case transaction.DispatchRegToReg:
		// CreateTransfer stamps the memo on both legs directly. A category is
		// not settable from `transfer add` yet (arrives in a later phase); an
		// empty NullableID means no category.
		pair, err := svc.Transaction.CreateTransfer(from.ID, to.ID, date, amount, memo, types.NullableID{})
		if err != nil {
			return nil, err
		}
		return &transferAddResult{
			TransferID: pair.FromTransaction.TransferID.ID,
			FromTxnID:  pair.FromTransaction.ID,
			ToTxnID:    pair.ToTransaction.ID,
		}, nil
	case transaction.DispatchRegToInv:
		// DepositFromAccount signature: (investmentID, regularID, date, amount, memo, categoryID).
		// "From" is the regular account; "To" is the investment account.
		res, err := svc.Investment.DepositFromAccount(to.ID, from.ID, date, amount, memo, types.NullableID{})
		if err != nil {
			return nil, err
		}
		return &transferAddResult{
			TransferID: res.TransferID,
			FromTxnID:  res.RegularTransaction.ID,
			ToTxnID:    res.InvestmentTransaction.ID,
		}, nil
	case transaction.DispatchInvToReg:
		// TransferCash signature: (investmentID, regularID, date, amount, memo, categoryID).
		// "From" is the investment account; "To" is the regular account.
		res, err := svc.Investment.TransferCash(from.ID, to.ID, date, amount, memo, types.NullableID{})
		if err != nil {
			return nil, err
		}
		return &transferAddResult{
			TransferID: res.TransferID,
			FromTxnID:  res.InvestmentTransaction.ID,
			ToTxnID:    res.RegularTransaction.ID,
		}, nil
	case transaction.DispatchInvToInv:
		res, err := svc.Investment.TransferCashBetweenInvestments(from.ID, to.ID, date, amount, memo)
		if err != nil {
			return nil, err
		}
		return &transferAddResult{
			TransferID: res.TransferID,
			FromTxnID:  res.SourceTransaction.ID,
			ToTxnID:    res.DestinationTransaction.ID,
		}, nil
	default:
		return nil, fmt.Errorf("unknown transfer dispatch kind")
	}
}
