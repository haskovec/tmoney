package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// =============================================================================
// CreatePayeeCommand
// =============================================================================

// CreatePayeeCommand creates a payee and can undo it by deleting.
type CreatePayeeCommand struct {
	svc   *service.PayeeService
	payee *models.Payee
}

// NewCreatePayeeCommand creates a command that will create a payee.
// The payee is created on Execute and deleted on Undo.
func NewCreatePayeeCommand(svc *service.PayeeService, payee *models.Payee) *CreatePayeeCommand {
	return &CreatePayeeCommand{
		svc:   svc,
		payee: payee,
	}
}

func (c *CreatePayeeCommand) Execute() error {
	return c.svc.Create(c.payee)
}

func (c *CreatePayeeCommand) Undo() error {
	return c.svc.Delete(c.payee.ID)
}

func (c *CreatePayeeCommand) Description() string {
	return "Create payee"
}

// =============================================================================
// EditPayeeCommand
// =============================================================================

// EditPayeeCommand edits a payee and can undo it by restoring
// the previous state.
type EditPayeeCommand struct {
	svc    *service.PayeeService
	before *models.Payee // state before editing (captured on Execute)
	after  *models.Payee // desired new state
}

// NewEditPayeeCommand creates a command that will update a payee.
// The before state is captured at execute time by reading from the database.
func NewEditPayeeCommand(svc *service.PayeeService, after *models.Payee) *EditPayeeCommand {
	return &EditPayeeCommand{
		svc:   svc,
		after: after,
	}
}

func (c *EditPayeeCommand) Execute() error {
	// Capture before state from the database
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	return c.svc.Update(c.after)
}

func (c *EditPayeeCommand) Undo() error {
	return c.svc.Update(c.before)
}

func (c *EditPayeeCommand) Description() string {
	return "Edit payee"
}

// =============================================================================
// DeletePayeeCommand
// =============================================================================

// DeletePayeeCommand deletes a payee and can undo it by recreating.
type DeletePayeeCommand struct {
	svc     *service.PayeeService
	id      models.ID
	before  *models.Payee    // full entity captured on Execute for undo
	aliases []*models.Alias  // aliases captured on Execute for undo
}

// NewDeletePayeeCommand creates a command that will delete a payee.
// The full entity and its aliases are captured at execute time so they
// can be recreated on undo.
func NewDeletePayeeCommand(svc *service.PayeeService, id models.ID) *DeletePayeeCommand {
	return &DeletePayeeCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeletePayeeCommand) Execute() error {
	// Capture full entity before deleting
	payee, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = payee

	// Capture aliases before deleting (they are cascaded on payee delete)
	aliases, err := c.svc.GetAliasesByPayee(c.id)
	if err != nil {
		return err
	}
	c.aliases = aliases

	return c.svc.Delete(c.id)
}

func (c *DeletePayeeCommand) Undo() error {
	// Recreate the payee
	if err := c.svc.Create(c.before); err != nil {
		return err
	}

	// Recreate aliases
	for _, alias := range c.aliases {
		if err := c.svc.CreateAlias(alias); err != nil {
			return err
		}
	}

	return nil
}

func (c *DeletePayeeCommand) Description() string {
	return "Delete payee"
}

// =============================================================================
// MergePayeesCommand
// =============================================================================

// MergePayeesCommand merges a source payee into a target payee.
// This is a compound operation: it updates all references, reassigns aliases,
// and deletes the source. Undo is not supported for merge operations due to
// their complexity (they modify transactions, scheduled transactions, and aliases
// across multiple tables using temp table workarounds).
type MergePayeesCommand struct {
	svc      *service.PayeeService
	sourceID models.ID
	targetID models.ID
}

// NewMergePayeesCommand creates a command that will merge sourceID into targetID.
func NewMergePayeesCommand(svc *service.PayeeService, sourceID, targetID models.ID) *MergePayeesCommand {
	return &MergePayeesCommand{
		svc:      svc,
		sourceID: sourceID,
		targetID: targetID,
	}
}

func (c *MergePayeesCommand) Execute() error {
	return c.svc.MergePayees(c.sourceID, c.targetID)
}

func (c *MergePayeesCommand) Undo() error {
	// Merge is not reversible - the source payee and all original references
	// are lost. A backup should be created before merge operations.
	return fmt.Errorf("merge payees cannot be undone")
}

func (c *MergePayeesCommand) Description() string {
	return "Merge payees"
}
