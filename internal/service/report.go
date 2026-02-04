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

// SpendingByCategoryMonth generates a spending by category report for a specific month.
func (s *ReportService) SpendingByCategoryMonth(year int, month int) (*models.SpendingReport, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Nanosecond) // Last moment of the month

	period := startDate.Format("January 2006")
	return s.spendingByCategory(period, startDate, endDate)
}

// SpendingByCategoryYear generates a spending by category report for a specific year.
func (s *ReportService) SpendingByCategoryYear(year int) (*models.SpendingReport, error) {
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond) // Last moment of year

	period := fmt.Sprintf("%d", year)
	return s.spendingByCategory(period, startDate, endDate)
}

// SpendingByCategoryDateRange generates a spending by category report for a custom date range.
func (s *ReportService) SpendingByCategoryDateRange(startDate, endDate time.Time) (*models.SpendingReport, error) {
	period := fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	return s.spendingByCategory(period, startDate, endDate)
}

// spendingByCategory generates a spending by category report for the given period.
func (s *ReportService) spendingByCategory(period string, startDate, endDate time.Time) (*models.SpendingReport, error) {
	// Query to get spending by category, including both direct transactions and splits.
	// We need to aggregate:
	// 1. Direct transactions (non-split) with expense categories
	// 2. Split transaction amounts with expense categories
	// Exclude transfers (system categories) and only count negative amounts (spending).
	query := `
		WITH category_spending AS (
			-- Direct transactions (non-split, with category)
			SELECT
				c.id AS category_id,
				c.name AS category_name,
				c.parent_id,
				COALESCE(parent.name, '') AS parent_name,
				ABS(t.amount) AS spending
			FROM transactions t
			JOIN categories c ON CAST(t.category_id AS VARCHAR) = CAST(c.id AS VARCHAR)
			LEFT JOIN categories parent ON CAST(c.parent_id AS VARCHAR) = CAST(parent.id AS VARCHAR)
			WHERE c.type = 'expense'
			  AND c.system_category = FALSE
			  AND t.date >= ?
			  AND t.date <= ?
			  AND t.amount < 0
			  AND NOT EXISTS (
				  SELECT 1 FROM transaction_splits ts
				  WHERE CAST(ts.transaction_id AS VARCHAR) = CAST(t.id AS VARCHAR)
			  )

			UNION ALL

			-- Split transaction amounts
			SELECT
				c.id AS category_id,
				c.name AS category_name,
				c.parent_id,
				COALESCE(parent.name, '') AS parent_name,
				ABS(ts.amount) AS spending
			FROM transaction_splits ts
			JOIN categories c ON CAST(ts.category_id AS VARCHAR) = CAST(c.id AS VARCHAR)
			LEFT JOIN categories parent ON CAST(c.parent_id AS VARCHAR) = CAST(parent.id AS VARCHAR)
			JOIN transactions t ON CAST(ts.transaction_id AS VARCHAR) = CAST(t.id AS VARCHAR)
			WHERE c.type = 'expense'
			  AND c.system_category = FALSE
			  AND t.date >= ?
			  AND t.date <= ?
			  AND ts.amount < 0
		)
		SELECT
			category_id,
			category_name,
			parent_id,
			parent_name,
			SUM(spending) AS total_spending
		FROM category_spending
		GROUP BY category_id, category_name, parent_id, parent_name
		ORDER BY total_spending DESC
	`

	rows, err := s.db.Conn().Query(query, startDate, endDate, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query spending by category: %w", err)
	}
	defer rows.Close()

	// Build a map of categories and subcategories
	type rawSpending struct {
		CategoryID   models.ID
		CategoryName string
		ParentID     models.NullableID
		ParentName   string
		Amount       models.Money
	}

	var rawItems []rawSpending
	totalSpending := models.ZeroMoney

	for rows.Next() {
		var item rawSpending
		if err := rows.Scan(&item.CategoryID, &item.CategoryName, &item.ParentID, &item.ParentName, &item.Amount); err != nil {
			return nil, fmt.Errorf("failed to scan spending row: %w", err)
		}
		rawItems = append(rawItems, item)
		totalSpending = totalSpending.Add(item.Amount)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating spending rows: %w", err)
	}

	// Organize into parent categories with subcategories
	parentMap := make(map[string]*models.CategorySpending) // key is category ID string
	var topLevelCategories []models.CategorySpending

	// First pass: create entries for all categories
	for _, item := range rawItems {
		percentage := 0.0
		if !totalSpending.IsZero() {
			percentage = item.Amount.Float64() / totalSpending.Float64() * 100
		}

		spending := models.CategorySpending{
			CategoryID:    item.CategoryID,
			Name:          item.CategoryName,
			ParentID:      item.ParentID,
			ParentName:    item.ParentName,
			Amount:        item.Amount,
			Percentage:    percentage,
			Subcategories: nil,
		}

		if item.ParentID.Valid {
			// This is a subcategory - add to parent
			parentKey := item.ParentID.ID.String()
			if parent, exists := parentMap[parentKey]; exists {
				parent.Subcategories = append(parent.Subcategories, spending)
				parent.Amount = parent.Amount.Add(item.Amount)
			} else {
				// Parent doesn't have spending yet, create placeholder
				parentSpending := &models.CategorySpending{
					CategoryID:    item.ParentID.ID,
					Name:          item.ParentName,
					ParentID:      models.NullableID{Valid: false},
					ParentName:    "",
					Amount:        item.Amount,
					Subcategories: []models.CategorySpending{spending},
				}
				parentMap[parentKey] = parentSpending
			}
		} else {
			// This is a top-level category
			key := item.CategoryID.String()
			if existing, exists := parentMap[key]; exists {
				// Already have subcategory spending, update the parent info
				existing.CategoryID = item.CategoryID
				existing.Name = item.CategoryName
				existing.Amount = existing.Amount.Add(item.Amount)
			} else {
				spendingCopy := spending
				parentMap[key] = &spendingCopy
			}
		}
	}

	// Collect top-level categories and recalculate percentages
	for _, cat := range parentMap {
		percentage := 0.0
		if !totalSpending.IsZero() {
			percentage = cat.Amount.Float64() / totalSpending.Float64() * 100
		}
		cat.Percentage = percentage
		topLevelCategories = append(topLevelCategories, *cat)
	}

	// Sort by amount descending
	sortCategoriesByAmount(topLevelCategories)

	return &models.SpendingReport{
		Period:        period,
		StartDate:     startDate,
		EndDate:       endDate,
		Categories:    topLevelCategories,
		TotalSpending: totalSpending,
	}, nil
}

// sortCategoriesByAmount sorts categories by amount in descending order.
func sortCategoriesByAmount(categories []models.CategorySpending) {
	for i := 0; i < len(categories)-1; i++ {
		for j := i + 1; j < len(categories); j++ {
			if categories[j].Amount.Cmp(categories[i].Amount) > 0 {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}
}
