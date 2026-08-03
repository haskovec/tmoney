package transfer

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
)

// The status mapping between the two ledgers lives HERE, in internal/transfer,
// and deliberately not in internal/investment.
//
// It was previously investment.statusFromRegular / a TUI copy / a CLI copy —
// three divergent implementations. Consolidating it looks like it belongs in
// internal/investment next to the TransactionStatus constants, but these
// functions name transaction.Status, so any home inside internal/investment
// preserves the internal/investment → internal/transaction import edge that
// specs/design-unified-transfer.md §5.2 exists to sever. internal/transfer
// already imports both ledgers, and after the phase-5 deletions it is the
// mapping's only consumer.

// UnrepresentableStatusError is returned when a regular transaction.Status has
// no investment-ledger equivalent. investment_transactions has no `void`
// status, so a voided regular leg cannot be mirrored onto an investment leg.
//
// The mapping this replaces (investment.statusFromRegular, update_edit.go:19)
// silently coerced void→pending, so a void regular leg round-tripped through an
// edit came back Uncleared. Returning an error instead makes the loss visible.
type UnrepresentableStatusError struct {
	Status transaction.Status
}

func (e *UnrepresentableStatusError) Error() string {
	return fmt.Sprintf(
		"transaction status %q has no investment-ledger equivalent; investment_transactions has no void status",
		e.Status,
	)
}

// StatusToRegular maps an investment leg's status onto the canonical
// transaction.Status used by the read model and every front end. Investment
// "pending" and regular "uncleared" name the same state.
//
// Total: every valid investment.TransactionStatus has a regular equivalent.
func StatusToRegular(s investment.TransactionStatus) transaction.Status {
	switch s {
	case investment.TransactionStatusCleared:
		return transaction.StatusCleared
	case investment.TransactionStatusReconciled:
		return transaction.StatusReconciled
	default:
		return transaction.StatusUncleared
	}
}

// StatusFromRegular maps a regular transaction.Status onto the investment
// ledger. It returns *UnrepresentableStatusError for transaction.StatusVoid,
// which has no investment equivalent, rather than silently degrading it.
func StatusFromRegular(s transaction.Status) (investment.TransactionStatus, error) {
	switch s {
	case transaction.StatusCleared:
		return investment.TransactionStatusCleared, nil
	case transaction.StatusReconciled:
		return investment.TransactionStatusReconciled, nil
	case transaction.StatusUncleared:
		return investment.TransactionStatusPending, nil
	case transaction.StatusVoid:
		return "", &UnrepresentableStatusError{Status: s}
	default:
		return "", fmt.Errorf("invalid transaction status: %q", s)
	}
}
