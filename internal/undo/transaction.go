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
	// Restore the row (bypassing the void check in Update) and its removed splits
	// in one transaction so the undo can't leave the row restored but split-less.
	return c.svc.RestoreVoidedTransactionWithSplits(c.id, c.beforeAmount, c.beforeMemo, c.beforeStatus, c.beforeSplits)
}

func (c *VoidTransactionCommand) Description() string {
	return "Void transaction"
}

// The four transfer commands that used to live here — CreateTransferCommand,
// DeleteTransferCommand, VoidTransferCommand and EditTransferCommand — now live
// in transfer.go, built on transfer.Service. Together with
// investment_transfer.go's three create commands they were seven commands for
// one concept, because there were four create paths and two edit paths with
// three different result shapes between them. See transfer.go.

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
	// against the current set — both in one transaction. Migration 026 dropped
	// transaction_splits' inbound FK, so updating a parent that still has split
	// children no longer trips DuckDB's FK-on-rewrite error — and keeping the old
	// rows in place lets ReplaceSplits preserve each retained transfer line's
	// counterpart instead of churning it through a clear-then-recreate cycle.
	return c.svc.UpdateWithSplits(c.after, c.afterSplits)
}

func (c *EditTransactionWithSplitsCommand) Undo() error {
	if c.before == nil {
		return fmt.Errorf("EditTransactionWithSplitsCommand: cannot undo before Execute")
	}
	// Mirror the Execute ordering: restore the parent, then let ReplaceSplits
	// reconcile the current (after) split set back to the original one,
	// re-linking each restored transfer line to its counterpart — atomically.
	return c.svc.UpdateWithSplits(c.before, c.beforeSplits)
}

func (c *EditTransactionWithSplitsCommand) Description() string {
	return "Edit transaction"
}
