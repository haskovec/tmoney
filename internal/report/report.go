package report

import (
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// NetWorth represents the net worth report data.
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
	AccountID types.ID
	Name      string
	Type      string
	Balance   types.Money
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
