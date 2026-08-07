package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Field-level validation and the two guards every write path runs first: the
// account must be open, and a payee's default category is applied before the
// transaction is checked.

// validateTransaction validates a transaction and returns any validation errors.
func (s *Service) validateTransaction(transaction *Transaction) error {
	errors := transaction.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	// Reject activity dated before the account opened (catches mistyped years
	// such as "0018" for "2018"). Voided rows retain their original date and
	// are not re-validated here.
	if !transaction.IsVoid() {
		acct, err := s.accountRepo.GetByID(transaction.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load account for date validation: %w", err)
		}
		if err := acct.ValidateTransactionDate(transaction.Date); err != nil {
			return err
		}
	}
	return nil
}

// validateSplit validates a split and returns any validation errors.
func (s *Service) validateSplit(split *Split) error {
	errors := split.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplits validates that splits' signed sum equals the transaction
// amount. Mixed-sign lines are allowed: each line carries its own sign
// independent of the parent. Transfer-typed lines additionally must not
// target the parent's own account.
// See SplitCollection.ValidateAgainstTransaction.
func (s *Service) validateSplits(transaction *Transaction, splits []*Split) error {
	if len(splits) == 0 {
		return nil
	}

	for _, split := range splits {
		if err := s.validateSplit(split); err != nil {
			return err
		}
		if split.TransferAccountID.Valid && split.TransferAccountID.ID == transaction.AccountID {
			errors := types.ValidationErrors{}
			errors.Add("transfer_account_id",
				(&SelfTransferError{AccountID: transaction.AccountID.String()}).Error())
			return &types.ServiceValidationError{Errors: errors}
		}
	}

	collection := SplitCollection(splits)
	errors := collection.ValidateAgainstTransaction(transaction.Amount)
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}

	return nil
}

// applyPayeeDefaultCategory sets the transaction category from the payee's default.
func (s *Service) applyPayeeDefaultCategory(transaction *Transaction) error {
	if s.payeeRepo == nil || !transaction.HasPayee() {
		return nil
	}

	p, err := s.payeeRepo.GetByID(transaction.PayeeID.ID)
	if err != nil {
		// Payee not found is not an error here - just skip
		if _, ok := err.(*dberrors.NotFoundError); ok {
			return nil
		}
		return fmt.Errorf("failed to get payee: %w", err)
	}

	if p.HasDefaultCategory() {
		transaction.SetCategory(p.DefaultCategoryID.ID)
	}

	return nil
}

// ensureAccountOpen returns an AccountClosedError when the account is closed.
// A closed account is frozen: no new transactions, edits, deletes, or status
// toggles. It is nil-tolerant for test fixtures constructed without an
// accountRepo (matching targetIsInvestment); the production wiring always
// passes a real repository.
func (s *Service) ensureAccountOpen(id types.ID) error {
	if s.accountRepo == nil {
		return nil
	}
	acct, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("failed to load account for closed check: %w", err)
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: id.String()}
	}
	return nil
}
