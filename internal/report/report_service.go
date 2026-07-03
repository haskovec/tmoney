package report

import (
	"fmt"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// ValuationResult holds the total value of an investment account and flags missing pricing.
type ValuationResult struct {
	TotalValue       types.Money
	HasMissingPrices bool // true if any holdings used cost basis instead of market price
}

// InvestmentValuer computes the total value of an investment account (cash + holdings).
type InvestmentValuer interface {
	GetAccountValuation(accountID types.ID, asOf types.Date) (*ValuationResult, error)
}

// Service provides business logic for generating reports.
type Service struct {
	accountRepo     *account.Repository
	db              *db.DB
	investmentValue InvestmentValuer
}

// ServiceOption configures optional dependencies for the report Service.
type ServiceOption func(*Service)

// WithInvestmentValuer sets the investment valuation provider.
func WithInvestmentValuer(v InvestmentValuer) ServiceOption {
	return func(s *Service) {
		s.investmentValue = v
	}
}

// NewService creates a new Service.
func NewService(accountRepo *account.Repository, database *db.DB, opts ...ServiceOption) *Service {
	s := &Service{
		accountRepo: accountRepo,
		db:          database,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NetWorthReport generates a net worth report as of the current date.
func (s *Service) NetWorthReport() (*NetWorth, error) {
	return s.NetWorthAsOf(time.Now())
}

// NetWorthAsOf generates a net worth report as of a specific date.
func (s *Service) NetWorthAsOf(asOf time.Time) (*NetWorth, error) {
	return s.netWorthAsOf(asOf, false)
}

// NetWorthAsOfIncludingClosed generates a net worth report including closed accounts.
func (s *Service) NetWorthAsOfIncludingClosed(asOf time.Time) (*NetWorth, error) {
	return s.netWorthAsOf(asOf, true)
}

// netWorthAsOf generates a net worth report with options.
func (s *Service) netWorthAsOf(asOf time.Time, includeClosed bool) (*NetWorth, error) {
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
				 AND t.date <= ?
				 AND t.status != 'void'),
				0
			) as balance
		FROM accounts a
		WHERE 1=1
	`
	args := []any{asOf}

	if !includeClosed {
		query += " AND a.active = TRUE"
	}

	query += " ORDER BY a.type, a.name"

	rows, err := s.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query account balances: %w", err)
	}
	defer rows.Close()

	var assets []AccountBalance
	var liabilities []AccountBalance
	totalAssets := types.ZeroMoney
	totalLiabilities := types.ZeroMoney

	for rows.Next() {
		var accountID types.ID
		var name string
		var accountType account.Type
		var balance types.Money

		if err := rows.Scan(&accountID, &name, &accountType, &balance); err != nil {
			return nil, fmt.Errorf("failed to scan account balance: %w", err)
		}

		accountBalance := AccountBalance{
			AccountID: accountID,
			Name:      name,
			Type:      string(accountType),
			Balance:   balance,
		}

		// For investment accounts, use the investment valuer to get total value
		// (cash + holdings market value) instead of the raw transaction balance.
		if accountType.IsInvestmentType() && s.investmentValue != nil {
			asOfDate := types.NewDate(asOf.Year(), asOf.Month(), asOf.Day())
			result, err := s.investmentValue.GetAccountValuation(accountID, asOfDate)
			if err == nil {
				accountBalance.Balance = result.TotalValue
				accountBalance.EstimatedValue = result.HasMissingPrices
				balance = result.TotalValue
			}
			// On error, fall through to use the transaction-based balance
		}

		if accountType.IsAssetType() {
			assets = append(assets, accountBalance)
			totalAssets = totalAssets.Add(balance)
		} else if accountType.IsLiabilityType() {
			liabilities = append(liabilities, accountBalance)
			// Liability balances are stored signed (negative = owed), so the
			// total stays signed too; presentation layers negate for display.
			totalLiabilities = totalLiabilities.Add(balance)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account balances: %w", err)
	}

	// Net worth = assets + liabilities over signed balances (liabilities ≤ 0
	// when owed), per the standardized liability sign convention.
	netWorth := totalAssets.Add(totalLiabilities)

	return &NetWorth{
		AsOfDate:         asOf,
		Assets:           assets,
		Liabilities:      liabilities,
		TotalAssets:      totalAssets,
		TotalLiabilities: totalLiabilities,
		NetWorth:         netWorth,
	}, nil
}

// SpendingByCategoryMonth generates a spending by category report for a specific month.
// When includeTransfers is false, categorized transfers are excluded (today's
// default); when true, they are folded into the totals.
func (s *Service) SpendingByCategoryMonth(year int, month int, includeTransfers bool) (*Spending, error) {
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Nanosecond) // Last moment of the month

	period := startDate.Format("January 2006")
	return s.spendingByCategory(period, startDate, endDate, includeTransfers)
}

// SpendingByCategoryYear generates a spending by category report for a specific year.
// See SpendingByCategoryMonth for the includeTransfers semantics.
func (s *Service) SpendingByCategoryYear(year int, includeTransfers bool) (*Spending, error) {
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond) // Last moment of year

	period := fmt.Sprintf("%d", year)
	return s.spendingByCategory(period, startDate, endDate, includeTransfers)
}

// SpendingByCategoryDateRange generates a spending by category report for a custom date range.
// See SpendingByCategoryMonth for the includeTransfers semantics.
func (s *Service) SpendingByCategoryDateRange(startDate, endDate time.Time, includeTransfers bool) (*Spending, error) {
	period := fmt.Sprintf("%s to %s", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	return s.spendingByCategory(period, startDate, endDate, includeTransfers)
}

// spendingByCategory generates a spending by category report for the given period.
func (s *Service) spendingByCategory(period string, startDate, endDate time.Time, includeTransfers bool) (*Spending, error) {
	// Query to get spending by category, including both direct transactions and splits.
	// We need to aggregate:
	// 1. Direct transactions (non-split) with expense categories
	// 2. Split transaction amounts with expense categories
	// Only count negative amounts (spending) and skip void rows.
	//
	// Transfers are excluded by explicit guards, not by the NULL-category
	// join accident: a categorized transfer (e.g. a linked credit-card
	// payment) must not count as spending. When includeTransfers is true the
	// guards drop, folding categorized transfers in; each mirrored pair still
	// counts at most once because only negative-amount (outflow) rows sum.
	txnTransferGuard := "AND t.transfer_id IS NULL"
	splitTransferGuard := "AND ts.transfer_account_id IS NULL"
	if includeTransfers {
		txnTransferGuard = ""
		splitTransferGuard = ""
	}
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
			  AND t.status != 'void'
			  ` + txnTransferGuard + `
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
			  AND t.status != 'void'
			  ` + splitTransferGuard + `
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
		CategoryID   types.ID
		CategoryName string
		ParentID     types.NullableID
		ParentName   string
		Amount       types.Money
	}

	var rawItems []rawSpending
	totalSpending := types.ZeroMoney

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
	parentMap := make(map[string]*CategorySpending) // key is category ID string
	var topLevelCategories []CategorySpending

	// First pass: create entries for all categories
	for _, item := range rawItems {
		percentage := 0.0
		if !totalSpending.IsZero() {
			percentage = item.Amount.Float64() / totalSpending.Float64() * 100
		}

		spending := CategorySpending{
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
				parentSpending := &CategorySpending{
					CategoryID:    item.ParentID.ID,
					Name:          item.ParentName,
					ParentID:      types.NullableID{Valid: false},
					ParentName:    "",
					Amount:        item.Amount,
					Subcategories: []CategorySpending{spending},
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

	return &Spending{
		Period:        period,
		StartDate:     startDate,
		EndDate:       endDate,
		Categories:    topLevelCategories,
		TotalSpending: totalSpending,
	}, nil
}

// sortCategoriesByAmount sorts categories by amount in descending order.
func sortCategoriesByAmount(categories []CategorySpending) {
	for i := 0; i < len(categories)-1; i++ {
		for j := i + 1; j < len(categories); j++ {
			if categories[j].Amount.Cmp(categories[i].Amount) > 0 {
				categories[i], categories[j] = categories[j], categories[i]
			}
		}
	}
}
