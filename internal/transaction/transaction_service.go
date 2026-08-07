package transaction

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// InvestmentCounterpartPort is how transaction.Service mints, finds, deletes and
// amends the investment_transactions row that is the counterpart of a transfer
// LINE inside a multi-line split (e.g. a paycheck → 401k contribution line, an
// auto-deposit to a brokerage).
//
// Whole-transaction transfers do NOT come through here — internal/transfer owns
// those and writes both ledgers directly. This port exists only because the
// split lifecycle stays in this package, and a split's counterpart must commit
// in the same transaction as the split row itself.
//
// It replaces InvestmentCashCounterpartAdapter. Two changes:
//
//  1. The transaction is an explicit db.Queryer PARAMETER, not a bound-copy
//     return. The old CounterpartInTx had to name
//     transaction.InvestmentCashCounterpartAdapter as its return type, and that
//     single reference is what forced internal/investment to import
//     internal/transaction. Passing the Queryer per call removes the last
//     cross-package type reference — and with it the CounterpartInTx naming
//     hack, which existed only because investment.Service already had an
//     InTx(tx) *Service and Go forbids two methods sharing a name.
//  2. It is injected at construction, not set afterwards. Once
//     investment.NewService no longer takes a *transaction.Repository, the
//     construction order inverts freely: build investmentSvc, then txnSvc with
//     the port. SetInvestmentCounterpart is gone.
//
// db.Queryer remains the only vocabulary shared across the boundary, which is
// exactly why it lives in internal/db.
//
// A nil port means transfer LINES targeting an investment account are refused
// (ensureTransferTargetRoutable) rather than written as a malformed regular row.
type InvestmentCounterpartPort interface {
	// CreateCounterpart mints the investment-side row on q. amount carries the
	// sign in the destination frame (positive = cash arriving, negative = cash
	// leaving); the caller provides the shared transferID. Returns the new row's
	// ID.
	CreateCounterpart(
		q db.Queryer,
		invAcctID, otherAcctID types.ID,
		date types.Date,
		amount types.Money,
		memo string,
		transferID types.ID,
	) (types.ID, error)

	// FindCounterpart returns the investment row linked to transferID.
	// found=false means no investment-side row exists for it (the counterpart
	// may be on the regular table, or none was ever minted).
	FindCounterpart(q db.Queryer, transferID types.ID) (rowID types.ID, reconciled bool, found bool, err error)

	// DeleteCounterpart removes the investment row by ID.
	DeleteCounterpart(q db.Queryer, rowID types.ID) error

	// UpdateCounterpartAmount mirrors a transfer-line amount edit onto the
	// investment row. newAmount is in the destination frame.
	UpdateCounterpartAmount(q db.Queryer, rowID types.ID, newAmount types.Money) error
}

// Service provides business logic for transaction operations.
type Service struct {
	txnRepo               *Repository
	splitRepo             *SplitRepository
	payeeRepo             *payee.Repository
	accountRepo           *account.Repository
	investmentCounterpart InvestmentCounterpartPort
	db                    *db.DB
	tx                    db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
func NewService(
	txnRepo *Repository,
	splitRepo *SplitRepository,
	payeeRepo *payee.Repository,
	accountRepo *account.Repository,
	investmentCounterpart InvestmentCounterpartPort,
	database *db.DB,
) *Service {
	return &Service{
		txnRepo:               txnRepo,
		splitRepo:             splitRepo,
		payeeRepo:             payeeRepo,
		accountRepo:           accountRepo,
		investmentCounterpart: investmentCounterpart,
		db:                    database,
	}
}

// InTx returns a copy of the service bound to tx, with every repository field
// and the investment-counterpart adapter rebound to the same transaction so all
// writes join one atomic unit. The original service is unchanged and remains
// safe for non-transactional use.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.txnRepo = s.txnRepo.WithTx(tx)
	c.splitRepo = s.splitRepo.WithTx(tx)
	if s.payeeRepo != nil {
		c.payeeRepo = s.payeeRepo.WithTx(tx)
	}
	if s.accountRepo != nil {
		c.accountRepo = s.accountRepo.WithTx(tx)
	}
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not
// query the pool directly — a pool read would miss the transaction's own
// uncommitted writes, and a pool write would escape its atomicity.
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. This is what makes service methods composable
// without savepoints: an outer flow binds once, inner calls join. A bound
// service must never reach db.WithTx (nesting would deadlock the mutex).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
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

	// A whole-transaction transfer leg is not editable through here: this method
	// writes ONE row, and a transfer's counterpart may live in
	// investment_transactions. transfer.Service owns the pair — including
	// status-only changes, which go through transfer.SetLegStatus (that is what
	// the register's cleared toggle uses).
	//
	// The probe distinguishes the two kinds of transfer-linked row: a split
	// line's counterpart is owned by the split lifecycle and still edits here,
	// via the reverse cascade below.
	if existing.IsTransfer() && existing.TransferID.Valid {
		parentSplit, err := s.splitRepo.GetByTransferID(existing.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit == nil {
			return &IsTransferError{ID: transaction.ID.String()}
		}
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

	// A transfer-linked row is one of two things, and only one of them belongs
	// to this service.
	if txn.IsTransfer() {
		// (a) The counterpart of a transfer LINE inside a multi-line split
		// (e.g. a paycheck's 401k contribution line). The split lifecycle owns
		// those, so the reverse cascade stays here: remove the parent's
		// transfer-line split-item, then the paired row.
		parentSplit, err := s.splitRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit != nil {
			return s.runInTx(func(b *Service) error {
				return b.deletePairedSideOfMultiLine(txn, parentSplit)
			})
		}

		// (b) A leg of a whole-transaction transfer, owned by
		// transfer.Service. Deleting one leg here would leave the counterpart
		// orphaned whenever it lives in investment_transactions — which the old
		// two-leg branch could not even see, since it read the pair through a
		// repository that assumed both legs were on `transactions`.
		return &IsTransferError{ID: id.String()}
	}

	// Cascade to paired counter-transactions of any transfer-typed split-
	// lines before removing the parent, then drop the splits and the parent —
	// all in one transaction so a mid-cascade failure leaves the whole
	// transaction (parent, splits, counterparts) intact. The parent itself is
	// not marked as a transfer (only the split-item carries the linkage), so
	// the legacy transfer branch above does not run.
	return s.runInTx(func(b *Service) error {
		if err := b.deleteTransferLinePairs(id); err != nil {
			return err
		}
		if _, err := b.splitRepo.DeleteByTransaction(id); err != nil {
			return fmt.Errorf("failed to delete splits: %w", err)
		}
		return b.txnRepo.Delete(id)
	})
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
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	if err := s.investmentCounterpart.DeleteCounterpart(s.q(), rowID); err != nil {
		return fmt.Errorf("failed to delete investment-side paired transfer transaction: %w", err)
	}
	return nil
}

// ListByAccount returns all transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.txnRepo.ListByAccount(accountID)
}

// ListByAccountAndDateRange returns transactions for an account within a date range.
func (s *Service) ListByAccountAndDateRange(accountID types.ID, startDate, endDate types.Date) ([]*Transaction, error) {
	return s.txnRepo.ListByAccountAndDateRange(accountID, startDate, endDate)
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

	// Persist the parent, its split rows, and every transfer-line counterpart
	// in one transaction: a mid-flow failure rolls the whole thing back, so a
	// parent never lands without its splits (or with only some counterparts).
	return s.runInTx(func(b *Service) error {
		if err := b.txnRepo.Create(transaction); err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}

		for _, split := range splits {
			split.TransactionID = transaction.ID
			if err := b.splitRepo.Create(split); err != nil {
				return fmt.Errorf("failed to create split: %w", err)
			}

			if !split.TransferAccountID.Valid {
				continue
			}

			if err := b.createTransferLineCounterpart(transaction.AccountID, transaction.Date, split); err != nil {
				return err
			}
		}

		return nil
	})
}

// createTransferLineCounterpart mints the paired counter-transaction for
// a transfer-line split. If the target account is investment-type, the
// row is created on the investment_transactions table via the configured
// InvestmentCounterpartPort; otherwise a regular transaction is
// created on the transactions table.
//
// A category on the split (a "categorized transfer", e.g. a loan payment's
// principal line labeled Loan:Principal) is mirrored onto a regular-side
// counterpart. The investment adapter row has no category column, so an
// investment-side counterpart carries none — the split line holds it alone.
//
// The split must already carry a valid TransferID (CreateWithSplits and
// moveTransferLine mint it before calling here).
func (s *Service) createTransferLineCounterpart(
	parentAcctID types.ID,
	parentDate types.Date,
	split *Split,
) error {
	targetAcctID := split.TransferAccountID.ID
	counterAmount := split.Amount.Neg()
	transferID := split.TransferID.ID

	// Guard the target account: it must be open, and an investment target
	// requires the counterpart adapter. Guards CreateWithSplits,
	// moveTransferLine (which re-targets a split), and ReplaceSplits.
	isInv, err := s.ensureTransferTargetRoutable(targetAcctID)
	if err != nil {
		return err
	}

	if isInv {
		if _, err := s.investmentCounterpart.CreateCounterpart(
			s.q(),
			targetAcctID, parentAcctID, parentDate, counterAmount, "", transferID,
		); err != nil {
			return fmt.Errorf("failed to create investment-side paired transfer transaction: %w", err)
		}
		return nil
	}

	paired := NewTransaction(targetAcctID, parentDate, counterAmount)
	paired.SetTransfer(transferID, parentAcctID)
	if !split.CategoryID.IsNil() {
		paired.SetCategory(split.CategoryID)
	}
	if err := s.txnRepo.Create(paired); err != nil {
		return fmt.Errorf("failed to create paired transfer transaction: %w", err)
	}
	return nil
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
		// Mirror an amount or category change onto the paired counterpart. A
		// category edit alone still cascades so the counterpart's label stays in
		// sync with the canonical split line. Pre-flight the counterpart before
		// persisting the split row so a reconciled counterpart fails cleanly with
		// no partial write.
		amountChanged := !existing.Amount.Equal(split.Amount)
		categoryChanged := existing.CategoryID != split.CategoryID
		cascade := existing.TransferID.Valid && (amountChanged || categoryChanged)
		if cascade {
			if err := s.ensureRetainedCounterpartMutable(existing.TransferID.ID, amountChanged); err != nil {
				return err
			}
		}
		// Persist the split row and mirror onto its counterpart atomically so a
		// failed mirror can't leave the split and counterpart out of sync.
		return s.runInTx(func(b *Service) error {
			if err := b.splitRepo.Update(split); err != nil {
				return err
			}
			if cascade {
				return b.mirrorToPairedCounterpart(existing.TransferID.ID, split.Amount.Neg(), splitCategoryNullable(split), amountChanged)
			}
			return nil
		})
	}

	return s.splitRepo.Update(split)
}

// moveTransferLine handles the target-account-change cascade: delete the old
// paired counter-transaction, mint a fresh transfer_id on the split-line,
// persist the split, and create a new paired counterpart in the new target.
// The new counterpart is routed to the investment table when the new
// target is an investment account, and carries the split's category onto a
// regular-side counterpart (via createTransferLineCounterpart).
func (s *Service) moveTransferLine(parent *Transaction, existing, split *Split) error {
	split.TransferID = types.NullableID{ID: types.NewID(), Valid: true}

	// Delete the old counterpart, rewrite the split row, and mint the new
	// counterpart in one transaction: a failure at any step leaves the original
	// split and counterpart untouched.
	return s.runInTx(func(b *Service) error {
		if existing.TransferID.Valid {
			if err := b.deletePairedCounterTransaction(existing.TransferID.ID); err != nil {
				return err
			}
		}
		if err := b.splitRepo.Update(split); err != nil {
			return err
		}
		return b.createTransferLineCounterpart(parent.AccountID, parent.Date, split)
	})
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

// mirrorToPairedCounterpart syncs the paired counter-transaction of a retained
// transfer line to the line's current amount and category. A regular-side
// counterpart mirrors both amount and category. An investment-side counterpart
// has no category column, so only its amount is mirrored — and only when the
// amount actually changed: a category-only change is a no-op there and must not
// touch (or be blocked by) the investment row. A reconciled counterpart that
// would be written blocks the sync with IsReconciledError.
//
// newAmount is the counterpart's amount (already negated relative to the split
// line); categoryID is the split line's category (Valid:false clears it);
// amountChanged reports whether the amount actually changed.
func (s *Service) mirrorToPairedCounterpart(transferID types.ID, newAmount types.Money, categoryID types.NullableID, amountChanged bool) error {
	paired, err := s.findPairedByTransferID(transferID)
	if err != nil {
		return err
	}
	if paired != nil {
		if paired.IsReconciled() {
			return &IsReconciledError{ID: paired.ID.String()}
		}
		paired.Amount = newAmount
		paired.CategoryID = categoryID
		return s.txnRepo.Update(paired)
	}
	// Investment-side counterpart: no category column, so a category-only change
	// leaves it untouched.
	if !amountChanged || s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return s.investmentCounterpart.UpdateCounterpartAmount(s.q(), rowID, newAmount)
}

// ensureRetainedCounterpartMutable verifies a retained transfer line's
// counterpart can be re-synced, mirroring mirrorToPairedCounterpart's own
// blocking rule so ReplaceSplits/UpdateSplit fail cleanly before any write. A
// regular-side counterpart mirrors both fields, so a reconciled one always
// blocks; an investment-side counterpart is written only when the amount
// changed, so a reconciled one blocks only then (a category-only change never
// touches it).
func (s *Service) ensureRetainedCounterpartMutable(transferID types.ID, amountChanged bool) error {
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
	if !amountChanged || s.investmentCounterpart == nil {
		return nil
	}
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
	if err != nil {
		return err
	}
	if found && reconciled {
		return &IsReconciledError{ID: rowID.String()}
	}
	return nil
}

// splitCategoryNullable converts a split's plain CategoryID into a NullableID
// for mirroring onto a counter-transaction (whose category column is nullable).
// A nil category becomes an unset NullableID (which clears the counterpart's
// category).
func splitCategoryNullable(split *Split) types.NullableID {
	if split.CategoryID.IsNil() {
		return types.NullableID{Valid: false}
	}
	return types.NullableID{ID: split.CategoryID, Valid: true}
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

	// Delete the counterpart and the split row atomically so a mid-cascade
	// failure leaves both the split and its counterpart intact.
	return s.runInTx(func(b *Service) error {
		if split.TransferAccountID.Valid && split.TransferID.Valid {
			if err := b.deletePairedCounterTransaction(split.TransferID.ID); err != nil {
				return err
			}
		}
		return b.splitRepo.Delete(splitID)
	})
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
//     account) keeps its counterpart; an amount or category change mirrors
//     onto it;
//   - a removed transfer line's counterpart is deleted;
//   - an added transfer line mints a transfer_id and creates a counterpart
//     (carrying the line's category onto a regular-side counterpart).
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

	// Execute the whole plan — delete removed counterparts, re-sync retained
	// ones, rebuild the split rows, and mint counterparts for added lines — in
	// one transaction. A failure at any step rolls the entire rewrite back, so
	// the original splits and counterparts survive intact.
	return s.runInTx(func(b *Service) error {
		// Reconcile counterparts of removed and retained-changed transfer lines.
		for _, transferID := range plan.removedTransferIDs {
			if err := b.deletePairedCounterTransaction(transferID); err != nil {
				return err
			}
		}
		for _, change := range plan.retainedChanged {
			if err := b.mirrorToPairedCounterpart(change.transferID, change.newAmount.Neg(), change.newCategory, change.amountChanged); err != nil {
				return err
			}
		}

		// Rebuild the split rows. Retained transfer lines already carry their
		// original transfer_id (linking them to the still-live counterpart), so a
		// plain drop-and-recreate is safe.
		if _, err := b.splitRepo.DeleteByTransaction(transactionID); err != nil {
			return fmt.Errorf("failed to delete existing splits: %w", err)
		}
		for _, split := range splits {
			split.TransactionID = transactionID
			if err := b.splitRepo.Create(split); err != nil {
				return fmt.Errorf("failed to create split: %w", err)
			}
		}

		// Mint counterparts for the added transfer lines.
		for _, split := range plan.addedSplits {
			if err := b.createTransferLineCounterpart(txn.AccountID, txn.Date, split); err != nil {
				return err
			}
		}

		return nil
	})
}

// UpdateWithSplits updates a transaction's parent fields and replaces its splits
// as one atomic unit. It is the composed method the edit-with-splits undo command
// uses (for both Execute and Undo) so the parent update and the split rewrite
// commit together; the two bound calls join the single tx opened here. Update and
// ReplaceSplits are each individually transactional, but wrapping them here makes
// the pair atomic — a failure in the split rewrite rolls the parent update back.
func (s *Service) UpdateWithSplits(transaction *Transaction, splits []*Split) error {
	return s.runInTx(func(b *Service) error {
		if err := b.Update(transaction); err != nil {
			return err
		}
		return b.ReplaceSplits(transaction.ID, splits)
	})
}

// retainedTransferChange records a transfer line kept across a ReplaceSplits
// whose amount or category changed, so its counterpart can be re-synced.
// amountChanged distinguishes a category-only change (which never touches an
// investment-side counterpart) from an amount change.
type retainedTransferChange struct {
	transferID    types.ID
	newAmount     types.Money
	newCategory   types.NullableID
	amountChanged bool
}

// splitReplacementPlan captures the transfer-line diff computed by
// planSplitReplacement for ReplaceSplits.
type splitReplacementPlan struct {
	// Counterparts to delete (old transfer lines with no match in the new set).
	removedTransferIDs []types.ID
	// Retained transfer lines whose amount or category changed (re-synced onto
	// the counterpart).
	retainedChanged []retainedTransferChange
	// New transfer lines with no match in the old set. Each already has a
	// transfer_id assigned; a counterpart must be minted for it.
	addedSplits []*Split
}

// planSplitReplacement diffs the desired transfer lines against the current
// ones and assigns transfer_ids onto the new splits in place: a retained line
// (matched first by transfer_id, then by target account) inherits its match's
// transfer_id; an added line keeps a caller-supplied transfer_id or is minted
// a fresh one. A retained line whose amount or category changed is recorded so
// its counterpart is re-synced. Plain categorized (non-transfer) lines are
// ignored (they carry no counterpart and are recreated wholesale by
// ReplaceSplits).
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
			amountChanged := !ns.Amount.Equal(match.split.Amount)
			categoryChanged := ns.CategoryID != match.split.CategoryID
			if amountChanged || categoryChanged {
				plan.retainedChanged = append(plan.retainedChanged, retainedTransferChange{
					transferID:    match.split.TransferID.ID,
					newAmount:     ns.Amount,
					newCategory:   splitCategoryNullable(ns),
					amountChanged: amountChanged,
				})
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
// Counterparts that will be deleted or re-synced (amount or category change)
// must not be reconciled; added transfer lines must target a routable account.
func (s *Service) preflightSplitReplacement(plan splitReplacementPlan) error {
	for _, transferID := range plan.removedTransferIDs {
		if err := s.ensureCounterpartNotReconciled(transferID); err != nil {
			return err
		}
	}
	for _, change := range plan.retainedChanged {
		if err := s.ensureRetainedCounterpartMutable(change.transferID, change.amountChanged); err != nil {
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
	rowID, reconciled, found, err := s.investmentCounterpart.FindCounterpart(s.q(), transferID)
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

// ensureAccountOpen returns an AccountClosedError when the account is closed.
// A closed account is frozen: no new transactions, edits, deletes, or status
// toggles. It is nil-tolerant for test fixtures constructed without an
// accountRepo (matching targetIsInvestment); the production wiring always
// passes a real repository.
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

	// Status-only change: use the narrow in-place UpdateStatus so DuckDB does
	// not rewrite the row (and cannot trip a desynced ART index on another
	// column). See transaction.Repository.UpdateStatus / migration 030.
	return s.txnRepo.UpdateStatus(id, StatusCleared)
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

	// Status-only change: narrow in-place update (see ClearTransaction).
	return s.txnRepo.UpdateStatus(id, StatusReconciled)
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

	// Status-only change: narrow in-place update (see ClearTransaction).
	return s.txnRepo.UpdateStatus(id, StatusUncleared)
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

	// Status-only change: narrow in-place update (see ClearTransaction).
	return s.txnRepo.UpdateStatus(id, StatusCleared)
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

	// A whole-transaction transfer is voided through transfer.Service, which
	// zeroes both legs wherever they live and refuses by name when a leg is on
	// the investment ledger (which has no void status). A split-line counterpart
	// falls through to the cascade below, as before.
	if txn.IsTransfer() {
		parentSplit, err := s.splitRepo.GetByTransferID(txn.TransferID.ID)
		if err != nil {
			return err
		}
		if parentSplit == nil {
			return &IsTransferError{ID: id.String()}
		}
	}

	// Void the transaction
	txn.Amount = types.ZeroMoney
	txn.SetMemo("**VOID**")
	txn.Void()

	// Cascade to paired counter-transactions, drop the splits, and void the
	// parent in one transaction — otherwise a mid-cascade failure could orphan
	// a counterpart (dangling transfer_id) or leave the parent un-voided.
	return s.runInTx(func(b *Service) error {
		if err := b.deleteTransferLinePairs(id); err != nil {
			return err
		}
		if _, err := b.splitRepo.DeleteByTransaction(id); err != nil {
			return fmt.Errorf("failed to delete splits for void: %w", err)
		}
		return b.txnRepo.Update(txn)
	})
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

	// A single write today, but wrapped so it composes: phase 7's void-undo
	// command chains this with ReplaceSplits under one caller-supplied tx via
	// InTx, and a bound service joins that tx rather than opening its own.
	return s.runInTx(func(b *Service) error {
		return b.txnRepo.Update(txn)
	})
}

// RestoreVoidedTransactionWithSplits restores a voided transaction and, when the
// transaction had splits removed by the void, restores those splits too — all in
// one transaction. It is the composed method the void-undo command uses so the
// row restore and the split restore commit together (or not at all); the two
// bound calls join the single tx opened here. When splits is empty only the row
// is restored (matching a void of a plain, split-free transaction).
func (s *Service) RestoreVoidedTransactionWithSplits(id types.ID, amount types.Money, memo types.NullableString, status Status, splits []*Split) error {
	return s.runInTx(func(b *Service) error {
		if err := b.RestoreVoidedTransaction(id, amount, memo, status); err != nil {
			return err
		}
		if len(splits) > 0 {
			return b.ReplaceSplits(id, splits)
		}
		return nil
	})
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

	// A split parent containing a transfer line can't be duplicated: the
	// split-copy loop below drops transfer linkage, and after migration 029
	// relaxed the transaction_splits CHECK a categorized transfer line would
	// degrade silently into a plain categorized split with no counterpart.
	// Refuse up front, before creating anything.
	splits, err := s.splitRepo.ListByTransaction(transactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get splits: %w", err)
	}
	for _, split := range splits {
		if split.TransferAccountID.Valid {
			return nil, &CannotDuplicateSplitTransferError{ID: transactionID.String()}
		}
	}

	// Create a new transaction with same properties
	duplicate := NewTransaction(original.AccountID, types.Today(), original.Amount)
	duplicate.PayeeID = original.PayeeID
	duplicate.CategoryID = original.CategoryID
	duplicate.Memo = original.Memo
	duplicate.CheckNumber = original.CheckNumber
	// Status is always uncleared for duplicates (set by NewTransaction)

	// The parent and its split copies commit atomically — a split failure
	// rolls back the parent instead of best-effort cleanup.
	if err := s.runInTx(func(b *Service) error {
		if err := b.txnRepo.Create(duplicate); err != nil {
			return err
		}

		// Duplicate splits if any (guaranteed transfer-free by the guard above).
		for _, split := range splits {
			newSplit := NewSplit(duplicate.ID, split.CategoryID, split.Amount)
			newSplit.Memo = split.Memo
			if err := b.splitRepo.Create(newSplit); err != nil {
				return fmt.Errorf("failed to duplicate split: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
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
