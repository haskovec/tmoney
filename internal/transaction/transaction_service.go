package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// InvestmentCashCounterpartAdapter lets the transaction service create,
// inspect, and clean up an investment.Transaction row that serves as the
// paired counterpart of a transfer-line split whose target is an
// investment account (e.g. a paycheck → 401k contribution line, an
// auto-deposit to a brokerage).
//
// The transaction package cannot import investment (investment already
// imports transaction). Wiring an adapter at app-construction time
// breaks the cycle. When the adapter is nil, transfer-line splits to
// investment accounts are rejected at the service layer rather than
// silently creating a malformed regular-table row.
type InvestmentCashCounterpartAdapter interface {
	// CreateTransferCashCounterpart mints the investment-side row.
	// amount carries the sign in the destination frame (positive = cash
	// arriving, negative = cash leaving); the caller provides the
	// shared transferID. Returns the new row's ID for rollback.
	CreateTransferCashCounterpart(
		invAcctID, otherAcctID types.ID,
		date types.Date,
		amount types.Money,
		memo string,
		transferID types.ID,
	) (types.ID, error)

	// FindTransferCashCounterpart returns the investment row linked to
	// the given transferID. found=false means no investment-side row
	// exists for this transferID (the counterpart may live on the
	// regular table, or no counterpart was ever minted).
	FindTransferCashCounterpart(transferID types.ID) (rowID types.ID, reconciled bool, found bool, err error)

	// DeleteTransferCashCounterpart removes the investment row by ID.
	DeleteTransferCashCounterpart(rowID types.ID) error

	// UpdateTransferCashCounterpartAmount mirrors a transfer-line amount
	// edit onto the investment row. newAmount is in the destination
	// frame (same convention as CreateTransferCashCounterpart).
	UpdateTransferCashCounterpartAmount(rowID types.ID, newAmount types.Money) error
}

// Service provides business logic for transaction operations.
type Service struct {
	txnRepo               *Repository
	splitRepo             *SplitRepository
	transferRepo          *TransferRepository
	payeeRepo             *payee.Repository
	accountRepo           *account.Repository
	investmentCounterpart InvestmentCashCounterpartAdapter
	db                    *db.DB
}

// NewService creates a new Service.
func NewService(
	txnRepo *Repository,
	splitRepo *SplitRepository,
	transferRepo *TransferRepository,
	payeeRepo *payee.Repository,
	accountRepo *account.Repository,
	database *db.DB,
) *Service {
	return &Service{
		txnRepo:      txnRepo,
		splitRepo:    splitRepo,
		transferRepo: transferRepo,
		payeeRepo:    payeeRepo,
		accountRepo:  accountRepo,
		db:           database,
	}
}

// SetInvestmentCounterpart wires an adapter for routing transfer-line
// splits whose target is an investment account through the investment
// service. Wired after construction so transaction.NewService can be
// called before investment.NewService (which depends on transaction).
// Calling with nil disables the dispatch (transfer-line splits to
// investment accounts will be rejected with NotRegularAccountError).
func (s *Service) SetInvestmentCounterpart(a InvestmentCashCounterpartAdapter) {
	s.investmentCounterpart = a
}

// =============================================================================
// Transaction CRUD Operations
// =============================================================================

// Create validates and creates a new transaction.
// If the transaction has a payee with a default category and no category is set,
// the default category will be auto-populated.
func (s *Service) Create(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
		return err
	}

	// Auto-populate category from payee's default if not set
	if !transaction.HasCategory() && transaction.HasPayee() {
		if err := s.applyPayeeDefaultCategory(transaction); err != nil {
			return err
		}
	}

	return s.txnRepo.Create(transaction)
}

// GetByID retrieves a transaction by its ID.
func (s *Service) GetByID(id types.ID) (*Transaction, error) {
	return s.txnRepo.GetByID(id)
}

// Update validates and updates an existing transaction.
// For transfers, use UpdateTransfer to update both sides.
// Void and reconciled transactions cannot be edited.
//
// If the transaction is the paired single-line counter-transaction of a
// multi-line split (i.e. its transfer_id matches a split-item's transfer_id),
// an amount change mirrors the new (negated) amount onto the parent's
// transfer-line split-item. A reconciled parent transaction blocks the
// reverse cascade with IsReconciledError.
func (s *Service) Update(transaction *Transaction) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	// Check existing transaction state
	existing, err := s.txnRepo.GetByID(transaction.ID)
	if err != nil {
		return err
	}

	// Prevent editing void transactions
	if existing.IsVoid() {
		return &IsVoidError{ID: transaction.ID.String()}
	}

	// Prevent editing reconciled transactions
	if existing.IsReconciled() {
		return &IsReconciledError{ID: transaction.ID.String()}
	}

	// Check if this is a transfer - caller should use UpdateTransfer
	if existing.IsTransfer() && !transaction.IsTransfer() {
		return &IsTransferError{ID: transaction.ID.String()}
	}

	// A closed account is frozen — block edits (incl. the cleared toggle,
	// which routes through here). Guard both the current account and, if the
	// edit moves the transaction, the destination account.
	if err := s.ensureAccountOpen(existing.AccountID); err != nil {
		return err
	}
	if transaction.AccountID != existing.AccountID {
		if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
			return err
		}
	}

	// Reverse cascade: if this transaction is the paired side of a multi-
	// line split, mirror an amount change back onto the parent's split-item.
	if existing.IsTransfer() && existing.TransferID.Valid {
		parentSplit, err := s.splitRepo.GetByTransferID(existing.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit != nil && !existing.Amount.Equal(transaction.Amount) {
			if err := s.cascadeAmountToParentSplit(parentSplit, transaction.Amount.Neg()); err != nil {
				return err
			}
		}
	}

	return s.txnRepo.Update(transaction)
}

// cascadeAmountToParentSplit mirrors an amount edit on the paired single-
// line counter-transaction back onto the parent multi-line split-item. The
// parent's split row is rewritten with the negated paired amount. A
// reconciled parent transaction blocks the cascade with IsReconciledError.
func (s *Service) cascadeAmountToParentSplit(parentSplit *Split, newSplitAmount types.Money) error {
	parent, err := s.txnRepo.GetByID(parentSplit.TransactionID)
	if err != nil {
		return err
	}
	if parent.IsReconciled() {
		return &IsReconciledError{ID: parent.ID.String()}
	}
	parentSplit.Amount = newSplitAmount
	if err := s.splitRepo.Update(parentSplit); err != nil {
		return fmt.Errorf("failed to mirror amount to parent split-item: %w", err)
	}
	return nil
}

// Delete removes a transaction.
// For transfers, this will delete both sides after confirmation.
// Void and reconciled transactions cannot be deleted.
func (s *Service) Delete(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Prevent deleting void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Prevent deleting reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// A closed account is frozen — block deletes.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// If this is a transfer, check both sides and delete
	if txn.IsTransfer() {
		// Reverse cascade: if this transaction is the paired side of a
		// multi-line split (its transfer_id matches a split-item's
		// transfer_id), remove the parent's transfer-line split-item and
		// then delete the paired transaction. Skip the legacy two-side
		// transfer path, which assumes both legs live on the transactions
		// table.
		parentSplit, err := s.splitRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit != nil {
			return s.deletePairedSideOfMultiLine(txn, parentSplit)
		}

		pair, err := s.transferRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if pair.FromTransaction.IsReconciled() {
			return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
		}
		if pair.ToTransaction.IsReconciled() {
			return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
		}
		// Block the delete if either leg lives on a closed account.
		if err := s.ensureAccountOpen(pair.ToTransaction.AccountID); err != nil {
			return err
		}
		return s.transferRepo.Delete(txn.TransferID.ID)
	}

	// Cascade to paired counter-transactions of any transfer-typed split-
	// lines before removing the parent. The parent itself is not marked as a
	// transfer (only the split-item carries the linkage), so the legacy
	// transfer branch above does not run.
	if err := s.deleteTransferLinePairs(id); err != nil {
		return err
	}

	// Delete any splits first
	if _, err := s.splitRepo.DeleteByTransaction(id); err != nil {
		return fmt.Errorf("failed to delete splits: %w", err)
	}

	return s.txnRepo.Delete(id)
}

// deletePairedSideOfMultiLine reverse-cascades a paired-side delete back to
// the parent multi-line transaction: the parent's transfer-line split-item
// is removed first, then the paired single-line counter-transaction itself.
// A reconciled parent blocks the cascade with IsReconciledError. The
// parent's other split-items are left intact even if the totals are now
// out of balance — the caller will reconcile on a later save.
func (s *Service) deletePairedSideOfMultiLine(paired *Transaction, parentSplit *Split) error {
	parent, err := s.txnRepo.GetByID(parentSplit.TransactionID)
	if err != nil {
		return err
	}
	if parent.IsReconciled() {
		return &IsReconciledError{ID: parent.ID.String()}
	}
	if err := s.splitRepo.Delete(parentSplit.ID); err != nil {
		return fmt.Errorf("failed to delete parent split-item: %w", err)
	}
	if err := s.txnRepo.Delete(paired.ID); err != nil {
		return fmt.Errorf("failed to delete paired transaction: %w", err)
	}
	return nil
}

// deleteTransferLinePairs deletes the paired counter-transaction of every
// transfer-typed split-line attached to the given parent transaction. A
// reconciled paired side blocks the cascade with IsReconciledError so the
// parent and its other splits remain intact.
func (s *Service) deleteTransferLinePairs(parentID types.ID) error {
	splits, err := s.splitRepo.ListByTransaction(parentID)
	if err != nil {
		return fmt.Errorf("failed to list splits for delete cascade: %w", err)
	}
	for _, split := range splits {
		if !split.TransferAccountID.Valid || !split.TransferID.Valid {
			continue
		}
		if err := s.deletePairedCounterTransaction(split.TransferID.ID); err != nil {
			return err
		}
	}
	return nil
}

// deletePairedCounterTransaction removes the single-line counter-transaction
// linked to a transfer-line's transfer_id. The counterpart may live on
// the regular transactions table (bank target) or on the
// investment_transactions table (investment target) — both are checked.
// Returns nil if no paired side exists; returns IsReconciledError if the
// paired side is reconciled.
func (s *Service) deletePairedCounterTransaction(transferID types.ID) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		if err := s.txnRepo.Delete(paired.ID); err != nil {
			return fmt.Errorf("failed to delete paired transfer transaction: %w", err)
		}
		return nil
	}
	if s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindTransferCashCounterpart(transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	if err := s.investmentCounterpart.DeleteTransferCashCounterpart(rowID); err != nil {
		return fmt.Errorf("failed to delete investment-side paired transfer transaction: %w", err)
	}
	return nil
}

// List returns all transactions ordered by date descending.
func (s *Service) List() ([]*Transaction, error) {
	return s.txnRepo.List()
}

// ListByAccount returns all transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.txnRepo.ListByAccount(accountID)
}

// ListByDateRange returns all transactions within a date range.
func (s *Service) ListByDateRange(startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByDateRange(startDate, endDate)
}

// ListByAccountAndDateRange returns transactions for an account within a date range.
func (s *Service) ListByAccountAndDateRange(accountID types.ID, startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByAccountAndDateRange(accountID, startDate, endDate)
}

// Search finds transactions matching the given criteria.
// Multiple criteria are combined with AND logic.
func (s *Service) Search(criteria SearchCriteria) ([]*Transaction, error) {
	return s.txnRepo.Search(criteria)
}

// SearchByPayee finds transactions by partial payee name match (case-insensitive).
func (s *Service) SearchByPayee(payeeName string) ([]*Transaction, error) {
	return s.txnRepo.SearchByPayee(payeeName)
}

// SearchByMemo finds transactions by partial memo match (case-insensitive).
func (s *Service) SearchByMemo(memo string) ([]*Transaction, error) {
	return s.txnRepo.SearchByMemo(memo)
}

// SearchByCategory finds transactions by partial category name match (case-insensitive).
func (s *Service) SearchByCategory(categoryName string) ([]*Transaction, error) {
	return s.txnRepo.SearchByCategory(categoryName)
}

// =============================================================================
// Split Transaction Operations
// =============================================================================

// CreateWithSplits creates a transaction along with its splits. The signed
// sum of split amounts must equal the transaction amount. For each
// transfer-typed split-line (TransferAccountID set), the service mints a
// fresh TransferID, stores it on the split-item, and creates a paired
// single-line transaction in the target account with the inverted amount
// and matching transfer_id.
func (s *Service) CreateWithSplits(transaction *Transaction, splits []*Split) error {
	if err := s.validateTransaction(transaction); err != nil {
		return err
	}

	if err := s.validateSplits(transaction, splits); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(transaction.AccountID); err != nil {
		return err
	}

	if transaction.HasCategory() && len(splits) > 0 {
		return &HasSplitsError{ID: transaction.ID.String()}
	}

	// Mint transfer_ids for transfer-lines so the split row and its paired
	// counter-transaction share the link from the first write.
	for _, split := range splits {
		if split.TransferAccountID.Valid && !split.TransferID.Valid {
			split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
		}
	}

	if err := s.txnRepo.Create(transaction); err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Track paired-counter-transactions so we can roll them back on a later
	// failure. Investment-side counterparts are tracked separately so the
	// rollback routes through the right repository.
	createdPairs := make([]transferLinePair, 0, len(splits))

	for _, split := range splits {
		split.TransactionID = transaction.ID
		if err := s.splitRepo.Create(split); err != nil {
			s.rollbackCreateWithSplits(transaction.ID, createdPairs)
			return fmt.Errorf("failed to create split: %w", err)
		}

		if !split.TransferAccountID.Valid {
			continue
		}

		pair, err := s.createTransferLineCounterpart(transaction.AccountID, transaction.Date, split)
		if err != nil {
			s.rollbackCreateWithSplits(transaction.ID, createdPairs)
			return err
		}
		createdPairs = append(createdPairs, pair)
	}

	return nil
}

// transferLinePair identifies a counterpart row created for a transfer-
// line split. isInvestment routes cleanup to the right repository.
type transferLinePair struct {
	rowID        types.ID
	isInvestment bool
}

// createTransferLineCounterpart mints the paired counter-transaction for
// a transfer-line split. If the target account is investment-type, the
// row is created on the investment_transactions table via the configured
// InvestmentCashCounterpartAdapter; otherwise a regular transaction is
// created on the transactions table.
//
// The split must already carry a valid TransferID (CreateWithSplits and
// moveTransferLine mint it before calling here).
func (s *Service) createTransferLineCounterpart(
	parentAcctID types.ID,
	parentDate types.Date,
	split *Split,
) (transferLinePair, error) {
	targetAcctID := split.TransferAccountID.ID
	counterAmount := split.Amount.Neg()
	transferID := split.TransferID.ID

	// Guard the target account: it must be open, and an investment target
	// requires the counterpart adapter. Guards CreateWithSplits,
	// moveTransferLine (which re-targets a split), and ReplaceSplits.
	isInv, err := s.ensureTransferTargetRoutable(targetAcctID)
	if err != nil {
		return transferLinePair{}, err
	}

	if isInv {
		rowID, err := s.investmentCounterpart.CreateTransferCashCounterpart(
			targetAcctID, parentAcctID, parentDate, counterAmount, "", transferID,
		)
		if err != nil {
			return transferLinePair{}, fmt.Errorf("failed to create investment-side paired transfer transaction: %w", err)
		}
		return transferLinePair{rowID: rowID, isInvestment: true}, nil
	}

	paired := NewTransaction(targetAcctID, parentDate, counterAmount)
	paired.SetTransfer(transferID, parentAcctID)
	if err := s.txnRepo.Create(paired); err != nil {
		return transferLinePair{}, fmt.Errorf("failed to create paired transfer transaction: %w", err)
	}
	return transferLinePair{rowID: paired.ID, isInvestment: false}, nil
}

// targetIsInvestment reports whether the given account is an investment-
// type account (TypeInvestment or TypeHSA). Returns false (no error) if
// accountRepo is not wired — only test fixtures hit that path.
func (s *Service) targetIsInvestment(acctID types.ID) (bool, error) {
	if s.accountRepo == nil {
		return false, nil
	}
	acct, err := s.accountRepo.GetByID(acctID)
	if err != nil {
		return false, fmt.Errorf("failed to load target account: %w", err)
	}
	return acct.Type.IsInvestmentType(), nil
}

// ensureTransferTargetRoutable verifies a transfer-line's target account can
// receive a paired counter-transaction: it must be open, and an investment
// target requires the investment-counterpart adapter to be wired. It returns
// whether the target is an investment account so the caller can route the
// counterpart to the right table without re-loading it. Reused by
// createTransferLineCounterpart and ReplaceSplits' pre-flight so a rewrite of
// the split rows can't strand a would-be counterpart.
func (s *Service) ensureTransferTargetRoutable(targetAcctID types.ID) (bool, error) {
	if err := s.ensureAccountOpen(targetAcctID); err != nil {
		return false, err
	}
	isInv, err := s.targetIsInvestment(targetAcctID)
	if err != nil {
		return false, err
	}
	if isInv && s.investmentCounterpart == nil {
		return false, fmt.Errorf(
			"transfer-line split targets investment account %s but no investment-counterpart adapter is wired on transaction.Service",
			targetAcctID.String(),
		)
	}
	return isInv, nil
}

// rollbackCreateWithSplits best-effort removes paired counter-transactions
// and the parent transaction (which cascades its splits) after a partial
// CreateWithSplits failure. Investment-side counterparts are routed
// through the adapter so they don't leak.
func (s *Service) rollbackCreateWithSplits(parentID types.ID, pairs []transferLinePair) {
	s.rollbackTransferLinePairs(pairs)
	_, _ = s.splitRepo.DeleteByTransaction(parentID)
	_ = s.txnRepo.Delete(parentID)
}

// rollbackTransferLinePairs best-effort removes the given counter-transactions,
// routing each to the repository that created it. Used to unwind counterparts
// minted partway through CreateWithSplits or ReplaceSplits before an error.
func (s *Service) rollbackTransferLinePairs(pairs []transferLinePair) {
	for _, p := range pairs {
		if p.isInvestment {
			if s.investmentCounterpart != nil {
				_ = s.investmentCounterpart.DeleteTransferCashCounterpart(p.rowID)
			}
			continue
		}
		_ = s.txnRepo.Delete(p.rowID)
	}
}

// GetSplits returns all splits for a transaction.
func (s *Service) GetSplits(transactionID types.ID) ([]*Split, error) {
	return s.splitRepo.ListByTransaction(transactionID)
}

// AddSplit adds a new split to an existing transaction.
// After adding, the splits must still sum to the transaction amount.
// Void and reconciled transactions cannot have splits added.
func (s *Service) AddSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	// Get the transaction
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	// Transfers cannot have splits
	if txn.IsTransfer() {
		return &TransferCannotHaveSplitsError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Create the split
	if err := s.splitRepo.Create(split); err != nil {
		return err
	}

	// Validate total (warning only - the user may be adding more splits)
	valid, err := s.splitRepo.ValidateSplitsAgainstTransaction(split.TransactionID)
	if err != nil {
		return fmt.Errorf("failed to validate splits: %w", err)
	}
	if !valid {
		// Return a specific error so caller knows splits don't match
		total, _ := s.splitRepo.GetTotalByTransaction(split.TransactionID)
		return &SplitTotalMismatchError{
			TransactionID:     split.TransactionID.String(),
			TransactionAmount: txn.Amount,
			SplitTotal:        total,
		}
	}

	return nil
}

// UpdateSplit updates an existing split.
// Splits on void or reconciled transactions cannot be updated.
//
// For transfer-line splits, the paired single-line counter-transaction in
// the target account is kept in sync:
//   - An amount edit mirrors the new (negated) amount onto the paired side.
//   - A target-account edit deletes the old paired side and creates a new
//     one in the new target with a fresh transfer_id.
//
// Self-transfers (transfer_account_id == parent.account_id) are rejected.
func (s *Service) UpdateSplit(split *Split) error {
	if err := s.validateSplit(split); err != nil {
		return err
	}

	existing, err := s.splitRepo.GetByID(split.ID)
	if err != nil {
		return err
	}

	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	if split.TransferAccountID.Valid && split.TransferAccountID.ID == txn.AccountID {
		errors := types.ValidationErrors{}
		errors.Add("transfer_account_id",
			(&SelfTransferError{AccountID: txn.AccountID.String()}).Error())
		return &types.ServiceValidationError{Errors: errors}
	}

	wasTransfer := existing.TransferAccountID.Valid
	isTransfer := split.TransferAccountID.Valid

	if wasTransfer && isTransfer {
		targetMoved := existing.TransferAccountID.ID != split.TransferAccountID.ID
		if targetMoved {
			return s.moveTransferLine(txn, existing, split)
		}
		// Preserve the linkage so callers may omit transfer_id on edits.
		if !split.TransferID.Valid && existing.TransferID.Valid {
			split.TransferID = existing.TransferID
		}
		if err := s.splitRepo.Update(split); err != nil {
			return err
		}
		if existing.TransferID.Valid && !existing.Amount.Equal(split.Amount) {
			return s.updatePairedAmount(existing.TransferID.ID, split.Amount.Neg())
		}
		return nil
	}

	return s.splitRepo.Update(split)
}

// moveTransferLine handles the target-account-change cascade: delete the old
// paired counter-transaction, mint a fresh transfer_id on the split-line,
// persist the split, and create a new paired counterpart in the new target.
// The new counterpart is routed to the investment table when the new
// target is an investment account.
func (s *Service) moveTransferLine(parent *Transaction, existing, split *Split) error {
	if existing.TransferID.Valid {
		if err := s.deletePairedCounterTransaction(existing.TransferID.ID); err != nil {
			return err
		}
	}

	split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
	if err := s.splitRepo.Update(split); err != nil {
		return err
	}

	if _, err := s.createTransferLineCounterpart(parent.AccountID, parent.Date, split); err != nil {
		return err
	}
	return nil
}

// findPairedByTransferID returns the paired single-line counter-transaction
// for a transfer-line's transfer_id, or nil if none exists. The parent
// (multi-line) transaction is not marked as a transfer, so only the paired
// counterpart lives on the transactions table with that transfer_id.
func (s *Service) findPairedByTransferID(transferID types.ID) (*Transaction, error) {
	matches, err := s.txnRepo.ListByTransferID(transferID)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return matches[0], nil
}

// updatePairedAmount sets the paired counter-transaction's amount to mirror a
// transfer-line amount edit. A reconciled paired side blocks the cascade.
// Handles both regular-side and investment-side counterparts.
func (s *Service) updatePairedAmount(transferID types.ID, newAmount types.Money) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		paired.Amount = newAmount
		return s.txnRepo.Update(paired)
	}
	if s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindTransferCashCounterpart(transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return s.investmentCounterpart.UpdateTransferCashCounterpartAmount(rowID, newAmount)
}

// DeleteSplit removes a split from a transaction.
// Splits on void or reconciled transactions cannot be deleted.
//
// For transfer-line splits, the paired single-line counter-transaction in
// the target account is also deleted. A reconciled paired side blocks the
// cascade with IsReconciledError. The parent transaction is left intact —
// its remaining splits may now leave the totals out of balance, which is
// the caller's responsibility to resolve on a subsequent save.
func (s *Service) DeleteSplit(splitID types.ID) error {
	// Get the split to find its parent transaction
	split, err := s.splitRepo.GetByID(splitID)
	if err != nil {
		return err
	}

	// Check the parent transaction status
	txn, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	if split.TransferAccountID.Valid && split.TransferID.Valid {
		if err := s.deletePairedCounterTransaction(split.TransferID.ID); err != nil {
			return err
		}
	}

	return s.splitRepo.Delete(splitID)
}

// ReplaceSplits replaces all splits for a transaction with new ones.
// The new splits must sum to the transaction amount.
// Void and reconciled transactions cannot have splits replaced.
//
// Transfer-line splits carry a paired single-line counter-transaction in
// their target account (regular or investment table). ReplaceSplits keeps
// those counterparts consistent by diffing the new transfer lines against
// the current ones rather than blindly dropping and recreating every row:
//
//   - a retained transfer line (matched by transfer_id, else by target
//     account) keeps its counterpart; an amount change mirrors onto it;
//   - a removed transfer line's counterpart is deleted;
//   - an added transfer line mints a transfer_id and creates a counterpart.
//
// Callers may omit transfer_id on retained lines (the TUI split dialog does),
// so matching falls back to the target account. A reconciled counterpart that
// would be deleted or amount-changed blocks the whole operation before any
// mutation. Without this, a rewrite of a split set containing a transfer line
// trips the transfer_id/transfer_account_id pairing CHECK mid-flight and
// orphans the counterpart.
func (s *Service) ReplaceSplits(transactionID types.ID, splits []*Split) error {
	// Get the transaction
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return err
	}

	// Prevent modifying void transactions
	if txn.IsVoid() {
		return &IsVoidError{ID: txn.ID.String()}
	}

	// Prevent modifying reconciled transactions
	if txn.IsReconciled() {
		return &IsReconciledError{ID: txn.ID.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// Validate splits sum to transaction amount
	if err := s.validateSplits(txn, splits); err != nil {
		return err
	}

	// Diff the new transfer lines against the current split set so retained
	// counterparts survive the rewrite. Assigns transfer_ids onto the new
	// splits in place (retained → existing id, added → fresh id).
	oldSplits, err := s.splitRepo.ListByTransaction(transactionID)
	if err != nil {
		return fmt.Errorf("failed to list existing splits: %w", err)
	}
	plan := planSplitReplacement(oldSplits, splits)

	// Pre-flight every fallible counterpart operation before mutating anything,
	// so a reconciled or unroutable counterpart fails cleanly with no partial
	// write.
	if err := s.preflightSplitReplacement(plan); err != nil {
		return err
	}

	// Reconcile counterparts of removed and retained-changed transfer lines.
	for _, transferID := range plan.removedTransferIDs {
		if err := s.deletePairedCounterTransaction(transferID); err != nil {
			return err
		}
	}
	for _, change := range plan.retainedAmountChanged {
		if err := s.updatePairedAmount(change.transferID, change.newAmount.Neg()); err != nil {
			return err
		}
	}

	// Rebuild the split rows. Retained transfer lines already carry their
	// original transfer_id (linking them to the still-live counterpart), so a
	// plain drop-and-recreate is safe.
	if _, err := s.splitRepo.DeleteByTransaction(transactionID); err != nil {
		return fmt.Errorf("failed to delete existing splits: %w", err)
	}
	for _, split := range splits {
		split.TransactionID = transactionID
		if err := s.splitRepo.Create(split); err != nil {
			return fmt.Errorf("failed to create split: %w", err)
		}
	}

	// Mint counterparts for the added transfer lines.
	createdPairs := make([]transferLinePair, 0, len(plan.addedSplits))
	for _, split := range plan.addedSplits {
		pair, err := s.createTransferLineCounterpart(txn.AccountID, txn.Date, split)
		if err != nil {
			s.rollbackTransferLinePairs(createdPairs)
			return err
		}
		createdPairs = append(createdPairs, pair)
	}

	return nil
}

// retainedTransferChange records a transfer line kept across a ReplaceSplits
// whose amount changed, so its counterpart amount can be mirrored.
type retainedTransferChange struct {
	transferID types.ID
	newAmount  types.Money
}

// splitReplacementPlan captures the transfer-line diff computed by
// planSplitReplacement for ReplaceSplits.
type splitReplacementPlan struct {
	// Counterparts to delete (old transfer lines with no match in the new set).
	removedTransferIDs []types.ID
	// Retained transfer lines whose amount changed (mirror onto counterpart).
	retainedAmountChanged []retainedTransferChange
	// New transfer lines with no match in the old set. Each already has a
	// transfer_id assigned; a counterpart must be minted for it.
	addedSplits []*Split
}

// planSplitReplacement diffs the desired transfer lines against the current
// ones and assigns transfer_ids onto the new splits in place: a retained line
// (matched first by transfer_id, then by target account) inherits its match's
// transfer_id; an added line keeps a caller-supplied transfer_id or is minted
// a fresh one. Categorized lines are ignored (they carry no counterpart and
// are recreated wholesale by ReplaceSplits).
func planSplitReplacement(oldSplits, newSplits []*Split) splitReplacementPlan {
	type oldTransfer struct {
		split    *Split
		consumed bool
	}
	olds := make([]*oldTransfer, 0, len(oldSplits))
	for _, os := range oldSplits {
		if os.TransferAccountID.Valid {
			olds = append(olds, &oldTransfer{split: os})
		}
	}

	matchByTransferID := func(id types.NullableID) *oldTransfer {
		if !id.Valid {
			return nil
		}
		for _, o := range olds {
			if !o.consumed && o.split.TransferID.Valid && o.split.TransferID.ID == id.ID {
				return o
			}
		}
		return nil
	}
	matchByTarget := func(acctID types.ID) *oldTransfer {
		for _, o := range olds {
			if !o.consumed && o.split.TransferAccountID.ID == acctID {
				return o
			}
		}
		return nil
	}

	var plan splitReplacementPlan
	for _, ns := range newSplits {
		if !ns.TransferAccountID.Valid {
			continue
		}
		match := matchByTransferID(ns.TransferID)
		if match == nil {
			match = matchByTarget(ns.TransferAccountID.ID)
		}
		if match != nil {
			match.consumed = true
			// Retained: adopt the existing counterpart's transfer_id.
			ns.TransferID = match.split.TransferID
			if !ns.Amount.Equal(match.split.Amount) {
				plan.retainedAmountChanged = append(plan.retainedAmountChanged,
					retainedTransferChange{transferID: match.split.TransferID.ID, newAmount: ns.Amount})
			}
			continue
		}
		// Added: mint a transfer_id unless the caller replayed one (e.g. the
		// void-undo restore path carries the captured lines' original ids).
		if !ns.TransferID.Valid {
			ns.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
		}
		plan.addedSplits = append(plan.addedSplits, ns)
	}

	for _, o := range olds {
		if !o.consumed && o.split.TransferID.Valid {
			plan.removedTransferIDs = append(plan.removedTransferIDs, o.split.TransferID.ID)
		}
	}
	return plan
}

// preflightSplitReplacement verifies every counterpart mutation the plan
// implies can succeed, so ReplaceSplits fails before it deletes any split row.
// Counterparts that will be deleted or amount-changed must not be reconciled;
// added transfer lines must target a routable account.
func (s *Service) preflightSplitReplacement(plan splitReplacementPlan) error {
	for _, transferID := range plan.removedTransferIDs {
		if err := s.ensureCounterpartNotReconciled(transferID); err != nil {
			return err
		}
	}
	for _, change := range plan.retainedAmountChanged {
		if err := s.ensureCounterpartNotReconciled(change.transferID); err != nil {
			return err
		}
	}
	for _, split := range plan.addedSplits {
		if _, err := s.ensureTransferTargetRoutable(split.TransferAccountID.ID); err != nil {
			return err
		}
	}
	return nil
}

// ensureCounterpartNotReconciled returns IsReconciledError if the counter-
// transaction linked to transferID (regular or investment table) is
// reconciled, and nil if it is unreconciled or absent.
func (s *Service) ensureCounterpartNotReconciled(transferID types.ID) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		return nil
	}
	if s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindTransferCashCounterpart(transferID)
	if err != nil {
		return err
	}
	if found && reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return nil
}

// ValidateSplitTotals checks if the splits for a transaction sum to the transaction amount.
func (s *Service) ValidateSplitTotals(transactionID types.ID) (bool, error) {
	return s.splitRepo.ValidateSplitsAgainstTransaction(transactionID)
}

// =============================================================================
// Transfer Operations
// =============================================================================

// rejectInvestmentAccount returns a NotRegularAccountError if the given
// account is investment-type. Used to keep linked cash transfers involving
// an investment account out of the regular transaction.Transaction ledger.
func (s *Service) rejectInvestmentAccount(accountID types.ID) error {
	if s.accountRepo == nil {
		return nil
	}
	acct, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return err
	}
	if acct.Type.IsInvestmentType() {
		return &NotRegularAccountError{AccountID: accountID.String(), Type: acct.Type.String()}
	}
	return nil
}

// CreateTransfer creates a linked transfer between two accounts.
// This creates two transactions: one debit in the from account, one credit in the to account.
//
// Investment-type accounts are rejected on either leg: linked cash transfers
// involving an investment account must go through investment.Service so the
// investment-side row is created as an investment.Transaction.
func (s *Service) CreateTransfer(fromAccountID, toAccountID types.ID, date types.Date, amount types.Money) (*TransferPair, error) {
	// Validate amount is positive
	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	if err := s.rejectInvestmentAccount(fromAccountID); err != nil {
		return nil, err
	}
	if err := s.rejectInvestmentAccount(toAccountID); err != nil {
		return nil, err
	}

	if err := s.guardTransferDate(fromAccountID, toAccountID, date); err != nil {
		return nil, err
	}

	// Create the transfer pair
	pair := NewTransferPair(fromAccountID, toAccountID, date, amount)

	// Validate the pair
	if errors := pair.Validate(); errors.HasErrors() {
		return nil, &types.ServiceValidationError{Errors: errors}
	}

	// Create both transactions
	if err := s.transferRepo.Create(pair); err != nil {
		return nil, err
	}

	return pair, nil
}

// GetTransferPair retrieves both sides of a transfer by the transfer ID.
func (s *Service) GetTransferPair(transferID types.ID) (*TransferPair, error) {
	return s.transferRepo.GetByTransferID(transferID)
}

// GetTransferCounterpart retrieves the other transaction in a transfer pair.
func (s *Service) GetTransferCounterpart(transactionID types.ID) (*Transaction, error) {
	return s.transferRepo.GetOtherSide(transactionID)
}

// UpdateTransfer updates both sides of a transfer.
// Only amount, date, memo, and status can be updated.
// Reconciled transfers cannot be edited.
//
// Investment-type accounts are rejected on either leg: linked cash transfers
// involving an investment account must go through investment.Service so the
// investment-side row is updated as an investment.Transaction.
func (s *Service) UpdateTransfer(transferID types.ID, date types.Date, amount types.Money, memo string, status Status) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	if err := s.rejectInvestmentAccount(pair.FromTransaction.AccountID); err != nil {
		return err
	}
	if err := s.rejectInvestmentAccount(pair.ToTransaction.AccountID); err != nil {
		return err
	}

	if err := s.guardTransferDate(pair.FromTransaction.AccountID, pair.ToTransaction.AccountID, date); err != nil {
		return err
	}

	// Prevent editing reconciled transfers
	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}

	// Prevent editing void transfers
	if pair.FromTransaction.IsVoid() {
		return &IsVoidError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsVoid() {
		return &IsVoidError{ID: pair.ToTransaction.ID.String()}
	}

	// Update common fields on both sides
	pair.FromTransaction.Date = date
	pair.ToTransaction.Date = date

	pair.FromTransaction.Amount = amount.Neg()
	pair.ToTransaction.Amount = amount.Abs()

	pair.FromTransaction.SetMemo(memo)
	pair.ToTransaction.SetMemo(memo)

	pair.FromTransaction.SetStatus(status)
	pair.ToTransaction.SetStatus(status)

	return s.transferRepo.Update(pair)
}

// UpdateTransferAmount updates the amount on both sides of a transfer.
// Reconciled transfers cannot be edited.
func (s *Service) UpdateTransferAmount(transferID types.ID, newAmount types.Money) error {
	if !newAmount.IsPositive() {
		return &InvalidTransferAmountError{Amount: newAmount}
	}

	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	return s.transferRepo.UpdateAmount(transferID, newAmount)
}

// UpdateTransferDate updates the date on both sides of a transfer.
// Reconciled transfers cannot be edited.
func (s *Service) UpdateTransferDate(transferID types.ID, newDate types.Date) error {
	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}
	if err := s.guardTransferDate(pair.FromTransaction.AccountID, pair.ToTransaction.AccountID, newDate); err != nil {
		return err
	}

	return s.transferRepo.UpdateDate(transferID, newDate)
}

// guardTransferDate rejects a transfer whose date precedes the opening date of
// either account it touches, or whose either leg is a closed account.
func (s *Service) guardTransferDate(fromAccountID, toAccountID types.ID, date types.Date) error {
	for _, id := range []types.ID{fromAccountID, toAccountID} {
		acct, err := s.accountRepo.GetByID(id)
		if err != nil {
			return fmt.Errorf("failed to load account for date validation: %w", err)
		}
		if acct.IsClosed() {
			return &account.AccountClosedError{ID: id.String()}
		}
		if err := acct.ValidateTransactionDate(date); err != nil {
			return err
		}
	}
	return nil
}

// ensureAccountOpen returns an AccountClosedError when the account is closed.
// A closed account is frozen: no new transactions, edits, deletes, or status
// toggles. It is nil-tolerant for test fixtures constructed without an
// accountRepo (matching targetIsInvestment / rejectInvestmentAccount); the
// production wiring always passes a real repository.
func (s *Service) ensureAccountOpen(id types.ID) error {
	if s.accountRepo == nil {
		return nil
	}
	acct, err := s.accountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("failed to load account for closed check: %w", err)
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: id.String()}
	}
	return nil
}

// UpdateTransferStatus updates the status on both sides of a transfer.
func (s *Service) UpdateTransferStatus(transferID types.ID, status Status) error {
	return s.transferRepo.UpdateStatus(transferID, status)
}

// DeleteTransfer removes both sides of a transfer.
// Reconciled transfers cannot be deleted.
func (s *Service) DeleteTransfer(transferID types.ID) error {
	if err := s.checkTransferEditable(transferID); err != nil {
		return err
	}

	return s.transferRepo.Delete(transferID)
}

// checkTransferEditable checks if a transfer can be edited/deleted.
// Returns an error if either side is reconciled or void.
func (s *Service) checkTransferEditable(transferID types.ID) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}
	if pair.FromTransaction.IsVoid() {
		return &IsVoidError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsVoid() {
		return &IsVoidError{ID: pair.ToTransaction.ID.String()}
	}

	// A transfer is frozen if either leg lives on a closed account.
	if err := s.ensureAccountOpen(pair.FromTransaction.AccountID); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(pair.ToTransaction.AccountID); err != nil {
		return err
	}

	return nil
}

// IsTransfer checks if a transaction is part of a transfer.
func (s *Service) IsTransfer(transactionID types.ID) (bool, error) {
	return s.transferRepo.IsTransfer(transactionID)
}

// =============================================================================
// Transaction Status Operations
// =============================================================================

// ClearTransaction marks a transaction as cleared.
// Void and reconciled transactions cannot be cleared directly.
func (s *Service) ClearTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	txn.Clear()
	return s.txnRepo.Update(txn)
}

// ReconcileTransaction marks a transaction as reconciled.
// Void transactions cannot be reconciled.
func (s *Service) ReconcileTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	txn.Reconcile()
	return s.txnRepo.Update(txn)
}

// MarkTransactionUncleared marks a transaction as uncleared.
// Void and reconciled transactions cannot be marked uncleared directly.
// Use UnReconcileTransaction to move from reconciled to cleared.
func (s *Service) MarkTransactionUncleared(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	txn.MarkUncleared()
	return s.txnRepo.Update(txn)
}

// UnReconcileTransaction moves a reconciled transaction back to cleared status.
// Only reconciled transactions can be un-reconciled.
func (s *Service) UnReconcileTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if !txn.IsReconciled() {
		return &NotReconciledError{ID: id.String()}
	}

	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	txn.Clear()
	return s.txnRepo.Update(txn)
}

// VoidTransaction voids a transaction by setting its amount to 0, memo to **VOID**,
// and status to void. For transfers, both sides are voided atomically.
// For split transactions, all splits are removed and any paired
// counter-transactions minted from transfer-line splits are cascade-
// deleted alongside the splits — matching the Delete cascade. Bank-side
// and investment-side counterparts are both handled; a reconciled
// counterpart blocks the void with IsReconciledError.
// Void and reconciled transactions cannot be voided.
func (s *Service) VoidTransaction(id types.ID) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	// Cannot void an already void transaction
	if txn.IsVoid() {
		return &IsVoidError{ID: id.String()}
	}

	// Cannot void a reconciled transaction
	if txn.IsReconciled() {
		return &IsReconciledError{ID: id.String()}
	}

	// A closed account is frozen — block voids.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}

	// If this is a transfer, void both sides
	if txn.IsTransfer() {
		return s.voidTransfer(txn.TransferID.ID)
	}

	// Cascade to paired counter-transactions of any transfer-typed split-
	// lines before deleting the splits — otherwise the counterparts (bank
	// or investment) are orphaned with a dangling transfer_id.
	if err := s.deleteTransferLinePairs(id); err != nil {
		return err
	}

	// Delete splits if any
	if _, err := s.splitRepo.DeleteByTransaction(id); err != nil {
		return fmt.Errorf("failed to delete splits for void: %w", err)
	}

	// Void the transaction
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	txn.Void()

	return s.txnRepo.Update(txn)
}

// voidTransfer voids both sides of a transfer atomically.
func (s *Service) voidTransfer(transferID types.ID) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	// Check if either side is reconciled
	if pair.FromTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.FromTransaction.ID.String()}
	}
	if pair.ToTransaction.IsReconciled() {
		return &IsReconciledError{ID: pair.ToTransaction.ID.String()}
	}

	// Block the void if either leg lives on a closed account.
	if err := s.ensureAccountOpen(pair.FromTransaction.AccountID); err != nil {
		return err
	}
	if err := s.ensureAccountOpen(pair.ToTransaction.AccountID); err != nil {
		return err
	}

	// Void both sides
	pair.FromTransaction.Amount = types.ZeroMoney
	pair.FromTransaction.SetMemo("**VOID**")
	pair.FromTransaction.Void()

	pair.ToTransaction.Amount = types.ZeroMoney
	pair.ToTransaction.SetMemo("**VOID**")
	pair.ToTransaction.Void()

	return s.transferRepo.Update(pair)
}

// RestoreVoidedTransaction restores a voided transaction to its original state.
// This is used by the undo system to reverse a void operation.
// Only void transactions can be restored.
func (s *Service) RestoreVoidedTransaction(id types.ID, amount types.Money, memo types.NullableString, status Status) error {
	txn, err := s.txnRepo.GetByID(id)
	if err != nil {
		return err
	}

	if !txn.IsVoid() {
		return fmt.Errorf("transaction %s is not void; cannot restore", id.String())
	}

	txn.Amount = amount
	txn.Memo = memo
	txn.SetStatus(status)

	return s.txnRepo.Update(txn)
}

// RestoreVoidedTransfer restores both sides of a voided transfer to their original state.
// This is used by the undo system to reverse a void transfer operation.
func (s *Service) RestoreVoidedTransfer(transferID types.ID, fromAmount types.Money, fromMemo types.NullableString, fromStatus Status, toAmount types.Money, toMemo types.NullableString, toStatus Status) error {
	pair, err := s.transferRepo.GetByTransferID(transferID)
	if err != nil {
		return err
	}

	if !pair.FromTransaction.IsVoid() {
		return fmt.Errorf("transfer from-side %s is not void; cannot restore", pair.FromTransaction.ID.String())
	}
	if !pair.ToTransaction.IsVoid() {
		return fmt.Errorf("transfer to-side %s is not void; cannot restore", pair.ToTransaction.ID.String())
	}

	pair.FromTransaction.Amount = fromAmount
	pair.FromTransaction.Memo = fromMemo
	pair.FromTransaction.SetStatus(fromStatus)

	pair.ToTransaction.Amount = toAmount
	pair.ToTransaction.Memo = toMemo
	pair.ToTransaction.SetStatus(toStatus)

	return s.transferRepo.Update(pair)
}

// =============================================================================
// Balance Impact Operations
// =============================================================================

// BalanceImpact represents the impact of a transaction on account balances.
type BalanceImpact struct {
	AccountID      types.ID
	Amount         types.Money
	IsTransferFrom bool
	IsTransferTo   bool
}

// GetBalanceImpact calculates the balance impact of a transaction.
// For transfers, returns impacts for both accounts.
func (s *Service) GetBalanceImpact(transactionID types.ID) ([]BalanceImpact, error) {
	txn, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	impacts := []BalanceImpact{
		{
			AccountID:      txn.AccountID,
			Amount:         txn.Amount,
			IsTransferFrom: txn.IsTransfer() && txn.Amount.IsNegative(),
			IsTransferTo:   txn.IsTransfer() && txn.Amount.IsPositive(),
		},
	}

	// For transfers, include the other side
	if txn.IsTransfer() {
		other, err := s.transferRepo.GetOtherSide(transactionID)
		if err != nil {
			return nil, err
		}
		impacts = append(impacts, BalanceImpact{
			AccountID:      other.AccountID,
			Amount:         other.Amount,
			IsTransferFrom: other.Amount.IsNegative(),
			IsTransferTo:   other.Amount.IsPositive(),
		})
	}

	return impacts, nil
}

// =============================================================================
// Duplicate Operations
// =============================================================================

// Duplicate creates a copy of a transaction with today's date and uncleared status.
func (s *Service) Duplicate(transactionID types.ID) (*Transaction, error) {
	original, err := s.txnRepo.GetByID(transactionID)
	if err != nil {
		return nil, err
	}

	// Cannot duplicate transfers (they need special handling)
	if original.IsTransfer() {
		return nil, &CannotDuplicateTransferError{ID: transactionID.String()}
	}

	// Create a new transaction with same properties
	duplicate := NewTransaction(original.AccountID, types.Today(), original.Amount)
	duplicate.PayeeID = original.PayeeID
	duplicate.CategoryID = original.CategoryID
	duplicate.Memo = original.Memo
	duplicate.CheckNumber = original.CheckNumber
	// Status is always uncleared for duplicates (set by NewTransaction)

	if err := s.txnRepo.Create(duplicate); err != nil {
		return nil, err
	}

	// Duplicate splits if any
	splits, err := s.splitRepo.ListByTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get splits: %w", err)
	}

	for _, split := range splits {
		newSplit := NewSplit(duplicate.ID, split.CategoryID, split.Amount)
		newSplit.Memo = split.Memo
		if err := s.splitRepo.Create(newSplit); err != nil {
			// Best effort cleanup
			_ = s.txnRepo.Delete(duplicate.ID)
			return nil, fmt.Errorf("failed to duplicate split: %w", err)
		}
	}

	return duplicate, nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validateTransaction validates a transaction and returns any validation errors.
func (s *Service) validateTransaction(transaction *Transaction) error {
	errors := transaction.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	// Reject activity dated before the account opened (catches mistyped years
	// such as "0018" for "2018"). Voided rows retain their original date and
	// are not re-validated here.
	if !transaction.IsVoid() {
		acct, err := s.accountRepo.GetByID(transaction.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load account for date validation: %w", err)
		}
		if err := acct.ValidateTransactionDate(transaction.Date); err != nil {
			return err
		}
	}
	return nil
}

// validateSplit validates a split and returns any validation errors.
func (s *Service) validateSplit(split *Split) error {
	errors := split.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateSplits validates that splits' signed sum equals the transaction
// amount. Mixed-sign lines are allowed: each line carries its own sign
// independent of the parent. Transfer-typed lines additionally must not
// target the parent's own account.
// See SplitCollection.ValidateAgainstTransaction.
func (s *Service) validateSplits(transaction *Transaction, splits []*Split) error {
	if len(splits) == 0 {
		return nil
	}

	for _, split := range splits {
		if err := s.validateSplit(split); err != nil {
			return err
		}
		if split.TransferAccountID.Valid && split.TransferAccountID.ID == transaction.AccountID {
			errors := types.ValidationErrors{}
			errors.Add("transfer_account_id",
				(&SelfTransferError{AccountID: transaction.AccountID.String()}).Error())
			return &types.ServiceValidationError{Errors: errors}
		}
	}

	collection := SplitCollection(splits)
	errors := collection.ValidateAgainstTransaction(transaction.Amount)
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}

	return nil
}

// applyPayeeDefaultCategory sets the transaction category from the payee's default.
func (s *Service) applyPayeeDefaultCategory(transaction *Transaction) error {
	if s.payeeRepo == nil || !transaction.HasPayee() {
		return nil
	}

	p, err := s.payeeRepo.GetByID(transaction.PayeeID.ID)
	if err != nil {
		// Payee not found is not an error here - just skip
		if _, ok := err.(*dberrors.NotFoundError); ok {
			return nil
		}
		return fmt.Errorf("failed to get payee: %w", err)
	}

	if p.HasDefaultCategory() {
		transaction.SetCategory(p.DefaultCategoryID.ID)
	}

	return nil
}
