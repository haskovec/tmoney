package transfer

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transferEditOptions are the inputs to `tmoney transfer edit`.
type transferEditOptions struct {
	file   string
	txnID  string
	amount string
	date   string
	memo   string
	status string

	// changed flags track which editable fields the user actually supplied,
	// so only those override the existing values (matches `security edit`).
	amountChanged bool
	dateChanged   bool
	memoChanged   bool
	statusChanged bool
}

// newTransferEditCmd registers `tmoney transfer edit`. The database file is
// taken from the persistent `--file` / `-f` flag. `--txn-id` (the UUID of
// either leg) is required, plus at least one editable flag.
func newTransferEditCmd() *cobra.Command {
	opts := &transferEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a transfer's amount, date, memo, or status",
		Long: "Edit a whole-transaction transfer, identified by the UUID of either leg " +
			"(`--txn-id`). Only supplied flags take effect; at least one of `--amount`, " +
			"`--date`, `--memo`, or `--status` is required. From/To accounts are not " +
			"editable (delete and re-add to move accounts). `--status` accepts " +
			"`cleared` or `uncleared`; reconciling is owned by `tmoney reconcile`. " +
			"Works for every account-type combination.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.amountChanged = cmd.Flags().Changed("amount")
			opts.dateChanged = cmd.Flags().Changed("date")
			opts.memoChanged = cmd.Flags().Changed("memo")
			opts.statusChanged = cmd.Flags().Changed("status")
			return runTransferEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.txnID, "txn-id", "", "UUID of either leg of the transfer (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "New transfer amount; must be positive")
	cmd.Flags().StringVar(&opts.date, "date", "", "New transfer date YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "New memo")
	cmd.Flags().StringVar(&opts.status, "status", "", "New status: cleared or uncleared")
	_ = cmd.MarkFlagRequired("txn-id")
	return cmd
}

// runTransferEdit updates a transfer's editable fields. The (from, to) account
// types pick which service method applies the update.
func runTransferEdit(opts *transferEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	if !opts.amountChanged && !opts.dateChanged && !opts.memoChanged && !opts.statusChanged {
		return fmt.Errorf("specify at least one of --amount, --date, --memo, --status")
	}

	legID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid --txn-id: %w", err)
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	res, err := resolveTransferPair(svc, legID)
	if err != nil {
		return err
	}

	if res.status == transaction.StatusReconciled {
		return fmt.Errorf("transfer is reconciled and cannot be edited; unreconcile it first")
	}

	// Compute the post-edit values: existing values, overridden by supplied flags.
	amount := res.amount
	if opts.amountChanged {
		amount, err = types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		if !amount.IsPositive() {
			return fmt.Errorf("--amount must be positive for transfers")
		}
	}

	date := res.date
	if opts.dateChanged {
		date, err = types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	}

	memo := res.memo
	if opts.memoChanged {
		memo = opts.memo
	}

	status := res.status
	if opts.statusChanged {
		status, err = parseEditStatus(opts.status)
		if err != nil {
			return err
		}
	}

	if err := dispatchTransferEdit(svc, res, date, amount, memo, status); err != nil {
		return fmt.Errorf("failed to update transfer: %w", err)
	}

	fmt.Fprintln(w, "Transfer updated successfully!")
	fmt.Fprintf(w, "  From:     %s\n", res.fromAccount.Name)
	fmt.Fprintf(w, "  To:       %s\n", res.toAccount.Name)
	fmt.Fprintf(w, "  Date:     %s\n", date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", cmdutil.FormatMoney(amount, res.fromAccount.Currency))
	fmt.Fprintf(w, "  Status:   %s\n", status)
	if memo != "" {
		fmt.Fprintf(w, "  Memo:     %s\n", memo)
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// parseEditStatus parses the --status flag for transfer edit. Only cleared and
// uncleared are accepted; reconciling is owned by the reconcile workflow.
func parseEditStatus(s string) (transaction.Status, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "cleared":
		return transaction.StatusCleared, nil
	case "uncleared":
		return transaction.StatusUncleared, nil
	case "reconciled":
		return "", fmt.Errorf("--status reconciled is not allowed here; use `tmoney reconcile` to reconcile transactions")
	default:
		return "", fmt.Errorf("invalid --status %q: want cleared or uncleared", s)
	}
}

// dispatchTransferEdit applies the update through the right service method for
// the resolved transfer's dispatch kind.
func dispatchTransferEdit(svc *app.Services, res *resolvedTransfer, date types.Date, amount types.Money, memo string, status transaction.Status) error {
	if res.kind == transaction.DispatchRegToReg {
		// res.categoryID preserves the existing category; `transfer edit` gains a
		// --category flag in a later phase to change or clear it.
		return svc.Transaction.UpdateTransfer(res.transferID, date, amount, memo, status, res.categoryID)
	}

	// Inv-involving: UpdateTransferCash takes the investment-side leg plus the
	// investment account and a direction ("out" = cash leaves the investment
	// account, "in" = cash arrives at it).
	var investmentAccountID, otherAccountID types.ID
	var direction string
	if res.fromAccount.Type.IsInvestmentType() {
		investmentAccountID = res.fromAccount.ID
		otherAccountID = res.toAccount.ID
		direction = "out"
	} else {
		investmentAccountID = res.toAccount.ID
		otherAccountID = res.fromAccount.ID
		direction = "in"
	}

	_, err := svc.Investment.UpdateTransferCash(
		res.investmentTxnID,
		investmentAccountID,
		otherAccountID,
		date,
		amount,
		memo,
		res.categoryID,
		direction,
		status,
	)
	return err
}
