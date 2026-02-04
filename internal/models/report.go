package models

import (
	"time"
)

// NetWorthReport represents the net worth report data.
type NetWorthReport struct {
	AsOfDate         time.Time
	Assets           []ReportAccountBalance
	Liabilities      []ReportAccountBalance
	TotalAssets      Money
	TotalLiabilities Money
	NetWorth         Money
}

// ReportAccountBalance holds balance information for an account in a report.
type ReportAccountBalance struct {
	AccountID ID
	Name      string
	Type      AccountType
	Balance   Money
}

// SpendingReport represents spending by category for a given time period.
type SpendingReport struct {
	Period        string
	StartDate     time.Time
	EndDate       time.Time
	Categories    []CategorySpending
	TotalSpending Money
}

// CategorySpending holds spending information for a single category.
type CategorySpending struct {
	CategoryID    ID
	Name          string
	ParentID      NullableID
	ParentName    string
	Amount        Money
	Percentage    float64
	Subcategories []CategorySpending
}
