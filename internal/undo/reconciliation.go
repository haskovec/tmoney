package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/service"
)

// FinishReconciliationCommand finishes a reconciliation session and can undo it
// by restoring all transaction statuses and reopening the session.
type FinishReconciliationCommand struct {
	reconSvc  *service.ReconciliationService
	txnSvc    *service.TransactionService
	accountID models.ID
	txnIDs    []models.ID

	// Captured on Execute
	sessionID        models.ID
	previousStatuses map[models.ID]models.TransactionStatus
	count            int
}

// NewFinishReconciliationCommand creates a command that will finish a reconciliation session.
// The accountID identifies the account being reconciled, and txnIDs are the checked
// transactions to be marked as reconciled. Previous statuses are captured at execute time.
func NewFinishReconciliationCommand(
	reconSvc *service.ReconciliationService,
	txnSvc *service.TransactionService,
	accountID models.ID,
	txnIDs []models.ID,
) *FinishReconciliationCommand {
	return &FinishReconciliationCommand{
		reconSvc:  reconSvc,
		txnSvc:    txnSvc,
		accountID: accountID,
		txnIDs:    txnIDs,
	}
}

func (c *FinishReconciliationCommand) Execute() error {
	// Capture the active session ID before finishing
	session, err := c.reconSvc.GetActiveSession(c.accountID)
	if err != nil {
		return fmt.Errorf("failed to get active session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("no active reconciliation session for account %s", c.accountID.String())
	}
	c.sessionID = session.ID

	// Capture previous statuses of transactions that will be reconciled
	c.previousStatuses = make(map[models.ID]models.TransactionStatus, len(c.txnIDs))
	for _, txnID := range c.txnIDs {
		txn, err := c.txnSvc.GetByID(txnID)
		if err != nil {
			return fmt.Errorf("failed to capture transaction %s status: %w", txnID.String(), err)
		}
		// Only capture non-reconciled transactions (already reconciled are skipped by FinishReconciliation)
		if !txn.IsReconciled() {
			c.previousStatuses[txnID] = txn.Status
		}
	}

	// Perform the reconciliation
	if err := c.reconSvc.FinishReconciliation(c.accountID, c.txnIDs, false); err != nil {
		return err
	}

	c.count = len(c.previousStatuses)
	return nil
}

func (c *FinishReconciliationCommand) Undo() error {
	// Restore transaction statuses to their pre-reconciliation state
	if err := c.reconSvc.RestoreTransactionStatuses(c.previousStatuses); err != nil {
		return fmt.Errorf("failed to restore transaction statuses: %w", err)
	}

	// Reopen the reconciliation session (completed → in_progress)
	if err := c.reconSvc.ReopenSession(c.sessionID); err != nil {
		return fmt.Errorf("failed to reopen reconciliation session: %w", err)
	}

	return nil
}

func (c *FinishReconciliationCommand) Description() string {
	return fmt.Sprintf("Reconcile %d transactions", c.count)
}
