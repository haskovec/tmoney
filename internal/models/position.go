package models

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
)

// Position represents an aggregated holding of a security in an account.
// Used for non-lot-tracking accounts where individual lots are not maintained.
type Position struct {
	BaseModel
	AccountID           ID       `json:"account_id"`
	SecurityID          ID       `json:"security_id"`
	Shares              Quantity `json:"shares"`
	AverageCostPerShare Money    `json:"average_cost_per_share"`
}

// NewPosition creates a new Position with zero shares.
func NewPosition(accountID, securityID ID) Position {
	return Position{
		BaseModel:           NewBaseModel(),
		AccountID:           accountID,
		SecurityID:          securityID,
		Shares:              ZeroQuantity,
		AverageCostPerShare: ZeroMoney,
	}
}

// NewPositionWithShares creates a new Position with the given shares and average cost.
func NewPositionWithShares(accountID, securityID ID, shares Quantity, averageCostPerShare Money) Position {
	return Position{
		BaseModel:           NewBaseModel(),
		AccountID:           accountID,
		SecurityID:          securityID,
		Shares:              shares,
		AverageCostPerShare: averageCostPerShare,
	}
}

// CostBasis returns the total cost basis: shares × average_cost_per_share.
func (p *Position) CostBasis() Money {
	return p.AverageCostPerShare.Mul(p.Shares.Decimal())
}

// MarketValue returns the current market value: shares × currentPrice.
func (p *Position) MarketValue(currentPrice Money) Money {
	return currentPrice.Mul(p.Shares.Decimal())
}

// AddShares adds new shares and recalculates the weighted average cost per share.
// newShares must be positive. pricePerShare must not be negative.
func (p *Position) AddShares(newShares Quantity, pricePerShare Money) error {
	if !newShares.IsPositive() {
		return fmt.Errorf("new shares must be positive, got %s", newShares)
	}
	if pricePerShare.IsNegative() {
		return fmt.Errorf("price per share must not be negative, got %s", pricePerShare)
	}

	// Weighted average: (existing_cost + new_cost) / total_shares
	existingCost := p.CostBasis()
	newCost := pricePerShare.Mul(newShares.Decimal())
	totalCost := existingCost.Add(newCost)
	totalShares := p.Shares.Add(newShares)

	p.Shares = totalShares
	p.AverageCostPerShare = totalCost.Mul(alpacadecimal.NewFromInt(1).Div(totalShares.Decimal()))
	p.Touch()
	return nil
}

// RemoveShares reduces the share count without changing the average cost.
// shares must be positive and not exceed current holdings.
func (p *Position) RemoveShares(shares Quantity) error {
	if !shares.IsPositive() {
		return fmt.Errorf("shares to remove must be positive, got %s", shares)
	}
	remaining := p.Shares.Sub(shares)
	if remaining.IsNegative() {
		return fmt.Errorf("cannot remove %s shares: only %s held", shares, p.Shares)
	}
	p.Shares = remaining
	p.Touch()
	return nil
}

// Validate checks all required fields and constraints on the Position.
func (p *Position) Validate() ValidationErrors {
	v := NewValidator()
	v.RequiredID("account_id", p.AccountID)
	v.RequiredID("security_id", p.SecurityID)
	v.NonNegativeQuantity("shares", p.Shares)
	v.NonNegative("average_cost_per_share", p.AverageCostPerShare)
	return v.Errors()
}

// IsValid returns true if the position passes all validation checks.
func (p *Position) IsValid() bool {
	return !p.Validate().HasErrors()
}
