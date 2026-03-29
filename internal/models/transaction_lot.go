package models

// TransactionLot represents a junction between an investment transaction and a lot.
// It records how many shares from a specific lot were used in a transaction (e.g., a sell).
type TransactionLot struct {
	ID            ID        `json:"id"`
	TransactionID ID        `json:"transaction_id"`
	LotID         ID        `json:"lot_id"`
	Shares        Quantity  `json:"shares"`
	CreatedAt     Timestamp `json:"created_at"`
}

// NewTransactionLot creates a new TransactionLot with the given fields.
func NewTransactionLot(transactionID, lotID ID, shares Quantity) TransactionLot {
	return TransactionLot{
		ID:            NewID(),
		TransactionID: transactionID,
		LotID:         lotID,
		Shares:        shares,
		CreatedAt:     Now(),
	}
}

// Validate checks all required fields and constraints on the TransactionLot.
func (tl *TransactionLot) Validate() ValidationErrors {
	v := NewValidator()
	v.RequiredID("transaction_id", tl.TransactionID)
	v.RequiredID("lot_id", tl.LotID)
	v.PositiveQuantity("shares", tl.Shares)
	return v.Errors()
}

// IsValid returns true if the TransactionLot passes all validation checks.
func (tl *TransactionLot) IsValid() bool {
	return !tl.Validate().HasErrors()
}
