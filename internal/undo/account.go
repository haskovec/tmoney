package undo

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// CreateAccountCommand
// =============================================================================

// CreateAccountCommand creates an account and can undo it by deleting.
type CreateAccountCommand struct {
	svc  *account.Service
	acct *account.Account
}

// NewCreateAccountCommand creates a command that will create an account.
// The account is created on Execute and deleted on Undo.
func NewCreateAccountCommand(svc *account.Service, acct *account.Account) *CreateAccountCommand {
	return &CreateAccountCommand{
		svc:  svc,
		acct: acct,
	}
}

func (c *CreateAccountCommand) Execute() error {
	return c.svc.Create(c.acct)
}

func (c *CreateAccountCommand) Undo() error {
	return c.svc.Delete(c.acct.ID)
}

func (c *CreateAccountCommand) Description() string {
	return "Create account"
}

// =============================================================================
// EditAccountCommand
// =============================================================================

// EditAccountCommand edits an account and can undo it by restoring
// the previous state.
type EditAccountCommand struct {
	svc    *account.Service
	before *account.Account // state before editing (captured on Execute)
	after  *account.Account // desired new state
}

// NewEditAccountCommand creates a command that will update an account.
// The before state is captured at execute time by reading from the database.
func NewEditAccountCommand(svc *account.Service, after *account.Account) *EditAccountCommand {
	return &EditAccountCommand{
		svc:   svc,
		after: after,
	}
}

func (c *EditAccountCommand) Execute() error {
	// Capture before state from the database
	before, err := c.svc.GetByID(c.after.ID)
	if err != nil {
		return err
	}
	c.before = before

	return c.svc.Update(c.after)
}

func (c *EditAccountCommand) Undo() error {
	return c.svc.Update(c.before)
}

func (c *EditAccountCommand) Description() string {
	return "Edit account"
}

// =============================================================================
// DeleteAccountCommand
// =============================================================================

// DeleteAccountCommand deletes an account and can undo it by recreating.
type DeleteAccountCommand struct {
	svc    *account.Service
	id     types.ID
	before *account.Account // full entity captured on Execute for undo
}

// NewDeleteAccountCommand creates a command that will delete an account.
// The full entity is captured at execute time so it can be recreated on undo.
func NewDeleteAccountCommand(svc *account.Service, id types.ID) *DeleteAccountCommand {
	return &DeleteAccountCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeleteAccountCommand) Execute() error {
	// Capture full entity before deleting
	acct, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = acct

	return c.svc.Delete(c.id)
}

func (c *DeleteAccountCommand) Undo() error {
	return c.svc.Create(c.before)
}

func (c *DeleteAccountCommand) Description() string {
	return "Delete account"
}

// =============================================================================
// CloseAccountCommand
// =============================================================================

// CloseAccountCommand closes an account on a given date and can undo it by
// reopening. Reopening restores the exact prior state (active, no close date),
// so no snapshot is needed for the close direction.
type CloseAccountCommand struct {
	svc  *account.Service
	id   types.ID
	date types.Date
}

// NewCloseAccountCommand creates a command that will close an account as of date.
func NewCloseAccountCommand(svc *account.Service, id types.ID, date types.Date) *CloseAccountCommand {
	return &CloseAccountCommand{
		svc:  svc,
		id:   id,
		date: date,
	}
}

func (c *CloseAccountCommand) Execute() error {
	return c.svc.Close(c.id, c.date)
}

func (c *CloseAccountCommand) Undo() error {
	return c.svc.Reopen(c.id)
}

func (c *CloseAccountCommand) Description() string {
	return "Close account"
}

// =============================================================================
// ReopenAccountCommand
// =============================================================================

// ReopenAccountCommand reopens a closed account and can undo it by re-closing
// to the EXACT prior close date (captured on Execute). The undo uses
// RestoreClosed, which bypasses close-date/zero-balance validation, so a
// back-dated or NULL (unknown) prior close date restores faithfully.
type ReopenAccountCommand struct {
	svc    *account.Service
	id     types.ID
	before types.NullableDate // prior close date, captured on Execute
}

// NewReopenAccountCommand creates a command that will reopen a closed account.
func NewReopenAccountCommand(svc *account.Service, id types.ID) *ReopenAccountCommand {
	return &ReopenAccountCommand{
		svc: svc,
		id:  id,
	}
}

func (c *ReopenAccountCommand) Execute() error {
	acct, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = acct.ClosedDate
	return c.svc.Reopen(c.id)
}

func (c *ReopenAccountCommand) Undo() error {
	return c.svc.RestoreClosed(c.id, c.before)
}

func (c *ReopenAccountCommand) Description() string {
	return "Reopen account"
}
