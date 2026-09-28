package report

import (
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// NetWorth represents the net worth report data.
//
// All balances are signed: liability balances are stored negative when money
// is owed (a $250,000 mortgage sits at -250,000), so a currency's NetWorth is
// its Assets plus its Liabilities. Presentation layers that list liabilities
// under an explicit LIABILITIES heading render the raw signed balance — a debt
// shows negative and an overpaid loan or credit-balance card shows positive (a
// credit) — rather than negating it.
//
// There is one total per currency in Totals, sorted by currency code. Money in
// different currencies is never added.
type NetWorth struct {
	AsOfDate    time.Time
	Assets      []AccountBalance
	Liabilities []AccountBalance
	Totals      []CurrencyTotal
	// InvestmentAsOfApproximate is true when the report is as of a past date
	// and lists an investment account. Such an account shows its current cash
	// and shares, priced as of the date: it is not a historical replay.
	InvestmentAsOfApproximate bool
}

// AccountBalance holds balance information for an account in a report.
type AccountBalance struct {
	AccountID      types.ID
	Name           string
	Type           string
	Currency       string
	Balance        types.Money
	EstimatedValue bool // true when any holding uses cost basis due to missing pricing data
	// Err is set when an investment account could not be valued. Balance is
	// then zero and must not be shown, and the account's currency has no
	// total (see CurrencyTotal).
	Err error
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
