package report

import (
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// NetWorth represents the net worth report data.
//
// All balances are signed: liability balances are stored negative when money
// is owed (a $250,000 mortgage sits at -250,000), so
// NetWorth = TotalAssets + TotalLiabilities. Presentation layers that list
// liabilities under an explicit LIABILITIES heading render the negated
// balance (negation, not abs — an overpaid loan or credit-balance card
// displays negative, which correctly reads as a credit).
type NetWorth struct {
	AsOfDate         time.Time
	Assets           []AccountBalance
	Liabilities      []AccountBalance
	TotalAssets      types.Money
	TotalLiabilities types.Money
	NetWorth         types.Money
}

// AccountBalance holds balance information for an account in a report.
type AccountBalance struct {
	AccountID      types.ID
	Name           string
	Type           string
	Balance        types.Money
	EstimatedValue bool // true when any holding uses cost basis due to missing pricing data
}

// Spending represents spending by category for a given time period.
type Spending struct {
	Period        string
	StartDate     time.Time
	EndDate       time.Time
	Categories    []CategorySpending
	TotalSpending types.Money
}

// CategorySpending holds spending information for a single category.
type CategorySpending struct {
	CategoryID    types.ID
	Name          string
	ParentID      types.NullableID
	ParentName    string
	Amount        types.Money
	Percentage    float64
	Subcategories []CategorySpending
}
