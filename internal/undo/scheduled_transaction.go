package undo

import (
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
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
//
// Redo determinism: the undo manager implements redo by calling Execute a
// second time. A loan-shaped multi-line schedule recomputes its
// interest/principal split from the loan's live balance, which may have
// changed between the original post and the redo — so re-running Post on redo
// would produce different rows. To keep redo deterministic, a multi-line post
// captures the created parent + splits on first Execute and replays them
// verbatim (via PostWithEdits) on redo. Single-line posts copy the template
// verbatim, so re-running Post already reproduces them and no capture is kept.
type PostScheduledTransactionCommand struct {
	svc           *scheduled.Service
	txnSvc        *transaction.Service
	id            types.ID
	amount        *types.Money             // optional override amount
	beforeST      *scheduled.Transaction   // schedule state before posting
	createdTxn    *transaction.Transaction // transaction created by Post
	createdSplits []*transaction.Split     // non-nil for multi-line: replayed verbatim on redo
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

	// Redo of a captured multi-line post: replay the exact rows created the
	// first time rather than recomputing (a loan-shaped schedule would
	// otherwise recompute against a since-changed balance).
	if c.createdTxn != nil && c.createdSplits != nil {
		_, err := c.svc.PostWithEdits(c.id, c.createdTxn, c.createdSplits)
		return err
	}

	// First post (or a single-line redo, which is deterministic): create the
	// transaction, advance the schedule, and capture any child splits.
	txn, splits, err := c.svc.PostReturningSplits(c.id, c.amount)
	if err != nil {
		return err
	}
	c.createdTxn = txn
	c.createdSplits = splits // nil for single-line; drives verbatim redo for multi-line

	return nil
}

func (c *PostScheduledTransactionCommand) Undo() error {
	// Delete the created transaction. It is nil when the occurrence produced no
	// regular-ledger row — an investment-to-investment transfer puts both legs in
	// investment_transactions — in which case there is nothing here to delete and
	// only the schedule needs restoring.
	if c.createdTxn != nil {
		if err := c.txnSvc.Delete(c.createdTxn.ID); err != nil {
			return err
		}
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
// PostScheduledTransferCommand
// =============================================================================

// PostScheduledTransferCommand posts one occurrence of a single-line transfer
// schedule from the post-time preview, using the edited date / amount and
// applying the edited memo + status to both legs. PostWithDate creates a clean
// linked transfer pair via the transaction service. Undo deletes the created
// pair (deleting either leg cascades to its counterpart) and restores the
// schedule to its previous state.
type PostScheduledTransferCommand struct {
	svc *scheduled.Service
	// transferSvc owns the posted transfer. Both the preview-override apply and
	// the undo delete address it by transfer_id, so this works whichever ledgers
	// the occurrence's legs landed in — including an inv↔inv occurrence, which
	// has no regular-table row to address at all.
	transferSvc *transfer.Service
	id          types.ID
	date        types.Date
	amount      types.Money
	memo        string
	cleared     bool
	category    types.NullableID
	beforeST    *scheduled.Transaction
	// postedTransferID is the transfer created by Execute, captured for Undo.
	// Undo addresses the transfer, not a leg, so it works for every shape.
	postedTransferID types.ID
	// createdTxn is the posted regular-ledger leg, exposed for the register's
	// cursor restore. It is nil for an inv↔inv occurrence, which has no
	// regular-table row.
	createdTxn *transaction.Transaction
}

// NewPostScheduledTransferCommand creates a command that posts a transfer
// schedule occurrence on the given date for the given (positive) amount.
// category is the one-off category applied to both posted legs (an invalid
// categoryID leaves the posted transfer uncategorized).
func NewPostScheduledTransferCommand(
	svc *scheduled.Service,
	transferSvc *transfer.Service,
	id types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	cleared bool,
	category types.NullableID,
) *PostScheduledTransferCommand {
	return &PostScheduledTransferCommand{
		svc:         svc,
		transferSvc: transferSvc,
		id:          id,
		date:        date,
		amount:      amount,
		memo:        memo,
		cleared:     cleared,
		category:    category,
	}
}

func (c *PostScheduledTransferCommand) Execute() error {
	before, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.beforeST = before

	amount := c.amount
	txn, transferID, err := c.svc.PostWithDateReturningTransfer(c.id, c.date, &amount)
	if err != nil {
		return err
	}
	// txn is the regular-ledger leg, and is nil for an inv↔inv occurrence whose
	// two legs both live in investment_transactions.
	c.createdTxn = txn

	// Take the transfer id from the POSTING PATH, never off a leg. Reading it
	// off txn.TransferID is what used to break Undo for an inv↔inv occurrence:
	// there is no regular-ledger leg to read, so postedTransferID stayed zero,
	// Undo's guard skipped the delete, and the schedule was rewound anyway —
	// leaving both posted legs in the ledger and re-posting them next time. The
	// undo reported success throughout.
	c.postedTransferID = transferID
	if c.postedTransferID.IsNil() {
		return nil // not a transfer occurrence; nothing to amend
	}

	// Apply the preview's edited memo + status to both legs. PostWithDate posts
	// using the template memo and uncleared status; this overwrites them with the
	// user's one-off edits.
	//
	// This is still a SECOND write after the posting transaction committed: if it
	// fails, the pair exists with template values and the schedule has already
	// advanced. Closing that needs the overrides threaded into PostWithDate so
	// one tx covers both — a pre-existing scheduled defect, tracked as design
	// section 13 open question 1, deliberately not folded in here.
	status := transaction.StatusUncleared
	if c.cleared {
		status = transaction.StatusCleared
	}
	edit := transfer.Edit{
		Date:   c.date,
		Amount: c.amount,
		Memo:   c.memo,
		Status: status,
	}
	// Only carry the one-off category when the pair can hold one.
	// transfer.Kind.StoresCategory() is false for exactly KindInvToInv, and an
	// inv↔inv pair is exactly the case with no regular-ledger leg — so a nil txn
	// is a precise proxy, and passing a category anyway would have the transfer
	// owner refuse the amendment of a post that already committed.
	if txn != nil {
		edit.CategoryID = c.category
	}
	_, err = c.transferSvc.Update(c.postedTransferID, edit)
	return err
}

func (c *PostScheduledTransferCommand) Undo() error {
	// Deleting by transfer_id removes BOTH legs wherever they live. Deleting the
	// regular leg through transaction.Service (what this did before) is now
	// refused outright, and would have left the counterpart behind even when it
	// worked.
	if !c.postedTransferID.IsNil() {
		if _, err := c.transferSvc.Delete(c.postedTransferID); err != nil {
			return err
		}
	}
	if c.beforeST != nil {
		return c.svc.Update(c.beforeST)
	}
	return nil
}

func (c *PostScheduledTransferCommand) Description() string {
	return "Post scheduled transfer"
}

// CreatedTransaction returns the posted regular-ledger leg, or nil — either
// before Execute, or for an inv↔inv occurrence that has no regular-table row.
func (c *PostScheduledTransferCommand) CreatedTransaction() *transaction.Transaction {
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
