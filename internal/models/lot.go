package models

import "fmt"

// Lot represents a specific purchase lot of shares in an investment account.
// Lots track cost basis for tax purposes using specific identification.
type Lot struct {
	BaseModel
	AccountID           ID       `json:"account_id"`
	SecurityID          ID       `json:"security_id"`
	Shares              Quantity `json:"shares"`
	OriginalShares      Quantity `json:"original_shares"`
	CostPerShare        Money    `json:"cost_per_share"`
	PurchaseDate        Date     `json:"purchase_date"`
	SourceTransactionID ID       `json:"source_transaction_id"`
	Closed              bool     `json:"closed"`
}

// NewLot creates a new Lot with the given required fields.
// OriginalShares is set equal to shares. Closed defaults to false.
func NewLot(accountID, securityID ID, shares Quantity, costPerShare Money, purchaseDate Date, sourceTransactionID ID) Lot {
	return Lot{
		BaseModel:           NewBaseModel(),
		AccountID:           accountID,
		SecurityID:          securityID,
		Shares:              shares,
		OriginalShares:      shares,
		CostPerShare:        costPerShare,
		PurchaseDate:        purchaseDate,
		SourceTransactionID: sourceTransactionID,
		Closed:              false,
	}
}

// CostBasis returns the current cost basis: shares × cost_per_share.
func (l *Lot) CostBasis() Money {
	return l.CostPerShare.Mul(l.Shares.Decimal())
}

// IsFullyClosed returns true if the lot has zero remaining shares.
func (l *Lot) IsFullyClosed() bool {
	return l.Shares.IsZero()
}

// Reduce decreases the lot's shares by the given amount.
// If shares reach zero, the lot is automatically marked as closed.
// Returns an error if amount is not positive, exceeds available shares, or the lot is already closed.
func (l *Lot) Reduce(amount Quantity) error {
	if !amount.IsPositive() {
		return fmt.Errorf("reduce amount must be positive, got %s", amount)
	}
	if l.Closed {
		return fmt.Errorf("cannot reduce a closed lot")
	}
	remaining := l.Shares.Sub(amount)
	if remaining.IsNegative() {
		return fmt.Errorf("cannot reduce by %s: only %s shares available", amount, l.Shares)
	}
	l.Shares = remaining
	if l.Shares.IsZero() {
		l.Closed = true
	}
	l.Touch()
	return nil
}

// Validate checks all required fields and constraints on the Lot.
func (l *Lot) Validate() ValidationErrors {
	v := NewValidator()
	v.RequiredID("account_id", l.AccountID)
	v.RequiredID("security_id", l.SecurityID)
	v.RequiredID("source_transaction_id", l.SourceTransactionID)
	v.RequiredDate("purchase_date", l.PurchaseDate)
	v.Positive("cost_per_share", l.CostPerShare)
	v.PositiveQuantity("original_shares", l.OriginalShares)

	// Shares must be positive unless the lot is closed
	if !l.Closed {
		v.PositiveQuantity("shares", l.Shares)
	} else {
		v.NonNegativeQuantity("shares", l.Shares)
	}

	return v.Errors()
}

// IsValid returns true if the lot passes all validation checks.
func (l *Lot) IsValid() bool {
	return !l.Validate().HasErrors()
}
