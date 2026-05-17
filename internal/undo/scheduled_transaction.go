package undo

import (
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// CreateScheduledTransactionCommand
// =============================================================================

// CreateScheduledTransactionCommand creates a scheduled transaction and can
// undo it by deleting.
type CreateScheduledTransactionCommand struct {
	svc *scheduled.Service
	st  *scheduled.Transaction
}

// NewCreateScheduledTransactionCommand creates a command that will create a
// scheduled transaction. The entity is created on Execute and deleted on Undo.
func NewCreateScheduledTransactionCommand(svc *scheduled.Service, st *scheduled.Transaction) *CreateScheduledTransactionCommand {
	return &CreateScheduledTransactionCommand{
		svc: svc,
		st:  st,
	}
}

func (c *CreateScheduledTransactionCommand) Execute() error {
	return c.svc.Create(c.st)
}

func (c *CreateScheduledTransactionCommand) Undo() error {
	return c.svc.Delete(c.st.ID)
}

func (c *CreateScheduledTransactionCommand) Description() string {
	return "Create scheduled transaction"
}

// =============================================================================
// EditScheduledTransactionCommand
// =============================================================================

// EditScheduledTransactionCommand edits a scheduled transaction and can undo
// it by restoring the previous state.
type EditScheduledTransactionCommand struct {
	svc    *scheduled.Service
	before *scheduled.Transaction // state before editing (captured on Execute)
	after  *scheduled.Transaction // desired new state
}

// NewEditScheduledTransactionCommand creates a command that will update a
// scheduled transaction. The before state is captured at execute time by
// reading from the database.
func NewEditScheduledTransactionCommand(svc *scheduled.Service, after *scheduled.Transaction) *EditScheduledTransactionCommand {
	return &EditScheduledTransactionCommand{
		svc:   svc,
		after: after,
	}
}

func (c *EditScheduledTransactionCommand) Execute() error {
	// Capture before state from the database
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	return c.svc.Update(c.after)
}

func (c *EditScheduledTransactionCommand) Undo() error {
	return c.svc.Update(c.before)
}

func (c *EditScheduledTransactionCommand) Description() string {
	return "Edit scheduled transaction"
}

// =============================================================================
// DeleteScheduledTransactionCommand
// =============================================================================

// DeleteScheduledTransactionCommand deletes a scheduled transaction and can
// undo it by recreating.
type DeleteScheduledTransactionCommand struct {
	svc    *scheduled.Service
	id     types.ID
	before *scheduled.Transaction // full entity captured on Execute for undo
}

// NewDeleteScheduledTransactionCommand creates a command that will delete a
// scheduled transaction. The full entity is captured at execute time so it
// can be recreated on undo.
func NewDeleteScheduledTransactionCommand(svc *scheduled.Service, id types.ID) *DeleteScheduledTransactionCommand {
	return &DeleteScheduledTransactionCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeleteScheduledTransactionCommand) Execute() error {
	// Capture full entity before deleting
	st, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = st

	return c.svc.Delete(c.id)
}

func (c *DeleteScheduledTransactionCommand) Undo() error {
	return c.svc.Create(c.before)
}

func (c *DeleteScheduledTransactionCommand) Description() string {
	return "Delete scheduled transaction"
}

// =============================================================================
// PostScheduledTransactionCommand
// =============================================================================

// PostScheduledTransactionCommand posts a scheduled transaction (creates a
// real transaction and advances the schedule). Undo deletes the created
// transaction and restores the schedule to its previous state.
type PostScheduledTransactionCommand struct {
	svc        *scheduled.Service
	txnSvc     *transaction.Service
	id         types.ID
	amount     *types.Money             // optional override amount
	beforeST   *scheduled.Transaction   // schedule state before posting
	createdTxn *transaction.Transaction // transaction created by Post
}

// NewPostScheduledTransactionCommand creates a command that will post a
// scheduled transaction. Pass nil for amount to use the scheduled amount.
func NewPostScheduledTransactionCommand(
	svc *scheduled.Service,
	txnSvc *transaction.Service,
	id types.ID,
	amount *types.Money,
) *PostScheduledTransactionCommand {
	return &PostScheduledTransactionCommand{
		svc:    svc,
		txnSvc: txnSvc,
		id:     id,
		amount: amount,
	}
}

func (c *PostScheduledTransactionCommand) Execute() error {
	// Capture schedule state before posting
	before, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.beforeST = before

	// Post creates the transaction and advances the schedule
	txn, err := c.svc.Post(c.id, c.amount)
	if err != nil {
		return err
	}
	c.createdTxn = txn

	return nil
}

func (c *PostScheduledTransactionCommand) Undo() error {
	// Delete the created transaction
	if err := c.txnSvc.Delete(c.createdTxn.ID); err != nil {
		return err
	}

	// Restore the scheduled transaction to its previous state
	return c.svc.Update(c.beforeST)
}

func (c *PostScheduledTransactionCommand) Description() string {
	return "Post scheduled transaction"
}

// CreatedTransaction returns the transaction created by Execute. Returns nil
// if Execute has not been called or failed.
func (c *PostScheduledTransactionCommand) CreatedTransaction() *transaction.Transaction {
	return c.createdTxn
}

// =============================================================================
// PostScheduledTransactionWithEditsCommand
// =============================================================================

// PostScheduledTransactionWithEditsCommand posts a scheduled transaction
// with per-instance overrides from the post-time preview dialog (creates a
// real transaction carrying the user's edits and advances the schedule).
// Undo deletes the created transaction (plus any paired counterparts for
// multi-line transfer-line splits) and restores the schedule to its
// previous state.
//
// The caller is responsible for assembling txn (and splits, for multi-
// line) with the desired edits already applied — see
// scheduled.Service.PostWithEdits.
type PostScheduledTransactionWithEditsCommand struct {
	svc        *scheduled.Service
	txnSvc     *transaction.Service
	id         types.ID
	txn        *transaction.Transaction // parent to create (with user edits)
	splits     []*transaction.Split     // nil for single-line; non-nil for multi-line
	beforeST   *scheduled.Transaction   // schedule state before posting
	createdTxn *transaction.Transaction // transaction created on Execute
}

// NewPostScheduledTransactionWithEditsCommand creates a command that posts
// a scheduled transaction with per-instance overrides. txn and splits must
// already carry any edits the user made in the preview dialog.
func NewPostScheduledTransactionWithEditsCommand(
	svc *scheduled.Service,
	txnSvc *transaction.Service,
	id types.ID,
	txn *transaction.Transaction,
	splits []*transaction.Split,
) *PostScheduledTransactionWithEditsCommand {
	return &PostScheduledTransactionWithEditsCommand{
		svc:    svc,
		txnSvc: txnSvc,
		id:     id,
		txn:    txn,
		splits: splits,
	}
}

func (c *PostScheduledTransactionWithEditsCommand) Execute() error {
	before, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.beforeST = before

	txn, err := c.svc.PostWithEdits(c.id, c.txn, c.splits)
	if err != nil {
		return err
	}
	c.createdTxn = txn
	return nil
}

func (c *PostScheduledTransactionWithEditsCommand) Undo() error {
	// Delete the parent transaction. For multi-line transactions the
	// transaction service's Delete cascades to paired counter-transactions
	// (split-item transfer-lines).
	if c.createdTxn != nil {
		if err := c.txnSvc.Delete(c.createdTxn.ID); err != nil {
			return err
		}
	}
	return c.svc.Update(c.beforeST)
}

func (c *PostScheduledTransactionWithEditsCommand) Description() string {
	return "Post scheduled transaction"
}

// CreatedTransaction returns the transaction created by Execute. Returns
// nil if Execute has not been called or failed.
func (c *PostScheduledTransactionWithEditsCommand) CreatedTransaction() *transaction.Transaction {
	return c.createdTxn
}

// =============================================================================
// SkipScheduledTransactionCommand
// =============================================================================

// SkipScheduledTransactionCommand skips a scheduled transaction occurrence
// (advances the schedule without creating a transaction). Undo restores the
// schedule to its previous state.
type SkipScheduledTransactionCommand struct {
	svc      *scheduled.Service
	id       types.ID
	beforeST *scheduled.Transaction // schedule state before skipping
}

// NewSkipScheduledTransactionCommand creates a command that will skip a
// scheduled transaction occurrence.
func NewSkipScheduledTransactionCommand(svc *scheduled.Service, id types.ID) *SkipScheduledTransactionCommand {
	return &SkipScheduledTransactionCommand{
		svc: svc,
		id:  id,
	}
}

func (c *SkipScheduledTransactionCommand) Execute() error {
	// Capture schedule state before skipping
	before, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.beforeST = before

	return c.svc.Skip(c.id)
}

func (c *SkipScheduledTransactionCommand) Undo() error {
	// Restore the scheduled transaction to its previous state
	return c.svc.Update(c.beforeST)
}

func (c *SkipScheduledTransactionCommand) Description() string {
	return "Skip scheduled transaction"
}
