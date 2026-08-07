package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Estimating the amount for a variable-amount schedule from what the payee was
// actually charged recently.

// EstimateAmount calculates an estimated amount based on recent transactions.
// Uses the average of the last N transactions to the same payee (where N is AmountEstimateCount).
// Returns nil if no estimate can be calculated.
func (s *Service) EstimateAmount(id types.ID) (*types.Money, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// If the scheduled transaction has a fixed amount, return that
	if st.HasAmount() {
		return &st.Amount.Money, nil
	}

	// If no estimate count is set, we can't estimate
	if !st.AmountEstimateCount.Valid || st.AmountEstimateCount.Int64 <= 0 {
		return nil, nil
	}

	// If no payee is set, we can't estimate
	if !st.HasPayee() {
		return nil, nil
	}

	// Get recent transactions to this payee
	count := int(st.AmountEstimateCount.Int64)
	transactions, err := s.getRecentTransactionsByPayee(st.AccountID, st.PayeeID.ID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, nil
	}

	// Calculate average
	var total types.Money
	for _, txn := range transactions {
		total = total.Add(txn.Amount)
	}

	average := total.Div(int64(len(transactions)))
	return &average, nil
}

// getRecentTransactionsByPayee retrieves the most recent transactions for a payee in an account.
func (s *Service) getRecentTransactionsByPayee(accountID, payeeID types.ID, limit int) ([]*transaction.Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id, memo,
			   check_number, status, transfer_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ?
		  AND CAST(payee_id AS VARCHAR) = ?
		ORDER BY date DESC, created_at DESC
		LIMIT ?
	`

	rows, err := s.q().Query(query, accountID.String(), payeeID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*transaction.Transaction
	for rows.Next() {
		txn := &transaction.Transaction{}
		err := rows.Scan(
			&txn.ID,
			&txn.AccountID,
			&txn.Date,
			&txn.Amount,
			&txn.PayeeID,
			&txn.CategoryID,
			&txn.Memo,
			&txn.CheckNumber,
			&txn.Status,
			&txn.TransferID,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, txn)
	}

	return transactions, rows.Err()
}
