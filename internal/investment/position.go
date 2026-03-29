package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// Position represents an aggregated holding of a security in an account.
// Used for non-lot-tracking accounts where individual lots are not maintained.
type Position struct {
	types.BaseModel
	AccountID           types.ID       `json:"account_id"`
	SecurityID          types.ID       `json:"security_id"`
	Shares              types.Quantity `json:"shares"`
	AverageCostPerShare types.Money    `json:"average_cost_per_share"`
}

// NewPosition creates a new Position with zero shares.
func NewPosition(accountID, securityID types.ID) Position {
	return Position{
		BaseModel:           types.NewBaseModel(),
		AccountID:           accountID,
		SecurityID:          securityID,
		Shares:              types.ZeroQuantity,
		AverageCostPerShare: types.ZeroMoney,
	}
}

// NewPositionWithShares creates a new Position with the given shares and average cost.
func NewPositionWithShares(accountID, securityID types.ID, shares types.Quantity, averageCostPerShare types.Money) Position {
	return Position{
		BaseModel:           types.NewBaseModel(),
		AccountID:           accountID,
		SecurityID:          securityID,
		Shares:              shares,
		AverageCostPerShare: averageCostPerShare,
	}
}

// CostBasis returns the total cost basis: shares x average_cost_per_share.
func (p *Position) CostBasis() types.Money {
	return p.AverageCostPerShare.Mul(p.Shares.Decimal())
}

// MarketValue returns the current market value: shares x currentPrice.
func (p *Position) MarketValue(currentPrice types.Money) types.Money {
	return currentPrice.Mul(p.Shares.Decimal())
}

// AddShares adds new shares and recalculates the weighted average cost per share.
// newShares must be positive. pricePerShare must not be negative.
func (p *Position) AddShares(newShares types.Quantity, pricePerShare types.Money) error {
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
func (p *Position) RemoveShares(shares types.Quantity) error {
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
func (p *Position) Validate() types.ValidationErrors {
	v := types.NewValidator()
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
