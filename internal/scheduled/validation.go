package scheduled

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Validation for a scheduled transaction and its split lines, including the rule
// that a transfer line may not carry a category.

// validateScheduledTransaction validates a scheduled transaction and returns any validation errors.
func (s *Service) validateScheduledTransaction(st *Transaction) error {
	errors := st.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateTransferCategory checks that a single-line transfer schedule's
// optional category label exists and is non-system (Transaction.Validate
// defers this rule to the service layer). Mirrors
// transaction.Service.validateTransferCategory — a raw lookup against the
// categories table via the service's db handle, delegating the rule itself to
// the shared transaction.ValidateTransferCategory guard so there is a single
// source of truth. Nil-tolerant for fixtures constructed without a db handle;
// production always wires one.
func (s *Service) validateTransferCategory(st *Transaction) error {
	if !st.IsTransfer() || !st.HasCategory() || s.db == nil {
		return nil
	}
	var name string
	var isSystem bool
	err := s.q().QueryRow(
		`SELECT name, system_category FROM categories WHERE CAST(id AS VARCHAR) = ?`,
		st.CategoryID.ID.String(),
	).Scan(&name, &isSystem)
	if errors.Is(err, sql.ErrNoRows) {
		return &dberrors.NotFoundError{Entity: "category", ID: st.CategoryID.ID.String()}
	}
	if err != nil {
		return fmt.Errorf("failed to check transfer category: %w", err)
	}
	return transaction.ValidateTransferCategory(&category.Category{Name: name, IsSystem: isSystem})
}

// validateScheduledSplits validates a scheduled transaction's child splits
// when present. Enforces:
//   - mutually exclusive shape: scalar category_id on the parent and child
//     splits cannot coexist (a multi-line schedule has no parent category);
//   - the multi-line parent must carry a fixed amount (variable amounts are
//     legacy single-line only — see specs/multiline-splits-and-paycheck.md);
//   - each split passes Split.Validate() (one of category_id /
//     transfer_account_id, non-zero amount, etc.);
//   - transfer-lines cannot target the parent's own account (self-transfer);
//   - the signed sum of split amounts equals the parent's amount.
//
// Returns nil for legacy single-line schedules (no child splits).
func (s *Service) validateScheduledSplits(st *Transaction) error {
	if len(st.Splits) == 0 {
		return nil
	}

	if st.HasCategory() {
		verrs := types.ValidationErrors{}
		verrs.Add("splits",
			"scheduled transaction cannot set both a scalar category_id and child splits")
		return &types.ServiceValidationError{Errors: verrs}
	}

	if !st.HasAmount() {
		verrs := types.ValidationErrors{}
		verrs.Add("amount",
			"multi-line scheduled transaction requires a fixed amount equal to the signed sum of its lines")
		return &types.ServiceValidationError{Errors: verrs}
	}

	for _, split := range st.Splits {
		// Ensure the parent linkage is set so split-level validation
		// doesn't trip on the required scheduled_transaction_id field.
		if split.ScheduledTransactionID.IsNil() {
			split.ScheduledTransactionID = st.ID
		}
		if errs := split.Validate(); errs.HasErrors() {
			return &types.ServiceValidationError{Errors: errs}
		}
		if split.TransferAccountID.Valid && split.TransferAccountID.ID == st.AccountID {
			verrs := types.ValidationErrors{}
			verrs.Add("transfer_account_id",
				"transfer-line cannot target the scheduled transaction's own account (self-transfer)")
			return &types.ServiceValidationError{Errors: verrs}
		}
	}

	if errs := st.Splits.ValidateAgainstTemplate(st.Amount.Money); errs.HasErrors() {
		return &types.ServiceValidationError{Errors: errs}
	}

	return nil
}
