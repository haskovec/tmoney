package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// FinishReconciliationCommand finishes a reconciliation session and can undo it
// by restoring all transaction statuses and reopening the session.
type FinishReconciliationCommand struct {
	reconSvc  *reconciliation.Service
	txnSvc    *transaction.Service
	accountID types.ID
	txnIDs    []types.ID

	// Captured on Execute
	sessionID        types.ID
	previousStatuses map[types.ID]transaction.Status
	count            int
}

// NewFinishReconciliationCommand creates a command that will finish a reconciliation session.
// The accountID identifies the account being reconciled, and txnIDs are the checked
// transactions to be marked as reconciled. Previous statuses are captured at execute time.
func NewFinishReconciliationCommand(
	reconSvc *reconciliation.Service,
	txnSvc *transaction.Service,
	accountID types.ID,
	txnIDs []types.ID,
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
	c.previousStatuses = make(map[types.ID]transaction.Status, len(c.txnIDs))
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
	// Restore transaction statuses and reopen the session atomically — one
	// service call, one transaction (mirrors the atomic FinishReconciliation).
	if err := c.reconSvc.UndoFinish(c.sessionID, c.previousStatuses); err != nil {
		return fmt.Errorf("failed to undo reconciliation finish: %w", err)
	}

	return nil
}

func (c *FinishReconciliationCommand) Description() string {
	return fmt.Sprintf("Reconcile %d transactions", c.count)
}
