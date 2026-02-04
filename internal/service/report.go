package service

import (
	"fmt"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// ReportService provides business logic for generating reports.
type ReportService struct {
	accountRepo *repository.AccountRepository
	db          *db.DB
}

// NewReportService creates a new ReportService.
func NewReportService(accountRepo *repository.AccountRepository, database *db.DB) *ReportService {
	return &ReportService{
		accountRepo: accountRepo,
		db:          database,
	}
}

// NetWorth generates a net worth report as of the current date.
func (s *ReportService) NetWorth() (*models.NetWorthReport, error) {
	return s.NetWorthAsOf(time.Now())
}

// NetWorthAsOf generates a net worth report as of a specific date.
func (s *ReportService) NetWorthAsOf(asOf time.Time) (*models.NetWorthReport, error) {
	return s.netWorthAsOf(asOf, false)
}

// NetWorthAsOfIncludingClosed generates a net worth report including closed accounts.
func (s *ReportService) NetWorthAsOfIncludingClosed(asOf time.Time) (*models.NetWorthReport, error) {
	return s.netWorthAsOf(asOf, true)
}

// netWorthAsOf generates a net worth report with options.
func (s *ReportService) netWorthAsOf(asOf time.Time, includeClosed bool) (*models.NetWorthReport, error) {
	// Query to get account balances as of a specific date
	// The balance is: opening_balance + sum(transactions where date <= asOf)
	query := `
		SELECT
			a.id,
			a.name,
			a.type,
			a.opening_balance + COALESCE(
				(SELECT SUM(t.amount)
				 FROM transactions t
				 WHERE t.account_id = a.id
				 AND t.date <= ?),
				0
			) as balance
		FROM accounts a
		WHERE 1=1
	`
	args := []interface{}{asOf}

	if !includeClosed {
		query += " AND a.active = TRUE"
	}

	query += " ORDER BY a.type, a.name"

	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query account balances: %w", err)
	}
	defer rows.Close()

	var assets []models.ReportAccountBalance
	var liabilities []models.ReportAccountBalance
	totalAssets := models.ZeroMoney
	totalLiabilities := models.ZeroMoney

	for rows.Next() {
		var accountID models.ID
		var name string
		var accountType models.AccountType
		var balance models.Money

		if err := rows.Scan(&accountID, &name, &accountType, &balance); err != nil {
			return nil, fmt.Errorf("failed to scan account balance: %w", err)
		}

		accountBalance := models.ReportAccountBalance{
			AccountID: accountID,
			Name:      name,
			Type:      accountType,
			Balance:   balance,
		}

		if accountType.IsAssetType() {
			assets = append(assets, accountBalance)
			totalAssets = totalAssets.Add(balance)
		} else if accountType.IsLiabilityType() {
			liabilities = append(liabilities, accountBalance)
			// For liabilities, the balance represents what is owed
			totalLiabilities = totalLiabilities.Add(balance)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account balances: %w", err)
	}

	// Net worth = Total Assets - Total Liabilities
	netWorth := totalAssets.Sub(totalLiabilities)

	return &models.NetWorthReport{
		AsOfDate:         asOf,
		Assets:           assets,
		Liabilities:      liabilities,
		TotalAssets:      totalAssets,
		TotalLiabilities: totalLiabilities,
		NetWorth:         netWorth,
	}, nil
}
