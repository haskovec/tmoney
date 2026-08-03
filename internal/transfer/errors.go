package transfer

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// MalformedPairError is returned when a transfer_id does not name exactly two
// legs across the two ledgers. It names the per-table counts so the failure is
// diagnosable.
//
// This replaces transaction.TransferRepository.GetByTransferID's bare
// "expected 2 transactions for transfer, found N" error, which counted rows on
// the `transactions` table ONLY — making it structurally incapable of seeing an
// inv↔reg or inv↔inv pair, and the reason `tmoney transaction void <bank leg of
// an inv↔reg transfer>` fails today.
type MalformedPairError struct {
	TransferID     types.ID
	RegularLegs    int
	InvestmentLegs int
}

func (e *MalformedPairError) Error() string {
	return fmt.Sprintf(
		"transfer %s is malformed: expected exactly 2 legs across both ledgers, found %d regular and %d investment",
		e.TransferID.String(), e.RegularLegs, e.InvestmentLegs,
	)
}

// SplitLineError is returned by every verb when the transfer_id is owned by a
// multi-line split's transfer-line item (e.g. a paycheck's 401k contribution
// line). Those are owned by transaction.Service's split lifecycle; edit them
// through the parent transaction's split dialog.
//
// Reads deliberately SUCCEED for a split line (Shape == ShapeSplitLine) so
// callers can explain the refusal rather than reporting "not found".
type SplitLineError struct {
	TransferID types.ID
	ParentID   types.ID
}

func (e *SplitLineError) Error() string {
	return fmt.Sprintf(
		"transfer %s is a transfer line inside a multi-line split (parent transaction %s); "+
			"edit it through the parent's splits",
		e.TransferID.String(), e.ParentID.String(),
	)
}

// ShareTransferError is returned when a cash verb is handed a transfer_shares
// pair. Share transfers are owned by investment.Service (TransferShares /
// UpdateTransferShares); this package owns cash only.
//
// Today no such guard exists: investment.Service.UpdateTransferCash never
// checks the row type, so handing it a share transfer's — or a buy's — ID
// deletes the row via repo.Delete without calling reverseTxnEffects, silently
// corrupting lots and positions.
type ShareTransferError struct {
	TransferID types.ID
}

func (e *ShareTransferError) Error() string {
	return fmt.Sprintf(
		"transfer %s moves shares, not cash; use the investment share-transfer operations",
		e.TransferID.String(),
	)
}

// InvalidAmountError is returned when a transfer amount is not strictly
// positive. Callers always supply a positive magnitude; the signs are applied
// per leg by planLegs.
//
// This is the single owner of a rule that exists today as two identical types,
// transaction.InvalidTransferAmountError and
// investment.InvalidTransferAmountError, which cannot be matched with a single
// errors.As.
type InvalidAmountError struct {
	Amount types.Money
}

func (e *InvalidAmountError) Error() string {
	return fmt.Sprintf("transfer amount must be positive, got %s", e.Amount.String())
}

// SameAccountError is returned when both legs of a transfer name the same
// account. Today this rule is enforced at four different points in four
// different orders across the four paths, so the error a user sees depends on
// which path they took.
type SameAccountError struct {
	AccountID types.ID
}

func (e *SameAccountError) Error() string {
	return fmt.Sprintf("cannot transfer to the same account (%s)", e.AccountID.String())
}

// CategoryNotSupportedError is returned when a category is supplied for a Kind
// that has nowhere to store it. Only inv↔inv qualifies: both its legs live in
// investment_transactions, which has no category_id column.
//
// Refusing beats silently dropping, which is what
// investment.Service.UpdateTransferCash does today on its inv↔inv branch
// (update_edit.go:608-633).
type CategoryNotSupportedError struct {
	Kind Kind
}

func (e *CategoryNotSupportedError) Error() string {
	return fmt.Sprintf(
		"a %s transfer cannot carry a category: both legs live in investment_transactions, which has no category column",
		e.Kind.String(),
	)
}

// ReconciledLegError is returned when an edit or delete would touch a
// reconciled leg. Today the domain enforces this for bank↔bank only
// (transaction.Service.checkTransferEditable); internal/investment has no
// reconciled guard at all, so the TUI can silently edit and delete reconciled
// investment transfers.
type ReconciledLegError struct {
	TransferID types.ID
	RowID      types.ID
}

func (e *ReconciledLegError) Error() string {
	return fmt.Sprintf(
		"transfer %s has a reconciled leg (%s); unreconcile it before editing or deleting",
		e.TransferID.String(), e.RowID.String(),
	)
}

// VoidLegError is returned when a verb that requires a live transfer is handed
// one with a voided leg.
type VoidLegError struct {
	TransferID types.ID
	RowID      types.ID
}

func (e *VoidLegError) Error() string {
	return fmt.Sprintf("transfer %s has a voided leg (%s)", e.TransferID.String(), e.RowID.String())
}

// VoidNotSupportedError is returned when a void is attempted on a transfer with
// an investment leg. investment_transactions' status CHECK constraint has no
// `void` value, so there is nowhere to record it; closing that gap needs a
// migration plus position/lot reversal and total-return exclusion.
//
// This replaces the misleading "expected 2 transactions for transfer, found 1"
// a user sees today.
type VoidNotSupportedError struct {
	TransferID types.ID
	Kind       Kind
}

func (e *VoidNotSupportedError) Error() string {
	return fmt.Sprintf(
		"a %s transfer cannot be voided: investment_transactions has no void status (delete it instead)",
		e.Kind.String(),
	)
}
