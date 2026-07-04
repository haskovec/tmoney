package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// CreateTransactionCommand
// =============================================================================

// CreateTransactionCommand creates a transaction and can undo it by deleting.
type CreateTransactionCommand struct {
	svc    *transaction.Service
	txn    *transaction.Transaction
	splits []*transaction.Split // optional splits for split transactions
}

// NewCreateTransactionCommand creates a command that will create a transaction.
// The transaction is created on Execute and deleted on Undo.
func NewCreateTransactionCommand(svc *transaction.Service, txn *transaction.Transaction) *CreateTransactionCommand {
	return &CreateTransactionCommand{
		svc: svc,
		txn: txn,
	}
}

// NewCreateTransactionWithSplitsCommand creates a command that will create a
// transaction with splits. Both are created on Execute and deleted on Undo.
func NewCreateTransactionWithSplitsCommand(svc *transaction.Service, txn *transaction.Transaction, splits []*transaction.Split) *CreateTransactionCommand {
	return &CreateTransactionCommand{
		svc:    svc,
		txn:    txn,
		splits: splits,
	}
}

func (c *CreateTransactionCommand) Execute() error {
	if len(c.splits) > 0 {
		return c.svc.CreateWithSplits(c.txn, c.splits)
	}
	return c.svc.Create(c.txn)
}

func (c *CreateTransactionCommand) Undo() error {
	return c.svc.Delete(c.txn.ID)
}

func (c *CreateTransactionCommand) Description() string {
	return "Create transaction"
}

// =============================================================================
// EditTransactionCommand
// =============================================================================

// EditTransactionCommand edits a transaction and can undo it by restoring
// the previous state.
type EditTransactionCommand struct {
	svc    *transaction.Service
	before *transaction.Transaction // state before editing (captured on Execute)
	after  *transaction.Transaction // desired new state
}

// NewEditTransactionCommand creates a command that will update a transaction.
// The before state is captured at execute time by reading from the database.
func NewEditTransactionCommand(svc *transaction.Service, after *transaction.Transaction) *EditTransactionCommand {
	return &EditTransactionCommand{
		svc:   svc,
		after: after,
	}
}

func (c *EditTransactionCommand) Execute() error {
	// Capture before state from the database
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	return c.svc.Update(c.after)
}

func (c *EditTransactionCommand) Undo() error {
	return c.svc.Update(c.before)
}

func (c *EditTransactionCommand) Description() string {
	return "Edit transaction"
}

// =============================================================================
// DeleteTransactionCommand
// =============================================================================

// DeleteTransactionCommand deletes a transaction and can undo it by recreating.
type DeleteTransactionCommand struct {
	svc    *transaction.Service
	id     types.ID
	before *transaction.Transaction // full entity captured on Execute for undo
	splits []*transaction.Split     // splits captured on Execute for undo
}

// NewDeleteTransactionCommand creates a command that will delete a transaction.
// The full entity is captured at execute time so it can be recreated on undo.
func NewDeleteTransactionCommand(svc *transaction.Service, id types.ID) *DeleteTransactionCommand {
	return &DeleteTransactionCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeleteTransactionCommand) Execute() error {
	// Capture full entity before deleting
	txn, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = txn

	// Capture splits before deleting
	splits, err := c.svc.GetSplits(c.id)
	if err != nil {
		return err
	}
	c.splits = splits

	return c.svc.Delete(c.id)
}

func (c *DeleteTransactionCommand) Undo() error {
	if len(c.splits) > 0 {
		return c.svc.CreateWithSplits(c.before, c.splits)
	}
	return c.svc.Create(c.before)
}

func (c *DeleteTransactionCommand) Description() string {
	return "Delete transaction"
}

// =============================================================================
// VoidTransactionCommand
// =============================================================================

// VoidTransactionCommand voids a transaction and can undo it by restoring
// the original amount, memo, and status.
type VoidTransactionCommand struct {
	svc          *transaction.Service
	id           types.ID
	beforeAmount types.Money
	beforeMemo   types.NullableString
	beforeStatus transaction.Status
	beforeSplits []*transaction.Split // splits removed during void
	captured     bool
}

// NewVoidTransactionCommand creates a command that will void a transaction.
// The original amount, memo, and status are captured at execute time.
func NewVoidTransactionCommand(svc *transaction.Service, id types.ID) *VoidTransactionCommand {
	return &VoidTransactionCommand{
		svc: svc,
		id:  id,
	}
}

func (c *VoidTransactionCommand) Execute() error {
	// Capture before state
	txn, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.beforeAmount = txn.Amount
	c.beforeMemo = txn.Memo
	c.beforeStatus = txn.Status

	// Capture splits (they are removed during void)
	splits, err := c.svc.GetSplits(c.id)
	if err != nil {
		return err
	}
	c.beforeSplits = splits
	c.captured = true

	return c.svc.VoidTransaction(c.id)
}

func (c *VoidTransactionCommand) Undo() error {
	// Use RestoreVoidedTransaction which bypasses the void check in Update
	if err := c.svc.RestoreVoidedTransaction(c.id, c.beforeAmount, c.beforeMemo, c.beforeStatus); err != nil {
		return err
	}

	// Restore splits if any were removed
	if len(c.beforeSplits) > 0 {
		return c.svc.ReplaceSplits(c.id, c.beforeSplits)
	}

	return nil
}

func (c *VoidTransactionCommand) Description() string {
	return "Void transaction"
}

// =============================================================================
// CreateTransferCommand
// =============================================================================

// CreateTransferCommand creates a transfer between accounts and can undo it
// by deleting both sides.
type CreateTransferCommand struct {
	svc           *transaction.Service
	fromAccountID types.ID
	toAccountID   types.ID
	date          types.Date
	amount        types.Money
	memo          string
	categoryID    types.NullableID
	pair          *transaction.TransferPair // populated after Execute
}

// NewCreateTransferCommand creates a command that will create a transfer.
// memo and categoryID are optional labels stamped on both legs; an invalid
// categoryID means no category.
func NewCreateTransferCommand(svc *transaction.Service, fromAccountID, toAccountID types.ID, date types.Date, amount types.Money, memo string, categoryID types.NullableID) *CreateTransferCommand {
	return &CreateTransferCommand{
		svc:           svc,
		fromAccountID: fromAccountID,
		toAccountID:   toAccountID,
		date:          date,
		amount:        amount,
		memo:          memo,
		categoryID:    categoryID,
	}
}

func (c *CreateTransferCommand) Execute() error {
	pair, err := c.svc.CreateTransfer(c.fromAccountID, c.toAccountID, c.date, c.amount, c.memo, c.categoryID)
	if err != nil {
		return err
	}
	c.pair = pair
	return nil
}

func (c *CreateTransferCommand) Undo() error {
	return c.svc.DeleteTransfer(c.pair.FromTransaction.TransferID.ID)
}

func (c *CreateTransferCommand) Description() string {
	return "Create transfer"
}

// Pair returns the transfer pair created by Execute. Returns nil if Execute
// has not been called or failed.
func (c *CreateTransferCommand) Pair() *transaction.TransferPair {
	return c.pair
}

// =============================================================================
// DeleteTransferCommand
// =============================================================================

// DeleteTransferCommand deletes a transfer and can undo it by recreating both sides.
type DeleteTransferCommand struct {
	svc        *transaction.Service
	transferID types.ID
	before     *transaction.TransferPair // captured on Execute for undo
}

// NewDeleteTransferCommand creates a command that will delete a transfer.
func NewDeleteTransferCommand(svc *transaction.Service, transferID types.ID) *DeleteTransferCommand {
	return &DeleteTransferCommand{
		svc:        svc,
		transferID: transferID,
	}
}

func (c *DeleteTransferCommand) Execute() error {
	// Capture both sides before deleting
	pair, err := c.svc.GetTransferPair(c.transferID)
	if err != nil {
		return err
	}
	c.before = pair

	return c.svc.DeleteTransfer(c.transferID)
}

func (c *DeleteTransferCommand) Undo() error {
	// Recreate both sides
	if err := c.svc.Create(c.before.FromTransaction); err != nil {
		return err
	}
	if err := c.svc.Create(c.before.ToTransaction); err != nil {
		// Best effort rollback
		_ = c.svc.Delete(c.before.FromTransaction.ID)
		return err
	}
	return nil
}

func (c *DeleteTransferCommand) Description() string {
	return "Delete transfer"
}

// =============================================================================
// VoidTransferCommand
// =============================================================================

// VoidTransferCommand voids both sides of a transfer and can undo it by
// restoring the original amounts, memos, and statuses.
type VoidTransferCommand struct {
	svc           *transaction.Service
	transactionID types.ID // any transaction ID in the transfer pair
	transferID    types.ID // populated during Execute
	beforeFrom    txnSnapshot
	beforeTo      txnSnapshot
	captured      bool
}

// txnSnapshot stores the fields needed to restore a voided transaction.
type txnSnapshot struct {
	amount types.Money
	memo   types.NullableString
	status transaction.Status
}

// NewVoidTransferCommand creates a command that will void a transfer.
// Pass any transaction ID from the transfer pair.
func NewVoidTransferCommand(svc *transaction.Service, transactionID types.ID) *VoidTransferCommand {
	return &VoidTransferCommand{
		svc:           svc,
		transactionID: transactionID,
	}
}

func (c *VoidTransferCommand) Execute() error {
	// Get the transaction to find the transfer ID
	txn, err := c.svc.GetByID(c.transactionID)
	if err != nil {
		return err
	}

	if !txn.IsTransfer() {
		return fmt.Errorf("transaction %s is not a transfer", c.transactionID.String())
	}

	c.transferID = txn.TransferID.ID

	// Capture both sides before voiding
	pair, err := c.svc.GetTransferPair(c.transferID)
	if err != nil {
		return err
	}
	c.beforeFrom = txnSnapshot{
		amount: pair.FromTransaction.Amount,
		memo:   pair.FromTransaction.Memo,
		status: pair.FromTransaction.Status,
	}
	c.beforeTo = txnSnapshot{
		amount: pair.ToTransaction.Amount,
		memo:   pair.ToTransaction.Memo,
		status: pair.ToTransaction.Status,
	}
	c.captured = true

	return c.svc.VoidTransaction(c.transactionID)
}

func (c *VoidTransferCommand) Undo() error {
	// Use RestoreVoidedTransfer which bypasses the void check in UpdateTransfer
	return c.svc.RestoreVoidedTransfer(
		c.transferID,
		c.beforeFrom.amount, c.beforeFrom.memo, c.beforeFrom.status,
		c.beforeTo.amount, c.beforeTo.memo, c.beforeTo.status,
	)
}

func (c *VoidTransferCommand) Description() string {
	return "Void transfer"
}

// =============================================================================
// EditTransferCommand
// =============================================================================

// EditTransferCommand edits both sides of a transfer (amount, date, memo,
// status) and can undo by restoring the prior pair state captured at
// Execute time. Account changes are not supported — the transfer pair's
// from/to are immutable; only the editable common fields move.
type EditTransferCommand struct {
	svc        *transaction.Service
	transferID types.ID
	date       types.Date
	amount     types.Money
	memo       string
	status     transaction.Status
	categoryID types.NullableID

	beforeDate     types.Date
	beforeAmount   types.Money
	beforeMemo     string
	beforeStatus   transaction.Status
	beforeCategory types.NullableID
	captured       bool
}

// NewEditTransferCommand creates a command that updates the editable common
// fields of both sides of a transfer. amount must be positive; the from-side
// gets the negated value, the to-side gets the positive value. categoryID is
// mirrored onto both legs (an invalid categoryID clears it).
func NewEditTransferCommand(svc *transaction.Service, transferID types.ID, date types.Date, amount types.Money, memo string, status transaction.Status, categoryID types.NullableID) *EditTransferCommand {
	return &EditTransferCommand{
		svc:        svc,
		transferID: transferID,
		date:       date,
		amount:     amount,
		memo:       memo,
		status:     status,
		categoryID: categoryID,
	}
}

func (c *EditTransferCommand) Execute() error {
	pair, err := c.svc.GetTransferPair(c.transferID)
	if err != nil {
		return err
	}
	c.beforeDate = pair.FromTransaction.Date
	c.beforeAmount = pair.ToTransaction.Amount // positive side
	c.beforeMemo = ""
	if pair.FromTransaction.Memo.Valid {
		c.beforeMemo = pair.FromTransaction.Memo.String
	}
	c.beforeStatus = pair.FromTransaction.Status
	// The outflow (From) leg's category is canonical for display; capture it so
	// undo restores the prior category on both legs.
	c.beforeCategory = pair.FromTransaction.CategoryID
	c.captured = true

	return c.svc.UpdateTransfer(c.transferID, c.date, c.amount, c.memo, c.status, c.categoryID)
}

func (c *EditTransferCommand) Undo() error {
	if !c.captured {
		return fmt.Errorf("EditTransferCommand: cannot undo before Execute")
	}
	return c.svc.UpdateTransfer(c.transferID, c.beforeDate, c.beforeAmount, c.beforeMemo, c.beforeStatus, c.beforeCategory)
}

func (c *EditTransferCommand) Description() string {
	return "Edit transfer"
}

// =============================================================================
// EditTransactionWithSplitsCommand
// =============================================================================

// EditTransactionWithSplitsCommand edits a transaction's parent fields and
// replaces its splits in one undo unit. It handles all four cases:
// plain→plain (no splits change), plain→split, split→plain, split→split.
// On Undo it restores both the parent and the prior split set.
type EditTransactionWithSplitsCommand struct {
	svc          *transaction.Service
	after        *transaction.Transaction
	afterSplits  []*transaction.Split
	before       *transaction.Transaction
	beforeSplits []*transaction.Split
}

// NewEditTransactionWithSplitsCommand creates a command that updates the
// transaction and replaces its splits with afterSplits. afterSplits may be
// empty to convert a split transaction back to a plain one.
func NewEditTransactionWithSplitsCommand(svc *transaction.Service, after *transaction.Transaction, afterSplits []*transaction.Split) *EditTransactionWithSplitsCommand {
	return &EditTransactionWithSplitsCommand{
		svc:         svc,
		after:       after,
		afterSplits: afterSplits,
	}
}

func (c *EditTransactionWithSplitsCommand) Execute() error {
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	beforeSplits, err := c.svc.GetSplits(c.after.ID)
	if err != nil {
		return err
	}
	c.beforeSplits = beforeSplits

	// Update the parent first, then let ReplaceSplits reconcile the splits
	// against the current set. Migration 026 dropped transaction_splits'
	// inbound FK, so updating a parent that still has split children no longer
	// trips DuckDB's FK-on-rewrite error — and keeping the old rows in place
	// lets ReplaceSplits preserve each retained transfer line's counterpart
	// instead of churning it through a clear-then-recreate cycle.
	if err := c.svc.Update(c.after); err != nil {
		return err
	}
	return c.svc.ReplaceSplits(c.after.ID, c.afterSplits)
}

func (c *EditTransactionWithSplitsCommand) Undo() error {
	if c.before == nil {
		return fmt.Errorf("EditTransactionWithSplitsCommand: cannot undo before Execute")
	}
	// Mirror the Execute ordering: restore the parent, then let ReplaceSplits
	// reconcile the current (after) split set back to the original one,
	// re-linking each restored transfer line to its counterpart.
	if err := c.svc.Update(c.before); err != nil {
		return err
	}
	return c.svc.ReplaceSplits(c.before.ID, c.beforeSplits)
}

func (c *EditTransactionWithSplitsCommand) Description() string {
	return "Edit transaction"
}
