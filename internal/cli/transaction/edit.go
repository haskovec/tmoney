package transaction

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// transactionEditOptions are the inputs to `tmoney transaction edit`.
// The *Changed booleans record which editable flags were supplied so the
// command can apply delta semantics (only supplied flags take effect).
type transactionEditOptions struct {
	file     string
	txnID    string
	amount   string
	date     string
	payee    string
	category string
	memo     string
	status   string

	amountChanged   bool
	dateChanged     bool
	payeeChanged    bool
	categoryChanged bool
	memoChanged     bool
	statusChanged   bool
}

// newTransactionEditCmd registers `tmoney transaction edit`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--txn-id` is required; at least one editable flag
// must be supplied and only supplied flags take effect (matching
// `transfer edit` / `security edit` / `investment edit`).
func newTransactionEditCmd() *cobra.Command {
	opts := &transactionEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing transaction",
		Long: "Edit a transaction identified by its UUID (find it with " +
			"`tmoney transaction list --show-ids`). Only the supplied flags " +
			"take effect; pass an empty string to `--payee`, `--category`, or " +
			"`--memo` to clear that field. Transfer legs are edited with " +
			"`tmoney transfer edit`; split transactions are edited in the TUI; " +
			"void and reconciled transactions are refused.",
		Example: "  tmoney transaction edit --txn-id <uuid> --amount -45.50\n" +
			"  tmoney transaction edit --txn-id <uuid> --category \"Food:Groceries\" --status cleared",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.amountChanged = cmd.Flags().Changed("amount")
			opts.dateChanged = cmd.Flags().Changed("date")
			opts.payeeChanged = cmd.Flags().Changed("payee")
			opts.categoryChanged = cmd.Flags().Changed("category")
			opts.memoChanged = cmd.Flags().Changed("memo")
			opts.statusChanged = cmd.Flags().Changed("status")
			return runTransactionEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.txnID, "txn-id", "", "UUID of the transaction to edit (required)")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "New amount; negative for expenses")
	cmd.Flags().StringVar(&opts.date, "date", "", "New transaction date YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "New payee name, auto-created if it doesn't exist (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.category, "category", "", "New category name, Parent or Parent:Subcategory (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "New memo (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.status, "status", "", "New status: cleared or uncleared (reconciling is done with `tmoney reconcile`)")
	_ = cmd.MarkFlagRequired("txn-id")
	return cmd
}

// runTransactionEdit executes `tmoney transaction edit`: field edits go
// through transaction.Service.Update (the same path the TUI edit dialog
// uses); a status change goes through the narrow status-only service
// methods so DuckDB does not rewrite the row.
func runTransactionEdit(opts *transactionEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	if !opts.amountChanged && !opts.dateChanged && !opts.payeeChanged &&
		!opts.categoryChanged && !opts.memoChanged && !opts.statusChanged {
		return fmt.Errorf("at least one editable flag is required (--amount, --date, --payee, --category, --memo, --status)")
	}

	txnID, err := types.ParseID(opts.txnID)
	if err != nil {
		return fmt.Errorf("invalid --txn-id: %w", err)
	}

	newStatus, err := parseEditStatus(opts)
	if err != nil {
		return err
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	txn, err := svc.Transaction.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("transaction %s not found", opts.txnID)
	}

	if err := guardTransactionEditable(svc, txn, "edited"); err != nil {
		return err
	}

	fieldEdit := opts.amountChanged || opts.dateChanged || opts.payeeChanged ||
		opts.categoryChanged || opts.memoChanged
	if fieldEdit {
		if err := applyFieldEdits(svc, txn, opts); err != nil {
			return err
		}
		if err := svc.Transaction.Update(txn); err != nil {
			return fmt.Errorf("failed to update transaction: %w", err)
		}
	}

	if opts.statusChanged {
		switch newStatus {
		case transactiondom.StatusCleared:
			err = svc.Transaction.ClearTransaction(txnID)
		case transactiondom.StatusUncleared:
			err = svc.Transaction.MarkTransactionUncleared(txnID)
		}
		if err != nil {
			return fmt.Errorf("failed to update transaction status: %w", err)
		}
		txn.Status = newStatus
	}

	printTransactionSummary(w, svc, "Transaction updated successfully!", txn)

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// parseEditStatus validates the --status flag value. Only the
// cleared/uncleared toggle is scriptable here; reconciled state belongs
// to `tmoney reconcile` and void to `tmoney transaction void`.
func parseEditStatus(opts *transactionEditOptions) (transactiondom.Status, error) {
	if !opts.statusChanged {
		return "", nil
	}
	status, err := transactiondom.ParseStatus(opts.status)
	if err != nil {
		return "", fmt.Errorf("invalid --status: %w (use cleared or uncleared)", err)
	}
	switch status {
	case transactiondom.StatusCleared, transactiondom.StatusUncleared:
		return status, nil
	case transactiondom.StatusReconciled:
		return "", fmt.Errorf("--status reconciled is not allowed; reconciling is done with `tmoney reconcile`")
	default:
		return "", fmt.Errorf("--status void is not allowed; use `tmoney transaction void`")
	}
}

// guardTransactionEditable rejects transactions this command must not
// touch: transfer legs (owned by `transfer edit`/`transfer delete`),
// split parents (TUI-only until split editing lands in the CLI), and
// void or reconciled rows. verb is "edited" or "deleted" for the message.
func guardTransactionEditable(svc *app.Services, txn *transactiondom.Transaction, verb string) error {
	if txn.IsTransfer() {
		if verb == "deleted" {
			return fmt.Errorf("transaction is part of a transfer; use `tmoney transfer delete`")
		}
		return fmt.Errorf("transaction is part of a transfer; use `tmoney transfer edit`")
	}
	splits, err := svc.Transaction.GetSplits(txn.ID)
	if err != nil {
		return fmt.Errorf("failed to check for splits: %w", err)
	}
	if len(splits) > 0 {
		return fmt.Errorf("transaction is a split transaction (%d lines); split transactions can only be %s in the TUI", len(splits), verb)
	}
	if txn.IsVoid() {
		return fmt.Errorf("transaction is void and cannot be %s", verb)
	}
	if txn.IsReconciled() {
		return fmt.Errorf("transaction is reconciled and cannot be %s; unreconcile it first", verb)
	}
	return nil
}

// applyFieldEdits mutates txn in place with the supplied delta flags.
// Payees auto-create (same as `transaction add`); categories must exist.
func applyFieldEdits(svc *app.Services, txn *transactiondom.Transaction, opts *transactionEditOptions) error {
	if opts.amountChanged {
		amount, err := types.NewMoney(opts.amount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		txn.Amount = amount
		txn.Touch()
	}
	if opts.dateChanged {
		date, err := types.ParseDate(opts.date)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
		txn.Date = date
		txn.Touch()
	}
	if opts.payeeChanged {
		if opts.payee == "" {
			txn.ClearPayee()
		} else {
			py, _, err := svc.Payee.GetOrCreate(opts.payee)
			if err != nil {
				return fmt.Errorf("failed to resolve payee: %w", err)
			}
			txn.SetPayee(py.ID)
		}
	}
	if opts.categoryChanged {
		if opts.category == "" {
			txn.ClearCategory()
		} else {
			cat, err := resolveCategoryByName(svc, opts.category)
			if err != nil {
				return err
			}
			txn.SetCategory(cat.ID)
		}
	}
	if opts.memoChanged {
		txn.SetMemo(opts.memo)
	}
	return nil
}

// resolveCategoryByName finds a category by name, first as a top-level
// category and then across all categories (so subcategory display names
// like "Food:Groceries" resolve). Shared by `transaction add` and
// `transaction edit`.
func resolveCategoryByName(svc *app.Services, name string) (*category.Category, error) {
	cat, err := svc.CategoryRepo.GetByName(name, nil)
	if err == nil {
		return cat, nil
	}
	categories, listErr := svc.CategoryRepo.List()
	if listErr != nil {
		return nil, fmt.Errorf("category %q not found", name)
	}
	for _, c := range categories {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, fmt.Errorf("category %q not found", name)
}

// printTransactionSummary prints a confirmation block for the given
// transaction, resolving account/payee/category names for display.
func printTransactionSummary(w io.Writer, svc *app.Services, header string, txn *transactiondom.Transaction) {
	accountName := "Unknown"
	currency := "USD"
	if acct, err := svc.Account.GetByID(txn.AccountID); err == nil {
		accountName = acct.Name
		currency = acct.Currency
	}

	fmt.Fprintln(w, header)
	fmt.Fprintf(w, "  Account:  %s\n", accountName)
	fmt.Fprintf(w, "  Date:     %s\n", txn.Date.String())
	fmt.Fprintf(w, "  Amount:   %s\n", cmdutil.FormatMoney(txn.Amount, currency))
	if txn.PayeeID.Valid {
		if py, err := svc.PayeeRepo.GetByID(txn.PayeeID.ID); err == nil {
			fmt.Fprintf(w, "  Payee:    %s\n", py.Name)
		}
	}
	if txn.CategoryID.Valid {
		if cat, err := svc.CategoryRepo.GetByID(txn.CategoryID.ID); err == nil {
			fmt.Fprintf(w, "  Category: %s\n", cat.Name)
		}
	}
	if txn.Memo.Valid {
		fmt.Fprintf(w, "  Memo:     %s\n", txn.Memo.String)
	}
	fmt.Fprintf(w, "  Status:   %s\n", txn.Status.DisplayName())
}
