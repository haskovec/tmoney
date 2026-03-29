package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
)

// AutoPostCommand represents an auto-post session that created transactions
// on file open. Each auto-post session is a single undo step that groups
// all auto-posted transactions together.
//
// This command is designed to be pushed onto the undo stack after the
// auto-post has already been performed (via Manager.Push), since auto-posting
// runs asynchronously on startup.
type AutoPostCommand struct {
	txnSvc       *transaction.Service
	scheduledSvc *scheduled.Service
	summary      *scheduled.AutoPostSummary
	count        int
}

// NewAutoPostCommand creates a command representing an already-completed
// auto-post session. The summary contains the created transactions and
// the before-state of each scheduled transaction for undo.
func NewAutoPostCommand(
	txnSvc *transaction.Service,
	scheduledSvc *scheduled.Service,
	summary *scheduled.AutoPostSummary,
) *AutoPostCommand {
	return &AutoPostCommand{
		txnSvc:       txnSvc,
		scheduledSvc: scheduledSvc,
		summary:      summary,
		count:        summary.PostedCount,
	}
}

// Execute is a no-op because auto-posting has already been performed.
// This command is intended to be used with Manager.Push.
func (c *AutoPostCommand) Execute() error {
	return nil
}

// Undo reverses the auto-post session by deleting all created transactions
// and restoring each scheduled transaction to its pre-post state.
func (c *AutoPostCommand) Undo() error {
	// Delete all created transactions in reverse order
	for i := len(c.summary.Results) - 1; i >= 0; i-- {
		result := c.summary.Results[i]
		for j := len(result.Transactions) - 1; j >= 0; j-- {
			if err := c.txnSvc.Delete(result.Transactions[j].ID); err != nil {
				return fmt.Errorf("failed to delete auto-posted transaction: %w", err)
			}
		}
	}

	// Restore scheduled transactions to their pre-post state
	for _, result := range c.summary.Results {
		if result.BeforeSchedule != nil && len(result.Transactions) > 0 {
			if err := c.scheduledSvc.Update(result.BeforeSchedule); err != nil {
				return fmt.Errorf("failed to restore schedule: %w", err)
			}
		}
	}

	return nil
}

// Description returns a human-readable description of the auto-post session.
func (c *AutoPostCommand) Description() string {
	return fmt.Sprintf("Auto-post %d scheduled transaction(s)", c.count)
}
