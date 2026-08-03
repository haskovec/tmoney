package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// The four commands in this file replace seven: transaction.go's
// CreateTransferCommand, DeleteTransferCommand, VoidTransferCommand and
// EditTransferCommand, plus investment_transfer.go's three
// CreateInvestmentTransferCash / CreateInvestmentDeposit /
// CreateInvestmentToInvestmentTransfer commands.
//
// There were seven because there were four create paths and two edit paths, each
// with its own result shape, so each needed its own command to hold its own
// snapshot type. With one service and one Result, a transfer's undo does not
// depend on which pair of account types it connects.

// =============================================================================
// CreateTransferCommand
// =============================================================================

// CreateTransferCommand creates a transfer and undoes it by deleting both legs.
type CreateTransferCommand struct {
	svc  *transfer.Service
	spec transfer.Spec

	result *transfer.Result // populated after Execute
}

// NewCreateTransferCommand creates a command that will create a transfer of any
// shape — bank↔bank, bank↔investment or investment↔investment alike.
func NewCreateTransferCommand(svc *transfer.Service, spec transfer.Spec) *CreateTransferCommand {
	return &CreateTransferCommand{svc: svc, spec: spec}
}

func (c *CreateTransferCommand) Execute() error {
	res, err := c.svc.Create(c.spec)
	if err != nil {
		return err
	}
	c.result = res
	return nil
}

func (c *CreateTransferCommand) Undo() error {
	if c.result == nil {
		return fmt.Errorf("cannot undo a transfer create that did not run")
	}
	_, err := c.svc.Delete(c.result.TransferID)
	return err
}

func (c *CreateTransferCommand) Description() string { return "Create transfer" }

// Result returns the created transfer, or nil if Execute has not run or failed.
// Presentation uses it to move the cursor onto the just-saved leg.
func (c *CreateTransferCommand) Result() *transfer.Result { return c.result }

// =============================================================================
// EditTransferCommand
// =============================================================================

// EditTransferCommand edits a transfer in place and undoes it by writing the
// captured pre-edit values back.
//
// The "before" state is captured by the service inside the same transaction as
// the edit (Result.Before), not read separately by the command beforehand. That
// closes the window where a concurrent write between the command's read and the
// service's write would make the undo restore stale values.
type EditTransferCommand struct {
	svc        *transfer.Service
	transferID types.ID
	edit       transfer.Edit

	before *transfer.Transfer
	result *transfer.Result
}

// NewEditTransferCommand creates a command that will edit a transfer.
func NewEditTransferCommand(svc *transfer.Service, transferID types.ID, edit transfer.Edit) *EditTransferCommand {
	return &EditTransferCommand{svc: svc, transferID: transferID, edit: edit}
}

func (c *EditTransferCommand) Execute() error {
	res, err := c.svc.Update(c.transferID, c.edit)
	if err != nil {
		return err
	}
	c.result = res
	c.before = res.Before
	return nil
}

func (c *EditTransferCommand) Undo() error {
	if c.before == nil {
		return fmt.Errorf("cannot undo a transfer edit that did not run")
	}
	_, err := c.svc.Update(c.transferID, transfer.Edit{
		Date:       c.before.Date,
		Amount:     c.before.Amount,
		Memo:       c.before.Memo,
		CategoryID: c.before.CategoryID,
		Status:     c.before.Status,
	})
	return err
}

func (c *EditTransferCommand) Description() string { return "Edit transfer" }

// Result returns the edit's result, or nil if Execute has not run or failed.
func (c *EditTransferCommand) Result() *transfer.Result { return c.result }

// =============================================================================
// DeleteTransferCommand
// =============================================================================

// DeleteTransferCommand deletes a transfer and undoes it by recreating both legs
// under the ORIGINAL transfer_id, so a second undo step still addresses the same
// transfer.
type DeleteTransferCommand struct {
	svc        *transfer.Service
	transferID types.ID

	spec transfer.Spec // reconstructed from the pre-delete state
}

// NewDeleteTransferCommand creates a command that will delete a transfer.
func NewDeleteTransferCommand(svc *transfer.Service, transferID types.ID) *DeleteTransferCommand {
	return &DeleteTransferCommand{svc: svc, transferID: transferID}
}

func (c *DeleteTransferCommand) Execute() error {
	res, err := c.svc.Delete(c.transferID)
	if err != nil {
		return err
	}
	before := res.Before
	c.spec = transfer.Spec{
		FromAccountID: before.From.AccountID,
		ToAccountID:   before.To.AccountID,
		Date:          before.Date,
		Amount:        before.Amount,
		Memo:          before.Memo,
		CategoryID:    before.CategoryID,
		Status:        before.Status,
	}
	return nil
}

func (c *DeleteTransferCommand) Undo() error {
	if c.spec.Amount.IsZero() && c.spec.FromAccountID.IsNil() {
		return fmt.Errorf("cannot undo a transfer delete that did not run")
	}
	_, err := c.svc.Recreate(c.transferID, c.spec)
	return err
}

func (c *DeleteTransferCommand) Description() string { return "Delete transfer" }

// =============================================================================
// VoidTransferCommand
// =============================================================================

// VoidTransferCommand voids both legs of a transfer and undoes it by restoring
// the captured amounts, memos and statuses.
//
// Snapshots are ROW-ID addressed rather than From/To addressed. That is
// load-bearing: voiding zeroes both amounts, and orientation is carried by the
// sign, so a voided pair cannot be re-oriented afterwards. Cross-references
// cannot do it either — each leg's transfer_account_id points at the other, a
// symmetric relation that holds equally with the legs swapped.
type VoidTransferCommand struct {
	svc        *transfer.Service
	transferID types.ID

	snapshots []transfer.RestoreLeg
}

// NewVoidTransferCommand creates a command that will void a transfer.
func NewVoidTransferCommand(svc *transfer.Service, transferID types.ID) *VoidTransferCommand {
	return &VoidTransferCommand{svc: svc, transferID: transferID}
}

func (c *VoidTransferCommand) Execute() error {
	res, err := c.svc.Void(c.transferID)
	if err != nil {
		return err
	}
	for _, leg := range res.Before.Legs() {
		snap := transfer.RestoreLeg{
			RowID:  leg.RowID,
			Amount: leg.Amount,
			Status: leg.Status,
		}
		if leg.Memo != "" {
			snap.Memo = types.NullableString{String: leg.Memo, Valid: true}
		}
		c.snapshots = append(c.snapshots, snap)
	}
	return nil
}

func (c *VoidTransferCommand) Undo() error {
	if len(c.snapshots) == 0 {
		return fmt.Errorf("cannot undo a transfer void that did not run")
	}
	_, err := c.svc.Restore(c.transferID, c.snapshots)
	return err
}

func (c *VoidTransferCommand) Description() string { return "Void transfer" }

// =============================================================================
// SetTransferLegStatusCommand
// =============================================================================

// SetTransferLegStatusCommand toggles the status of ONE leg of a transfer.
//
// This is what the register's cleared toggle needs. Clearing your side of a
// transfer says your bank has posted it, which is independent of whether the
// other account's side has — so the two sides must move independently.
//
// The toggle used to route through EditTransactionCommand →
// transaction.Service.Update, which rewrites the entire row and has no
// transfer-aware path at all. Once Update refuses whole-transfer legs, that
// route stops working, so this command exists to keep Space-to-clear working on
// a transfer leg.
type SetTransferLegStatusCommand struct {
	svc    *transfer.Service
	legID  types.ID
	status transaction.Status

	before transaction.Status
	ran    bool
}

// NewSetTransferLegStatusCommand creates a command that sets one leg's status.
func NewSetTransferLegStatusCommand(svc *transfer.Service, legID types.ID, status transaction.Status) *SetTransferLegStatusCommand {
	return &SetTransferLegStatusCommand{svc: svc, legID: legID, status: status}
}

func (c *SetTransferLegStatusCommand) Execute() error {
	res, err := c.svc.SetLegStatus(c.legID, c.status)
	if err != nil {
		return err
	}
	// Recover this leg's prior status from the pre-write snapshot.
	for _, leg := range res.Before.Legs() {
		if leg.RowID == c.legID {
			c.before = leg.Status
			c.ran = true
			break
		}
	}
	if !c.ran {
		return fmt.Errorf("leg %s not found in the pre-write snapshot", c.legID.String())
	}
	return nil
}

func (c *SetTransferLegStatusCommand) Undo() error {
	if !c.ran {
		return fmt.Errorf("cannot undo a leg status change that did not run")
	}
	_, err := c.svc.SetLegStatus(c.legID, c.before)
	return err
}

func (c *SetTransferLegStatusCommand) Description() string { return "Change transfer status" }
