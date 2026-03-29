package investment

import "github.com/haskovec/tmoney/internal/types"

// TransactionLot represents a junction between an investment transaction and a lot.
// It records how many shares from a specific lot were used in a transaction (e.g., a sell).
type TransactionLot struct {
	ID            types.ID        `json:"id"`
	TransactionID types.ID        `json:"transaction_id"`
	LotID         types.ID        `json:"lot_id"`
	Shares        types.Quantity  `json:"shares"`
	CreatedAt     types.Timestamp `json:"created_at"`
}

// NewTransactionLot creates a new TransactionLot with the given fields.
func NewTransactionLot(transactionID, lotID types.ID, shares types.Quantity) TransactionLot {
	return TransactionLot{
		ID:            types.NewID(),
		TransactionID: transactionID,
		LotID:         lotID,
		Shares:        shares,
		CreatedAt:     types.Now(),
	}
}

// Validate checks all required fields and constraints on the TransactionLot.
func (tl *TransactionLot) Validate() types.ValidationErrors {
	v := types.NewValidator()
	v.RequiredID("transaction_id", tl.TransactionID)
	v.RequiredID("lot_id", tl.LotID)
	v.PositiveQuantity("shares", tl.Shares)
	return v.Errors()
}

// IsValid returns true if the TransactionLot passes all validation checks.
func (tl *TransactionLot) IsValid() bool {
	return !tl.Validate().HasErrors()
}
