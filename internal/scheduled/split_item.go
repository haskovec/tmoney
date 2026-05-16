package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/types"
)

// Split represents one line of a multi-line scheduled transaction template.
//
// A line is either categorized (CategoryID set, TransferAccountID zero) or
// a transfer-line that targets another account (TransferAccountID set,
// CategoryID = NilID). The two shapes are mutually exclusive and enforced
// at the database via a CHECK constraint (see migration 015). Templates do
// not carry a transfer_id — the paired transaction (and its transfer_id)
// is minted at post time by the transaction service.
type Split struct {
	types.BaseModel

	// Core properties
	ScheduledTransactionID types.ID    `json:"scheduled_transaction_id"`
	Amount                 types.Money `json:"amount"`

	// Exactly one of CategoryID / TransferAccountID is set per row.
	CategoryID        types.NullableID `json:"category_id"`
	TransferAccountID types.NullableID `json:"transfer_account_id"`

	// Optional properties
	Memo types.NullableString `json:"memo"`
}

// NewCategorizedSplit creates a categorized scheduled-split row.
func NewCategorizedSplit(scheduledTransactionID, categoryID types.ID, amount types.Money) *Split {
	return &Split{
		BaseModel:              types.NewBaseModel(),
		ScheduledTransactionID: scheduledTransactionID,
		Amount:                 amount,
		CategoryID:             types.NullableID{ID: categoryID, Valid: true},
	}
}

// NewTransferSplit creates a transfer-typed scheduled-split row targeting
// another account.
func NewTransferSplit(scheduledTransactionID, transferAccountID types.ID, amount types.Money) *Split {
	return &Split{
		BaseModel:              types.NewBaseModel(),
		ScheduledTransactionID: scheduledTransactionID,
		Amount:                 amount,
		TransferAccountID:      types.NullableID{ID: transferAccountID, Valid: true},
	}
}

// IsTransfer returns true if this split is a transfer-line.
func (s *Split) IsTransfer() bool {
	return s.TransferAccountID.Valid
}

// IsCategorized returns true if this split is a categorized line.
func (s *Split) IsCategorized() bool {
	return s.CategoryID.Valid
}

// SetMemo sets the memo for this split.
func (s *Split) SetMemo(memo string) {
	if memo == "" {
		s.Memo = types.NullableString{Valid: false}
	} else {
		s.Memo = types.NullableString{String: memo, Valid: true}
	}
	s.Touch()
}

// ClearMemo removes the memo from this split.
func (s *Split) ClearMemo() {
	s.Memo = types.NullableString{Valid: false}
	s.Touch()
}

// Validate validates the scheduled split and returns any validation errors.
func (s *Split) Validate() types.ValidationErrors {
	v := types.NewValidator()

	v.RequiredID("scheduled_transaction_id", s.ScheduledTransactionID)

	// Exactly one of category_id / transfer_account_id must be set.
	switch {
	case s.CategoryID.Valid && s.TransferAccountID.Valid:
		v.AddError("split", "cannot set both category_id and transfer_account_id")
	case !s.CategoryID.Valid && !s.TransferAccountID.Valid:
		v.AddError("split", "must set exactly one of category_id or transfer_account_id")
	}

	if s.Amount.IsZero() {
		v.AddError("amount", "cannot be zero")
	}

	if s.Memo.Valid {
		v.MaxLength("memo", s.Memo.String, 500)
	}

	return v.Errors()
}

// IsValid returns true if the scheduled split passes validation.
func (s *Split) IsValid() bool {
	return !s.Validate().HasErrors()
}

// SplitCollection represents a collection of scheduled splits for a template.
type SplitCollection []*Split

// Total returns the signed sum of all split amounts.
func (sc SplitCollection) Total() types.Money {
	total := types.ZeroMoney
	for _, s := range sc {
		total = total.Add(s.Amount)
	}
	return total
}

// ValidateAgainstTemplate validates that the signed sum of lines equals the
// template's net amount. Multi-line schedules with mixed signs must net to
// the parent's stored amount (see specs/multiline-splits-and-paycheck.md).
func (sc SplitCollection) ValidateAgainstTemplate(parentAmount types.Money) types.ValidationErrors {
	var errors types.ValidationErrors
	if len(sc) == 0 {
		return errors
	}

	total := sc.Total()
	if !total.Equal(parentAmount) {
		errors.Add("splits", fmt.Sprintf(
			"split total (%s) does not equal scheduled transaction amount (%s)",
			total.String(), parentAmount.String(),
		))
	}
	return errors
}
