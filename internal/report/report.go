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
	// then zero and must not be shown. That currency's net worth, and the
	// total of the side the account is on, are not available; the other
	// side's total still shows (see CurrencyTotal).
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

// Holdings is the holdings report: what the active investment accounts hold
// today, added up across the accounts. There is one section per currency;
// money in different currencies is never added.
type Holdings struct {
	AsOfDate types.Date
	Sections []HoldingsSection // sorted by currency code
	// Failed lists the accounts that could not be valued. Their positions
	// and cash are in no row, and their currency's section is not Available.
	Failed []AccountFailure
}

// HoldingsSection is the holdings of one currency.
type HoldingsSection struct {
	Currency  string
	Rows      []HoldingRow // largest Value first; the Cash row sorts with the others
	Value     types.Money  // the sum of the rows' Value
	CostBasis types.Money  // the sum of the rows' CostBasis
	Gain      types.Money  // the sum of the priced rows' Gain
	// Available is false when an account in this currency could not be
	// valued. Value then leaves that account out and must not be shown as
	// the total. Each row's Percent is of the accounts that were valued.
	Available bool
	// Estimated is true when a row has no price and is valued at cost.
	Estimated bool
}

// HoldingRow is one security across all the accounts, or the Cash row.
type HoldingRow struct {
	SecurityID types.ID // NilID on the Cash row
	Cash       bool
	Label      string         // the ticker, or the name when there is no ticker; "Cash"
	Name       string         // "Uninvested cash" on the Cash row
	Shares     types.Quantity // zero on the Cash row
	Price      types.Money    // zero on the Cash row and when Estimated
	Value      types.Money
	CostBasis  types.Money      // the cash itself on the Cash row
	Gain       types.Money      // Value - CostBasis; zero on the Cash row and when Estimated
	Percent    float64          // of the section's Value
	Estimated  bool             // no price: Value is the cost basis
	Accounts   []HoldingAccount // largest Value first
}

// HoldingAccount is one account's part of a HoldingRow.
type HoldingAccount struct {
	AccountID types.ID
	Name      string
	Shares    types.Quantity // zero on the Cash row
	Value     types.Money
	Percent   float64 // of the row's Value
}

// AccountFailure is an account that a report could not value.
type AccountFailure struct {
	AccountID types.ID
	Name      string
	Currency  string
	Err       error
}
