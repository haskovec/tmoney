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
	tx db.Queryer // nil outside a transaction
}

// NewHoldingsRepository creates a new HoldingsRepository.
func NewHoldingsRepository(database *db.DB) *HoldingsRepository {
	return &HoldingsRepository{db: database}
}

// q returns the active Queryer: the bound transaction if any, else the
// live connection. All SQL in this repo goes through q().
func (r *HoldingsRepository) q() db.Queryer {
	if r.tx != nil {
		return r.tx
	}
	return r.db.Conn()
}

// WithTx returns a copy of the repository bound to tx. The original is
// unchanged and remains safe for non-transactional use.
func (r *HoldingsRepository) WithTx(tx db.Queryer) *HoldingsRepository {
	c := *r
	c.tx = tx
	return &c
}

// ListByAccount returns all holdings with non-zero shares for a given account.
func (r *HoldingsRepository) ListByAccount(accountID types.ID) ([]ViewHolding, error) {
	query := `
		SELECT account_id, account_name, security_id, ticker, security_name, total_shares, total_cost_basis
		FROM portfolio_holdings
		WHERE CAST(account_id AS VARCHAR) = ? AND total_shares > 0
		ORDER BY ticker ASC
	`
	rows, err := r.q().Query(query, accountID.String())
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
