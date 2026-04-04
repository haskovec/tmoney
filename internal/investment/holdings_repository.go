package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// ViewHolding represents a row from the portfolio_holdings database view.
type ViewHolding struct {
	AccountID      types.ID
	AccountName    string
	SecurityID     types.ID
	Ticker         string
	SecurityName   string
	TotalShares    types.Quantity
	TotalCostBasis types.Money
}

// HoldingsRepository queries the portfolio_holdings database view.
type HoldingsRepository struct {
	db *db.DB
}

// NewHoldingsRepository creates a new HoldingsRepository.
func NewHoldingsRepository(database *db.DB) *HoldingsRepository {
	return &HoldingsRepository{db: database}
}

// ListByAccount returns all holdings with non-zero shares for a given account.
func (r *HoldingsRepository) ListByAccount(accountID types.ID) ([]ViewHolding, error) {
	query := `
		SELECT account_id, account_name, security_id, ticker, security_name, total_shares, total_cost_basis
		FROM portfolio_holdings
		WHERE CAST(account_id AS VARCHAR) = ? AND total_shares > 0
		ORDER BY ticker ASC
	`
	rows, err := r.db.Conn().Query(query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query portfolio_holdings view: %w", err)
	}
	defer rows.Close()

	holdings := make([]ViewHolding, 0)
	for rows.Next() {
		var h ViewHolding
		err := rows.Scan(&h.AccountID, &h.AccountName, &h.SecurityID, &h.Ticker, &h.SecurityName, &h.TotalShares, &h.TotalCostBasis)
		if err != nil {
			return nil, fmt.Errorf("failed to scan view holding: %w", err)
		}
		holdings = append(holdings, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating portfolio_holdings: %w", err)
	}

	return holdings, nil
}
