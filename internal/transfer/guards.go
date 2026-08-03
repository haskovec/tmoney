package transfer

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// resolveAccounts loads both leg accounts, or reports which one is missing.
func (s *Service) resolveAccounts(fromID, toID types.ID) (from, to *account.Account, err error) {
	from, err = s.accountRepo.GetByID(fromID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load from-account: %w", err)
	}
	to, err = s.accountRepo.GetByID(toID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load to-account: %w", err)
	}
	return from, to, nil
}

// guardSpec runs every precondition for writing a transfer, in a FIXED order.
//
// The order is part of the contract. Today the error a user sees for, say, a
// same-account transfer between closed investment accounts depends entirely on
// which of the four paths they happened to take: transaction.CreateTransfer
// raises a ServiceValidationError wrapped in "failed to create transfer:", while
// the three investment paths raise a bare "cannot transfer between the same
// account" — and each checks the rules in a different sequence.
//
//  1. amount strictly positive     -> *InvalidAmountError
//  2. from != to                   -> *SameAccountError
//  3. both accounts exist          -> dberrors.NotFoundError (via resolveAccounts)
//  4. neither account closed       -> account.AccountClosedError
//  5. date >= both opening dates   -> account.ValidateTransactionDate's error
//  6. category exists + non-system -> dberrors.NotFoundError /
//     *transaction.SystemCategoryTransferError
//  7. category storable for Kind   -> *CategoryNotSupportedError
//  8. status valid                 -> error naming the invalid status
//
// Steps 6 and 7 are NEW ENFORCEMENT for the three investment paths:
// internal/investment performs no category validation whatsoever, and its
// comments explicitly delegate it to the caller. Inputs the investment paths
// accept today are rejected here. That is the point.
func (s *Service) guardSpec(spec Spec) (from, to *account.Account, err error) {
	if !spec.Amount.IsPositive() {
		return nil, nil, &InvalidAmountError{Amount: spec.Amount}
	}
	if spec.FromAccountID == spec.ToAccountID {
		return nil, nil, &SameAccountError{AccountID: spec.FromAccountID}
	}

	from, to, err = s.resolveAccounts(spec.FromAccountID, spec.ToAccountID)
	if err != nil {
		return nil, nil, err
	}

	for _, acct := range []*account.Account{from, to} {
		if acct.IsClosed() {
			return nil, nil, &account.AccountClosedError{ID: acct.ID.String()}
		}
	}

	// Both legs are date-guarded against their OWN account's opening date. The
	// old paths guarded only the regular leg explicitly and relied on the
	// investment service's validateTransaction for the other, so a date guard
	// existed for every shape but through two different mechanisms.
	for _, acct := range []*account.Account{from, to} {
		if err := acct.ValidateTransactionDate(spec.Date); err != nil {
			return nil, nil, err
		}
	}

	kind := ClassifyKind(from.Type, to.Type)
	if err := s.guardCategory(kind, spec.CategoryID); err != nil {
		return nil, nil, err
	}

	if !spec.Status.IsValid() {
		return nil, nil, fmt.Errorf("invalid transfer status: %q", spec.Status)
	}
	if spec.Status == transaction.StatusVoid {
		// A transfer is voided through Void, not by writing void legs: the
		// investment ledger has no void status, and Void's contract is to
		// preserve the pre-void amounts for Restore.
		return nil, nil, fmt.Errorf("cannot create or edit a transfer directly into the void status; use Void")
	}

	return from, to, nil
}

// guardCategory enforces both halves of the category rule: the category must
// exist and be non-system, and the transfer's Kind must have somewhere to store
// it.
//
// The second half is the domain home for a refusal currently implemented in five
// presentation-layer sites and nowhere in the domain — which is why
// investment.UpdateTransferCash can silently drop a category on its inv↔inv
// branch today.
func (s *Service) guardCategory(kind Kind, categoryID types.NullableID) error {
	if !categoryID.Valid {
		return nil
	}
	if !kind.StoresCategory() {
		return &CategoryNotSupportedError{Kind: kind}
	}

	cat, err := s.categoryRepo.GetByID(categoryID.ID)
	if err != nil {
		var notFound *dberrors.NotFoundError
		if errors.As(err, &notFound) {
			return err
		}
		return fmt.Errorf("failed to load transfer category: %w", err)
	}
	// Reuses the single existing owner of the non-system rule rather than
	// restating it.
	return transaction.ValidateTransferCategory(cat)
}

// checkMutable reports whether an existing transfer may be edited or deleted.
//
// Every rule here is enforced for bank↔bank today and for nothing else:
// transaction.Service.checkTransferEditable covers the regular pair, while
// internal/investment has no reconciled guard at all (grep IsReconciled in
// investment_service.go returns one hit, inside FindTransferCashCounterpart), so
// the TUI can currently edit and delete reconciled investment transfers with no
// complaint.
//
// Both legs are inspected, so a HALF-reconciled pair is refused — which is the
// case that matters, since reconciliation reconciles one leg at a time and never
// touches investment_transactions.
func checkMutable(t *Transfer) error {
	if t.Shape == ShapeSplitLine {
		return &SplitLineError{TransferID: t.TransferID, ParentID: t.ParentTransactionID}
	}
	if t.Movement == MovementShares {
		return &ShareTransferError{TransferID: t.TransferID}
	}
	for _, leg := range t.Legs() {
		if leg.Status == transaction.StatusReconciled {
			return &ReconciledLegError{TransferID: t.TransferID, RowID: leg.RowID}
		}
	}
	return nil
}

// checkLive additionally refuses a transfer with a voided leg, for the verbs
// that need a live transfer (edit, reverse, void).
func checkLive(t *Transfer) error {
	if err := checkMutable(t); err != nil {
		return err
	}
	for _, leg := range t.Legs() {
		if leg.Status == transaction.StatusVoid {
			return &VoidLegError{TransferID: t.TransferID, RowID: leg.RowID}
		}
	}
	return nil
}
