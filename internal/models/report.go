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
