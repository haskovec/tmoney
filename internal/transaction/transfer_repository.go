package transaction

import (
	"database/sql"
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// TransferRepository provides database operations for linked transfer transactions.
type TransferRepository struct {
	db      *db.DB
	txnRepo *Repository
}

// NewTransferRepository creates a new TransferRepository.
func NewTransferRepository(database *db.DB, txnRepo *Repository) *TransferRepository {
	return &TransferRepository{
		db:      database,
		txnRepo: txnRepo,
	}
}

// Create creates both sides of a transfer pair in the database.
func (r *TransferRepository) Create(pair *TransferPair) error {
	// Validate the transfer pair
	if errors := pair.Validate(); errors.HasErrors() {
		return fmt.Errorf("invalid transfer pair: %v", errors)
	}

	// Create the from transaction
	if err := r.txnRepo.Create(pair.FromTransaction); err != nil {
		return fmt.Errorf("failed to create from transaction: %w", err)
	}

	// Create the to transaction. If this fails, manually unwind the from
	// transaction — the two Create calls are not wrapped in a SQL transaction,
	// so partial state is otherwise possible.
	if err := r.txnRepo.Create(pair.ToTransaction); err != nil {
		_ = r.txnRepo.Delete(pair.FromTransaction.ID)
		return fmt.Errorf("failed to create to transaction: %w", err)
	}

	return nil
}

// GetByTransferID retrieves both sides of a transfer by the shared transfer ID.
func (r *TransferRepository) GetByTransferID(transferID types.ID) (*TransferPair, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(transfer_id AS VARCHAR) = ?
		ORDER BY amount ASC
	`

	rows, err := r.db.Conn().Query(query, transferID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query transfer transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		txn := &Transaction{}
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
			&txn.TransferAccountID,
			&txn.BankReferenceID,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, txn)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, &dberrors.NotFoundError{Entity: "transfer", ID: transferID.String()}
	}

	if len(transactions) != 2 {
		return nil, fmt.Errorf("expected 2 transactions for transfer, found %d", len(transactions))
	}

	// The from transaction has negative amount (money leaving)
	// The to transaction has positive amount (money arriving)
	// We ordered by amount ASC, so negative comes first
	return &TransferPair{
		FromTransaction: transactions[0],
		ToTransaction:   transactions[1],
	}, nil
}

// GetOtherSide retrieves the other transaction in a transfer pair given one transaction ID.
func (r *TransferRepository) GetOtherSide(transactionID types.ID) (*Transaction, error) {
	// First get the transaction to find its transfer_id
	txn, err := r.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	if !txn.IsTransfer() {
		return nil, fmt.Errorf("transaction %s is not a transfer", transactionID.String())
	}

	// Find the other transaction with the same transfer_id
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(transfer_id AS VARCHAR) = ? AND CAST(id AS VARCHAR) != ?
	`

	other := &Transaction{}
	err = r.db.Conn().QueryRow(query, txn.TransferID.ID.String(), transactionID.String()).Scan(
		&other.ID,
		&other.AccountID,
		&other.Date,
		&other.Amount,
		&other.PayeeID,
		&other.CategoryID,
		&other.Memo,
		&other.CheckNumber,
		&other.Status,
		&other.TransferID,
		&other.TransferAccountID,
		&other.BankReferenceID,
		&other.CreatedAt,
		&other.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, &dberrors.NotFoundError{Entity: "transfer counterpart", ID: transactionID.String()}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer counterpart: %w", err)
	}

	return other, nil
}

// Update updates both sides of a transfer pair.
// It ensures amounts remain equal and opposite, and transfer links are consistent.
func (r *TransferRepository) Update(pair *TransferPair) error {
	// Validate the transfer pair
	if errors := pair.Validate(); errors.HasErrors() {
		return fmt.Errorf("invalid transfer pair: %v", errors)
	}

	// Update the from transaction
	if err := r.txnRepo.Update(pair.FromTransaction); err != nil {
		return fmt.Errorf("failed to update from transaction: %w", err)
	}

	// Update the to transaction
	if err := r.txnRepo.Update(pair.ToTransaction); err != nil {
		return fmt.Errorf("failed to update to transaction: %w", err)
	}

	return nil
}

// UpdateAmount updates the amount on both sides of a transfer.
// The amount should be positive (it will be negated for the from side).
func (r *TransferRepository) UpdateAmount(transferID types.ID, newAmount types.Money) error {
	pair, err := r.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Update amounts (from is negative, to is positive)
	pair.FromTransaction.Amount = newAmount.Neg()
	pair.ToTransaction.Amount = newAmount.Abs()

	return r.Update(pair)
}

// UpdateDate updates the date on both sides of a transfer.
func (r *TransferRepository) UpdateDate(transferID types.ID, newDate types.Date) error {
	pair, err := r.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	pair.FromTransaction.Date = newDate
	pair.ToTransaction.Date = newDate

	return r.Update(pair)
}

// UpdateMemo updates the memo on both sides of a transfer.
func (r *TransferRepository) UpdateMemo(transferID types.ID, memo string) error {
	pair, err := r.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	pair.FromTransaction.SetMemo(memo)
	pair.ToTransaction.SetMemo(memo)

	return r.Update(pair)
}

// UpdateStatus updates the status on both sides of a transfer.
func (r *TransferRepository) UpdateStatus(transferID types.ID, status Status) error {
	pair, err := r.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	pair.FromTransaction.SetStatus(status)
	pair.ToTransaction.SetStatus(status)

	return r.Update(pair)
}

// Delete removes both sides of a transfer from the database.
func (r *TransferRepository) Delete(transferID types.ID) error {
	pair, err := r.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Delete both transactions
	if err := r.txnRepo.Delete(pair.FromTransaction.ID); err != nil {
		return fmt.Errorf("failed to delete from transaction: %w", err)
	}

	if err := r.txnRepo.Delete(pair.ToTransaction.ID); err != nil {
		return fmt.Errorf("failed to delete to transaction: %w", err)
	}

	return nil
}

// DeleteByTransactionID removes both sides of a transfer given one transaction ID.
func (r *TransferRepository) DeleteByTransactionID(transactionID types.ID) error {
	// Get the transaction to find its transfer_id
	txn, err := r.txnRepo.GetByID(transactionID)
	if err != nil {
		return err
	}

	if !txn.IsTransfer() {
		return fmt.Errorf("transaction %s is not a transfer", transactionID.String())
	}

	return r.Delete(txn.TransferID.ID)
}

// IsTransfer checks if a transaction is part of a transfer.
func (r *TransferRepository) IsTransfer(transactionID types.ID) (bool, error) {
	txn, err := r.txnRepo.GetByID(transactionID)
	if err != nil {
		return false, err
	}
	return txn.IsTransfer(), nil
}

// ListByAccount retrieves all transfer transactions for a specific account.
func (r *TransferRepository) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	query := `
		SELECT id, account_id, date, amount, payee_id, category_id,
			memo, check_number, status, transfer_id, transfer_account_id,
			bank_reference_id, created_at, updated_at
		FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ? AND transfer_id IS NOT NULL
		ORDER BY date DESC, created_at DESC
	`

	rows, err := r.db.Conn().Query(query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query transfer transactions: %w", err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		txn := &Transaction{}
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
			&txn.TransferAccountID,
			&txn.BankReferenceID,
			&txn.CreatedAt,
			&txn.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transaction: %w", err)
		}
		transactions = append(transactions, txn)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating transactions: %w", err)
	}

	return transactions, nil
}

// CountByAccount returns the number of transfer transactions for an account.
func (r *TransferRepository) CountByAccount(accountID types.ID) (int, error) {
	var count int
	err := r.db.Conn().QueryRow(`
		SELECT COUNT(*) FROM transactions
		WHERE CAST(account_id AS VARCHAR) = ? AND transfer_id IS NOT NULL
	`, accountID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count transfers: %w", err)
	}
	return count, nil
}
