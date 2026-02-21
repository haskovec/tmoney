package undo

import (
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// =============================================================================
// CreateAccountCommand
// =============================================================================

// CreateAccountCommand creates an account and can undo it by deleting.
type CreateAccountCommand struct {
	svc     *service.AccountService
	account *models.Account
}

// NewCreateAccountCommand creates a command that will create an account.
// The account is created on Execute and deleted on Undo.
func NewCreateAccountCommand(svc *service.AccountService, account *models.Account) *CreateAccountCommand {
	return &CreateAccountCommand{
		svc:     svc,
		account: account,
	}
}

func (c *CreateAccountCommand) Execute() error {
	return c.svc.Create(c.account)
}

func (c *CreateAccountCommand) Undo() error {
	return c.svc.Delete(c.account.ID)
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
	svc    *service.AccountService
	before *models.Account // state before editing (captured on Execute)
	after  *models.Account // desired new state
}

// NewEditAccountCommand creates a command that will update an account.
// The before state is captured at execute time by reading from the database.
func NewEditAccountCommand(svc *service.AccountService, after *models.Account) *EditAccountCommand {
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
	svc    *service.AccountService
	id     models.ID
	before *models.Account // full entity captured on Execute for undo
}

// NewDeleteAccountCommand creates a command that will delete an account.
// The full entity is captured at execute time so it can be recreated on undo.
func NewDeleteAccountCommand(svc *service.AccountService, id models.ID) *DeleteAccountCommand {
	return &DeleteAccountCommand{
		svc: svc,
		id:  id,
	}
}

func (c *DeleteAccountCommand) Execute() error {
	// Capture full entity before deleting
	account, err := c.svc.GetByID(c.id)
	if err != nil {
		return err
	}
	c.before = account

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

// CloseAccountCommand closes an account and can undo it by reopening.
type CloseAccountCommand struct {
	svc *service.AccountService
	id  models.ID
}

// NewCloseAccountCommand creates a command that will close an account.
func NewCloseAccountCommand(svc *service.AccountService, id models.ID) *CloseAccountCommand {
	return &CloseAccountCommand{
		svc: svc,
		id:  id,
	}
}

func (c *CloseAccountCommand) Execute() error {
	return c.svc.Close(c.id)
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

// ReopenAccountCommand reopens a closed account and can undo it by closing.
type ReopenAccountCommand struct {
	svc *service.AccountService
	id  models.ID
}

// NewReopenAccountCommand creates a command that will reopen a closed account.
func NewReopenAccountCommand(svc *service.AccountService, id models.ID) *ReopenAccountCommand {
	return &ReopenAccountCommand{
		svc: svc,
		id:  id,
	}
}

func (c *ReopenAccountCommand) Execute() error {
	return c.svc.Reopen(c.id)
}

func (c *ReopenAccountCommand) Undo() error {
	return c.svc.Close(c.id)
}

func (c *ReopenAccountCommand) Description() string {
	return "Reopen account"
}
