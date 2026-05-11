package investment

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// TransactionLotRepository provides database operations for investment transaction-lot junctions.
type TransactionLotRepository struct {
	db *db.DB
}

// NewTransactionLotRepository creates a new TransactionLotRepository.
func NewTransactionLotRepository(database *db.DB) *TransactionLotRepository {
	return &TransactionLotRepository{db: database}
}

const transactionLotColumns = `id, transaction_id, lot_id, shares, created_at`

func scanTransactionLot(row interface{ Scan(...any) error }) (*TransactionLot, error) {
	tl := &TransactionLot{}
	err := row.Scan(&tl.ID, &tl.TransactionID, &tl.LotID, &tl.Shares, &tl.CreatedAt)
	return tl, err
}

// Create inserts a new transaction-lot junction record into the database.
func (r *TransactionLotRepository) Create(tl *TransactionLot) error {
	query := `
		INSERT INTO investment_transaction_lots (
			id, transaction_id, lot_id, shares, created_at
		) VALUES (?, CAST(? AS UUID), CAST(? AS UUID), ?, ?)
	`
	_, err := r.db.Conn().Exec(query,
		tl.ID, tl.TransactionID.String(), tl.LotID.String(),
		tl.Shares.String(), tl.CreatedAt.Time(),
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction lot: %w", err)
	}
	return nil
}

// SumSharesByLot returns total consumed shares per lot ID for the supplied
// lot IDs. Lots with no junctions are returned with a zero quantity.
func (r *TransactionLotRepository) SumSharesByLot(lotIDs []types.ID) (map[types.ID]types.Quantity, error) {
	result := make(map[types.ID]types.Quantity, len(lotIDs))
	for _, id := range lotIDs {
		result[id] = types.ZeroQuantity
	}
	if len(lotIDs) == 0 {
		return result, nil
	}

	// Build placeholder list and arg slice
	var sb strings.Builder
	args := make([]any, 0, len(lotIDs))
	for i, id := range lotIDs {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteByte('?')
		args = append(args, id.String())
	}

	query := `SELECT CAST(lot_id AS VARCHAR), shares FROM investment_transaction_lots WHERE CAST(lot_id AS VARCHAR) IN (` + sb.String() + `)`
	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query junctions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var idStr string
		var shares types.Quantity
		if err := rows.Scan(&idStr, &shares); err != nil {
			return nil, fmt.Errorf("failed to scan junction row: %w", err)
		}
		id, err := types.ParseID(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse lot id %q: %w", idStr, err)
		}
		existing := result[id]
		result[id] = existing.Add(shares)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating junctions: %w", err)
	}
	return result, nil
}

// GetByTransaction retrieves all lot allocations for a given transaction.
func (r *TransactionLotRepository) GetByTransaction(transactionID types.ID) ([]*TransactionLot, error) {
	query := `
		SELECT ` + transactionLotColumns + `
		FROM investment_transaction_lots
		WHERE CAST(transaction_id AS VARCHAR) = ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.Conn().Query(query, transactionID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction lots: %w", err)
	}
	defer rows.Close()

	results := make([]*TransactionLot, 0)
	for rows.Next() {
		tl, err := scanTransactionLot(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction lot: %w", err)
		}
		results = append(results, tl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transaction lots: %w", err)
	}
	return results, nil
}
