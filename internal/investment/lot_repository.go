package investment

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// LotRepository provides database operations for investment lots.
type LotRepository struct {
	db *db.DB
}

// NewLotRepository creates a new LotRepository.
func NewLotRepository(database *db.DB) *LotRepository {
	return &LotRepository{db: database}
}

// lotColumns is the standard column list for investment lots.
const lotColumns = `id, account_id, security_id, shares, original_shares, cost_per_share,
	purchase_date, source_transaction_id, closed, created_at, updated_at`

// scanLot scans a row into a Lot.
func scanLot(row interface{ Scan(...any) error }) (*Lot, error) {
	l := &Lot{}
	err := row.Scan(
		&l.ID, &l.AccountID, &l.SecurityID, &l.Shares, &l.OriginalShares,
		&l.CostPerShare, &l.PurchaseDate, &l.SourceTransactionID, &l.Closed,
		&l.CreatedAt, &l.UpdatedAt,
	)
	return l, err
}

// Create inserts a new lot into the database.
func (r *LotRepository) Create(lot *Lot) error {
	query := `
		INSERT INTO investment_lots (
			id, account_id, security_id, shares, original_shares, cost_per_share,
			purchase_date, source_transaction_id, closed, created_at, updated_at
		) VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?, ?, ?, ?, CAST(? AS UUID), ?, ?, ?)
	`
	_, err := r.db.Conn().Exec(query,
		lot.ID, lot.AccountID.String(), lot.SecurityID.String(),
		lot.Shares.String(), lot.OriginalShares.String(), lot.CostPerShare.String(),
		lot.PurchaseDate.Time(), lot.SourceTransactionID.String(),
		lot.Closed, lot.CreatedAt, lot.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create lot: %w", err)
	}
	return nil
}

// GetByID retrieves a lot by its ID.
func (r *LotRepository) GetByID(id types.ID) (*Lot, error) {
	query := `SELECT ` + lotColumns + ` FROM investment_lots WHERE CAST(id AS VARCHAR) = ?`
	l, err := scanLot(r.db.Conn().QueryRow(query, id.String()))
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "lot", ID: id.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lot: %w", err)
	}
	return l, nil
}

// ListByAccountAndSecurity retrieves lots for a specific account and security.
func (r *LotRepository) ListByAccountAndSecurity(accountID, securityID types.ID, includeClosed bool) ([]*Lot, error) {
	query := `
		SELECT ` + lotColumns + `
		FROM investment_lots
		WHERE CAST(account_id AS VARCHAR) = ? AND CAST(security_id AS VARCHAR) = ?
	`
	args := []any{accountID.String(), securityID.String()}
	if !includeClosed {
		query += " AND closed = false"
	}
	query += " ORDER BY purchase_date ASC"

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list lots: %w", err)
	}
	defer rows.Close()

	lots := make([]*Lot, 0)
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lot: %w", err)
		}
		lots = append(lots, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lots: %w", err)
	}
	return lots, nil
}

// Update updates an existing lot in the database.
func (r *LotRepository) Update(lot *Lot) error {
	lot.Touch()

	result, err := r.db.Conn().Exec(`
		UPDATE investment_lots SET
			account_id = CAST(? AS UUID),
			security_id = CAST(? AS UUID),
			shares = ?,
			original_shares = ?,
			cost_per_share = ?,
			purchase_date = ?,
			source_transaction_id = CAST(? AS UUID),
			closed = ?,
			updated_at = ?
		WHERE CAST(id AS VARCHAR) = ?
	`,
		lot.AccountID.String(), lot.SecurityID.String(),
		lot.Shares.String(), lot.OriginalShares.String(), lot.CostPerShare.String(),
		lot.PurchaseDate.Time(), lot.SourceTransactionID.String(),
		lot.Closed, lot.UpdatedAt.Time(),
		lot.ID.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update lot: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return &dberrors.NotFoundError{Entity: "lot", ID: lot.ID.String()}
	}
	return nil
}

// UpdateSharesAndClosed updates only the shares and closed fields of an
// existing lot. Narrow-write helper used by sell/edit and the rebuild
// tool to avoid a read-modify-write round trip through Update.
func (r *LotRepository) UpdateSharesAndClosed(id types.ID, shares types.Quantity, closed bool) error {
	res, err := r.db.Conn().Exec(
		`UPDATE investment_lots SET shares = ?, closed = ?, updated_at = ? WHERE CAST(id AS VARCHAR) = ?`,
		shares.String(), closed, types.Now().Time(), id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to update lot shares/closed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if n == 0 {
		return &dberrors.NotFoundError{Entity: "lot", ID: id.String()}
	}
	return nil
}

// GetBySourceTransaction retrieves the lot (if any) created by the given source transaction.
// Returns NotFoundError when no lot references the transaction.
func (r *LotRepository) GetBySourceTransaction(txnID types.ID) (*Lot, error) {
	query := `SELECT ` + lotColumns + ` FROM investment_lots WHERE CAST(source_transaction_id AS VARCHAR) = ?`
	l, err := scanLot(r.db.Conn().QueryRow(query, txnID.String()))
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "lot", ID: txnID.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lot by source transaction: %w", err)
	}
	return l, nil
}

// Delete removes a lot from the database. The lot's junction rows are NOT
// touched — callers must ensure no junctions reference the lot before deleting
// (otherwise foreign-key style consistency is violated).
func (r *LotRepository) Delete(id types.ID) error {
	res, err := r.db.Conn().Exec(`DELETE FROM investment_lots WHERE CAST(id AS VARCHAR) = ?`, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete lot: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if n == 0 {
		return &dberrors.NotFoundError{Entity: "lot", ID: id.String()}
	}
	return nil
}

// ListAllByAccount retrieves every lot for an account (open and closed) for
// every security. Used by the rebuild-positions tool to recompute lot
// shares/closed from junction records.
func (r *LotRepository) ListAllByAccount(accountID types.ID) ([]*Lot, error) {
	query := `SELECT ` + lotColumns + ` FROM investment_lots WHERE CAST(account_id AS VARCHAR) = ? ORDER BY purchase_date ASC, created_at ASC`
	rows, err := r.db.Conn().Query(query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list lots: %w", err)
	}
	defer rows.Close()

	lots := make([]*Lot, 0)
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lot: %w", err)
		}
		lots = append(lots, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lots: %w", err)
	}
	return lots, nil
}

// HasOpenLots returns true if any account holds open lots for the given security.
func (r *LotRepository) HasOpenLots(securityID types.ID) (bool, error) {
	var count int
	err := r.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM investment_lots WHERE CAST(security_id AS VARCHAR) = ? AND closed = false`,
		securityID.String(),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check open lots: %w", err)
	}
	return count > 0, nil
}

// GetOpenLotsBySecurity returns all open (non-closed) lots across all accounts for a given security.
func (r *LotRepository) GetOpenLotsBySecurity(securityID types.ID) ([]*Lot, error) {
	query := `
		SELECT ` + lotColumns + `
		FROM investment_lots
		WHERE CAST(security_id AS VARCHAR) = ? AND closed = false
		ORDER BY purchase_date ASC
	`
	rows, err := r.db.Conn().Query(query, securityID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get open lots by security: %w", err)
	}
	defer rows.Close()

	lots := make([]*Lot, 0)
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lot: %w", err)
		}
		lots = append(lots, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating lots: %w", err)
	}
	return lots, nil
}
