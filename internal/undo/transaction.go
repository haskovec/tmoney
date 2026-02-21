package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// =============================================================================
// CreateTransactionCommand
// =============================================================================

// CreateTransactionCommand creates a transaction and can undo it by deleting.
type CreateTransactionCommand struct {
	svc         *service.TransactionService
	transaction *models.Transaction
	splits      []*models.Split // optional splits for split transactions
}

// NewCreateTransactionCommand creates a command that will create a transaction.
// The transaction is created on Execute and deleted on Undo.
func NewCreateTransactionCommand(svc *service.TransactionService, txn *models.Transaction) *CreateTransactionCommand {
	return &CreateTransactionCommand{
		svc:         svc,
		transaction: txn,
	}
}

// NewCreateTransactionWithSplitsCommand creates a command that will create a
// transaction with splits. Both are created on Execute and deleted on Undo.
func NewCreateTransactionWithSplitsCommand(svc *service.TransactionService, txn *models.Transaction, splits []*models.Split) *CreateTransactionCommand {
	return &CreateTransactionCommand{
		svc:         svc,
		transaction: txn,
		splits:      splits,
	}
}

func (c *CreateTransactionCommand) Execute() error {
	if len(c.splits) > 0 {
		return c.svc.CreateWithSplits(c.transaction, c.splits)
	}
	return c.svc.Create(c.transaction)
}

func (c *CreateTransactionCommand) Undo() error {
	return c.svc.Delete(c.transaction.ID)
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
	svc    *service.TransactionService
	before *models.Transaction // state before editing (captured on Execute)
	after  *models.Transaction // desired new state
}

// NewEditTransactionCommand creates a command that will update a transaction.
// The before state is captured at execute time by reading from the database.
func NewEditTransactionCommand(svc *service.TransactionService, after *models.Transaction) *EditTransactionCommand {
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
	svc    *service.TransactionService
	id     models.ID
	before *models.Transaction // full entity captured on Execute for undo
	splits []*models.Split     // splits captured on Execute for undo
}

// NewDeleteTransactionCommand creates a command that will delete a transaction.
// The full entity is captured at execute time so it can be recreated on undo.
func NewDeleteTransactionCommand(svc *service.TransactionService, id models.ID) *DeleteTransactionCommand {
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
	svc          *service.TransactionService
	id           models.ID
	beforeAmount models.Money
	beforeMemo   models.NullableString
	beforeStatus models.TransactionStatus
	beforeSplits []*models.Split // splits removed during void
	captured     bool
}

// NewVoidTransactionCommand creates a command that will void a transaction.
// The original amount, memo, and status are captured at execute time.
func NewVoidTransactionCommand(svc *service.TransactionService, id models.ID) *VoidTransactionCommand {
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
	svc           *service.TransactionService
	fromAccountID models.ID
	toAccountID   models.ID
	date          models.Date
	amount        models.Money
	pair          *models.TransferPair // populated after Execute
}

// NewCreateTransferCommand creates a command that will create a transfer.
func NewCreateTransferCommand(svc *service.TransactionService, fromAccountID, toAccountID models.ID, date models.Date, amount models.Money) *CreateTransferCommand {
	return &CreateTransferCommand{
		svc:           svc,
		fromAccountID: fromAccountID,
		toAccountID:   toAccountID,
		date:          date,
		amount:        amount,
	}
}

func (c *CreateTransferCommand) Execute() error {
	pair, err := c.svc.CreateTransfer(c.fromAccountID, c.toAccountID, c.date, c.amount)
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
func (c *CreateTransferCommand) Pair() *models.TransferPair {
	return c.pair
}

// =============================================================================
// DeleteTransferCommand
// =============================================================================

// DeleteTransferCommand deletes a transfer and can undo it by recreating both sides.
type DeleteTransferCommand struct {
	svc        *service.TransactionService
	transferID models.ID
	before     *models.TransferPair // captured on Execute for undo
}

// NewDeleteTransferCommand creates a command that will delete a transfer.
func NewDeleteTransferCommand(svc *service.TransactionService, transferID models.ID) *DeleteTransferCommand {
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
	svc           *service.TransactionService
	transactionID models.ID // any transaction ID in the transfer pair
	transferID    models.ID // populated during Execute
	beforeFrom    transactionSnapshot
	beforeTo      transactionSnapshot
	captured      bool
}

// transactionSnapshot stores the fields needed to restore a voided transaction.
type transactionSnapshot struct {
	amount models.Money
	memo   models.NullableString
	status models.TransactionStatus
}

// NewVoidTransferCommand creates a command that will void a transfer.
// Pass any transaction ID from the transfer pair.
func NewVoidTransferCommand(svc *service.TransactionService, transactionID models.ID) *VoidTransferCommand {
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
	c.beforeFrom = transactionSnapshot{
		amount: pair.FromTransaction.Amount,
		memo:   pair.FromTransaction.Memo,
		status: pair.FromTransaction.Status,
	}
	c.beforeTo = transactionSnapshot{
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
