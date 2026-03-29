package investment

import (
	"fmt"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/types"
)

// ComputePricePerShare calculates the price per share from total amount, shares, and optional commission.
// Formula: price_per_share = (total_amount - commission) / shares
// If commission is zero, it simplifies to total_amount / shares.
func ComputePricePerShare(totalAmount types.Money, shares types.Quantity, commission types.Money) (types.Money, error) {
	if shares.IsZero() {
		return types.ZeroMoney, fmt.Errorf("shares must not be zero")
	}
	net := totalAmount.Sub(commission)
	reciprocal := alpacadecimal.NewFromInt(1).Div(shares.Decimal())
	return net.Mul(reciprocal), nil
}

// ComputeTotalAmount calculates the total amount from shares, price per share, and optional commission.
// Formula: total_amount = (shares * price_per_share) + commission
// If commission is zero, it simplifies to shares * price_per_share.
func ComputeTotalAmount(shares types.Quantity, pricePerShare types.Money, commission types.Money) types.Money {
	base := pricePerShare.Mul(shares.Decimal())
	return base.Add(commission)
}

// SmartComputeResult holds the computed fields for a buy/sell transaction.
type SmartComputeResult struct {
	TotalAmount   types.Money
	PricePerShare types.Money
}

// SmartCompute auto-fills missing fields for a buy/sell transaction.
// At least shares + one of (totalAmount, pricePerShare) is required.
// If both totalAmount and pricePerShare are provided, totalAmount takes precedence
// and pricePerShare is recomputed.
func SmartCompute(
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
) (*SmartComputeResult, error) {
	if shares.IsZero() || shares.IsNegative() {
		return nil, fmt.Errorf("shares must be positive")
	}

	hasTotalAmount := totalAmount != nil
	hasPricePerShare := pricePerShare != nil

	if !hasTotalAmount && !hasPricePerShare {
		return nil, fmt.Errorf("at least one of total_amount or price_per_share is required")
	}

	result := &SmartComputeResult{}

	if hasTotalAmount {
		// Total given — compute price_per_share from it
		result.TotalAmount = *totalAmount
		price, err := ComputePricePerShare(*totalAmount, shares, commission)
		if err != nil {
			return nil, fmt.Errorf("failed to compute price_per_share: %w", err)
		}
		result.PricePerShare = price
	} else {
		// Only price given — compute total from it
		result.PricePerShare = *pricePerShare
		result.TotalAmount = ComputeTotalAmount(shares, *pricePerShare, commission)
	}

	return result, nil
}
