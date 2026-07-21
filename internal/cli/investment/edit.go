package investment

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// investmentEditOptions are the inputs to `tmoney investment edit`.
// The *Changed booleans record which editable flags were supplied so the
// command can apply delta semantics (only supplied flags take effect).
type investmentEditOptions struct {
	file          string
	txnID         string
	date          string
	shares        string
	amount        string
	pricePerShare string
	commission    string
	memo          string
	status        string
	lot           string

	dateChanged       bool
	sharesChanged     bool
	amountChanged     bool
	priceChanged      bool
	commissionChanged bool
	memoChanged       bool
	statusChanged     bool
}

// newInvestmentEditCmd registers `tmoney investment edit`. The database
// file is taken from the persistent `--file` / `-f` flag inherited from
// the root command. `--txn-id` is required; at least one editable flag
// must be supplied and only supplied flags take effect (matching
// `transfer edit` / `security edit`).
func newInvestmentEditCmd() *cobra.Command {
	opts := &investmentEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing investment transaction",
		Long: "Edit an investment transaction identified by its UUID (find it " +
			"with `tmoney investment list --show-ids`). Only the supplied flags " +
			"take effect. Editing a buy/sell/reinvest/fee-liquidation reverses " +
			"the old position/lot effect and re-applies the new values — the " +
			"same path the TUI edit dialog uses. `--status cleared|pending` is " +
			"the scriptable register `c` key and goes through a narrow " +
			"status-only update. Transfer legs are edited with " +
			"`tmoney transfer edit`; reconciled transactions are refused.",
		Example: "  tmoney investment edit --txn-id <uuid> --shares 1.587\n" +
			"  tmoney investment edit --txn-id <uuid> --date 2024-01-16 --memo \"fixed date\"\n" +
			"  tmoney investment edit --txn-id <uuid> --status cleared",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.dateChanged = cmd.Flags().Changed("date")
			opts.sharesChanged = cmd.Flags().Changed("shares")
			opts.amountChanged = cmd.Flags().Changed("amount")
			opts.priceChanged = cmd.Flags().Changed("price-per-share")
			opts.commissionChanged = cmd.Flags().Changed("commission")
			opts.memoChanged = cmd.Flags().Changed("memo")
			opts.statusChanged = cmd.Flags().Changed("status")
			return runInvestmentEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.txnID, "txn-id", "", "UUID of the investment transaction to edit (required)")
	cmd.Flags().StringVar(&opts.date, "date", "", "New transaction date YYYY-MM-DD")
	cmd.Flags().StringVar(&opts.shares, "shares", "", "New number of shares")
	cmd.Flags().StringVar(&opts.amount, "amount", "", "New total amount (positive magnitude, like the add commands)")
	cmd.Flags().StringVar(&opts.pricePerShare, "price-per-share", "", "New price per share")
	cmd.Flags().StringVar(&opts.commission, "commission", "", "New commission amount")
	cmd.Flags().StringVar(&opts.memo, "memo", "", "New memo (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.status, "status", "", "New status: cleared or pending (reconciling is done with `tmoney reconcile`)")
	cmd.Flags().StringVar(&opts.lot, "lot", "", "Lot ID to allocate a sell/fee-liquidation against (required to edit those on lot-tracked accounts)")
	_ = cmd.MarkFlagRequired("txn-id")
	return cmd
}

// editedValues holds the merged old/new field values for the rebuilt
// transaction.
type editedValues struct {
	date           types.Date
	shares         types.Quantity
	totalAmount    *types.Money
	price          *types.Money
	commission     types.Money
	amount         types.Money // cash-type magnitude
	memo           string
	lotAllocations []investmentdom.SellLotAllocation
}

// runInvestmentEdit executes `tmoney investment edit`: field edits
// dispatch on the stored transaction type to the matching
// Service.Update* method; a status change goes through the narrow
// SetClearedStatus path so a status-only edit does not rewrite the row.
func runInvestmentEdit(opts *investmentEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}
	fieldEdit := opts.dateChanged || opts.sharesChanged || opts.amountChanged ||
		opts.priceChanged || opts.commissionChanged || opts.memoChanged
	if !fieldEdit && !opts.statusChanged {
		return fmt.Errorf("at least one editable flag is required (--date, --shares, --amount, --price-per-share, --commission, --memo, --status)")
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

	old, err := svc.InvestmentRepo.GetByID(txnID)
	if err != nil {
		return fmt.Errorf("investment transaction %s not found", opts.txnID)
	}

	if err := guardEditable(old, opts); err != nil {
		return err
	}

	newTxn := old
	if fieldEdit {
		vals, err := mergeEditValues(old, opts)
		if err != nil {
			return err
		}

		if opts.lot != "" {
			lotID, err := types.ParseID(opts.lot)
			if err != nil {
				return fmt.Errorf("invalid --lot: %w", err)
			}
			vals.lotAllocations = []investmentdom.SellLotAllocation{
				{LotID: lotID, Shares: vals.shares},
			}
		}

		newTxn, err = dispatchUpdate(svc.Investment, old, vals)
		if err != nil {
			return fmt.Errorf("failed to update %s transaction: %w", old.Type.DisplayName(), err)
		}
	}

	finalStatus := newTxn.Status
	switch {
	case opts.statusChanged:
		if err := svc.Investment.SetClearedStatus(newTxn.ID, newStatus == investmentdom.TransactionStatusCleared); err != nil {
			return fmt.Errorf("failed to update transaction status: %w", err)
		}
		finalStatus = newStatus
	case fieldEdit && old.Status == investmentdom.TransactionStatusCleared:
		// The update recreates the row (new ID, default pending); carry a
		// cleared status over so the edit doesn't silently unclear the entry.
		if err := svc.Investment.SetClearedStatus(newTxn.ID, true); err != nil {
			return fmt.Errorf("transaction updated but restoring cleared status failed: %w", err)
		}
		finalStatus = investmentdom.TransactionStatusCleared
	}

	acct, err := svc.Account.GetByID(old.AccountID)
	currency := "USD"
	if err == nil {
		currency = acct.Currency
	}

	fmt.Fprintln(w, "Investment transaction updated successfully!")
	fmt.Fprintf(w, "  Type:     %s\n", newTxn.Type.DisplayName())
	fmt.Fprintf(w, "  Date:     %s\n", newTxn.Date.String())
	if newTxn.Shares.Valid {
		fmt.Fprintf(w, "  Shares:   %s\n", newTxn.Shares.Quantity.String())
	}
	if newTxn.PricePerShare.Valid {
		fmt.Fprintf(w, "  Price:    %s\n", cmdutil.FormatMoney(newTxn.PricePerShare.Money, currency))
	}
	fmt.Fprintf(w, "  Total:    %s\n", cmdutil.FormatMoney(newTxn.TotalAmount, currency))
	fmt.Fprintf(w, "  Status:   %s\n", finalStatus.DisplayName())
	if fieldEdit {
		fmt.Fprintf(w, "  New ID:   %s\n", newTxn.ID.String())
	}

	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// parseEditStatus validates the --status flag value. Only the
// cleared/pending toggle is scriptable here; reconciled state belongs
// to `tmoney reconcile`.
func parseEditStatus(opts *investmentEditOptions) (investmentdom.TransactionStatus, error) {
	if !opts.statusChanged {
		return "", nil
	}
	status, err := investmentdom.ParseTransactionStatus(opts.status)
	if err != nil {
		return "", fmt.Errorf("invalid --status: %w (use cleared or pending)", err)
	}
	if status == investmentdom.TransactionStatusReconciled {
		return "", fmt.Errorf("--status reconciled is not allowed; reconciling is done with `tmoney reconcile`")
	}
	return status, nil
}

// guardEditable rejects transactions this command must not touch and
// flags that don't apply to the transaction's type.
func guardEditable(old *investmentdom.Transaction, opts *investmentEditOptions) error {
	switch old.Type {
	case investmentdom.TransactionTypeTransferCash, investmentdom.TransactionTypeTransferShares:
		return fmt.Errorf("transaction is part of a transfer; use `tmoney transfer edit` for cash transfers, or the TUI for share transfers")
	case investmentdom.TransactionTypeExchange:
		return fmt.Errorf("exchange transactions are created by corporate actions and cannot be edited")
	}
	if old.TransferID.Valid {
		return fmt.Errorf("transaction is part of a transfer; use `tmoney transfer edit`")
	}
	if old.Status == investmentdom.TransactionStatusReconciled {
		return fmt.Errorf("transaction is reconciled and cannot be edited; unreconcile it first")
	}

	typeName := old.Type.String()
	if opts.sharesChanged && !old.Type.RequiresShares() {
		return fmt.Errorf("--shares does not apply to a %s transaction", typeName)
	}
	if opts.priceChanged && !old.Type.RequiresShares() {
		return fmt.Errorf("--price-per-share does not apply to a %s transaction", typeName)
	}
	if opts.commissionChanged {
		switch old.Type {
		case investmentdom.TransactionTypeBuy, investmentdom.TransactionTypeSell,
			investmentdom.TransactionTypeFeeLiquidation:
		default:
			return fmt.Errorf("--commission does not apply to a %s transaction", typeName)
		}
	}
	if opts.lot != "" {
		switch old.Type {
		case investmentdom.TransactionTypeSell, investmentdom.TransactionTypeFeeLiquidation:
		default:
			return fmt.Errorf("--lot does not apply to a %s transaction", typeName)
		}
	}
	return nil
}

// mergeEditValues applies delta semantics: each field takes the flag
// value when supplied, else the stored value. For the share-bearing
// types, amount/price interplay follows SmartCompute: supplying only
// --amount (or neither) re-derives the price, supplying only
// --price-per-share re-derives the total.
func mergeEditValues(old *investmentdom.Transaction, opts *investmentEditOptions) (*editedValues, error) {
	vals := &editedValues{date: old.Date}

	var err error
	if opts.dateChanged {
		vals.date, err = types.ParseDate(opts.date)
		if err != nil {
			return nil, fmt.Errorf("invalid --date: %w", err)
		}
	}

	if old.Shares.Valid {
		vals.shares = old.Shares.Quantity
	}
	if opts.sharesChanged {
		vals.shares, err = types.NewQuantity(opts.shares)
		if err != nil {
			return nil, fmt.Errorf("invalid --shares: %w", err)
		}
	}

	switch {
	case opts.amountChanged:
		a, err := types.NewMoney(opts.amount)
		if err != nil {
			return nil, fmt.Errorf("invalid --amount: %w", err)
		}
		vals.totalAmount = &a
		vals.amount = a
		if opts.priceChanged {
			p, err := types.NewMoney(opts.pricePerShare)
			if err != nil {
				return nil, fmt.Errorf("invalid --price-per-share: %w", err)
			}
			vals.price = &p
		}
	case opts.priceChanged:
		p, err := types.NewMoney(opts.pricePerShare)
		if err != nil {
			return nil, fmt.Errorf("invalid --price-per-share: %w", err)
		}
		vals.price = &p
		vals.amount = old.TotalAmount.Abs()
	default:
		total := old.TotalAmount.Abs()
		vals.totalAmount = &total
		vals.amount = total
	}

	if old.Commission.Valid {
		vals.commission = old.Commission.Money
	} else {
		vals.commission = types.ZeroMoney
	}
	if opts.commissionChanged {
		vals.commission, err = types.NewMoney(opts.commission)
		if err != nil {
			return nil, fmt.Errorf("invalid --commission: %w", err)
		}
	}

	if old.Memo.Valid {
		vals.memo = old.Memo.String
	}
	if opts.memoChanged {
		vals.memo = opts.memo
	}

	return vals, nil
}

// dispatchUpdate routes the merged values to the Service.Update* method
// matching the stored transaction type — the same methods the TUI edit
// dialogs call.
func dispatchUpdate(svc *investmentdom.Service, old *investmentdom.Transaction, vals *editedValues) (*investmentdom.Transaction, error) {
	securityID := types.NilID
	if old.SecurityID.Valid {
		securityID = old.SecurityID.ID
	}

	switch old.Type {
	case investmentdom.TransactionTypeBuy:
		return svc.UpdateBuy(old.ID, old.AccountID, securityID, vals.date, vals.shares,
			vals.totalAmount, vals.price, vals.commission, vals.memo)
	case investmentdom.TransactionTypeSell:
		return svc.UpdateSell(old.ID, old.AccountID, securityID, vals.date, vals.shares,
			vals.totalAmount, vals.price, vals.commission, vals.memo, vals.lotAllocations)
	case investmentdom.TransactionTypeFeeLiquidation:
		return svc.UpdateFeeLiquidation(old.ID, old.AccountID, securityID, vals.date, vals.shares,
			vals.totalAmount, vals.price, vals.commission, vals.memo, vals.lotAllocations)
	case investmentdom.TransactionTypeReinvestDividend:
		return svc.UpdateReinvestDividend(old.ID, old.AccountID, securityID, vals.date, vals.shares,
			vals.totalAmount, vals.price, vals.memo)
	case investmentdom.TransactionTypeDividend:
		return svc.UpdateDividend(old.ID, old.AccountID, securityID, vals.date, vals.amount, vals.memo)
	case investmentdom.TransactionTypeDeposit:
		return svc.UpdateDeposit(old.ID, old.AccountID, vals.date, vals.amount, vals.memo)
	case investmentdom.TransactionTypeWithdrawal:
		return svc.UpdateWithdrawal(old.ID, old.AccountID, vals.date, vals.amount, vals.memo)
	case investmentdom.TransactionTypeFee:
		return svc.UpdateFee(old.ID, old.AccountID, vals.date, vals.amount, vals.memo)
	case investmentdom.TransactionTypeInterest:
		return svc.UpdateInterest(old.ID, old.AccountID, vals.date, vals.amount, vals.memo)
	default:
		return nil, fmt.Errorf("editing %s transactions is not supported", old.Type.DisplayName())
	}
}
