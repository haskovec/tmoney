package investment

import (
	"fmt"
	"sort"

	"github.com/alpacahq/alpacadecimal"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for investment transaction operations.
type Service struct {
	repo                *Repository
	accountRepo         *account.Repository
	positionRepo        *PositionRepository
	lotRepo             *LotRepository
	transactionLotRepo  *TransactionLotRepository
	priceRepo           *price.Repository
	txnRepo             *transaction.Repository
	corporateActionRepo *CorporateActionRepository
	holdingsRepo        *HoldingsRepository
	db                  *db.DB
	tx                  db.Queryer // nil outside a transaction
}

// InTx returns a copy of the service bound to tx, with every repository field
// rebound to the same transaction so all writes join one atomic unit. The
// original service is unchanged and remains safe for non-transactional use.
//
// Optional repos (priceRepo, txnRepo, corporateActionRepo, holdingsRepo) are
// nil-guarded to match test fixtures that construct a partial service; the
// production wiring always sets them.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.repo = s.repo.WithTx(tx)
	c.accountRepo = s.accountRepo.WithTx(tx)
	c.positionRepo = s.positionRepo.WithTx(tx)
	c.lotRepo = s.lotRepo.WithTx(tx)
	c.transactionLotRepo = s.transactionLotRepo.WithTx(tx)
	if s.priceRepo != nil {
		c.priceRepo = s.priceRepo.WithTx(tx)
	}
	if s.txnRepo != nil {
		c.txnRepo = s.txnRepo.WithTx(tx)
	}
	if s.corporateActionRepo != nil {
		c.corporateActionRepo = s.corporateActionRepo.WithTx(tx)
	}
	if s.holdingsRepo != nil {
		c.holdingsRepo = s.holdingsRepo.WithTx(tx)
	}
	return &c
}

// CounterpartInTx returns a tx-bound copy of the service typed as a
// transaction.InvestmentCashCounterpartAdapter, so the transaction service's
// split/void flows mint, edit, and delete investment-side counterparts inside
// their own transaction. It is a thin alias for InTx: the interface method
// cannot be named InTx (that name is already taken by InTx(tx) *Service, whose
// signature differs), so the adapter contract spells it CounterpartInTx.
func (s *Service) CounterpartInTx(tx db.Queryer) transaction.InvestmentCashCounterpartAdapter {
	return s.InTx(tx)
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

// healInOwnTx runs syncPositionAndLots for (accountID, securityID) in its own
// committed transaction, before a trade's transaction is opened. The heal is
// idempotent repair — committing it is desirable even when the trade then
// fails, and it keeps the trade tx small.
//
// When the service is already tx-bound (a trade invoked from update_edit's
// reverse/reapply flow), the re-heal is skipped: the outermost entry point
// healed before opening the main tx, and the in-tx state is being mutated
// deliberately.
func (s *Service) healInOwnTx(accountID, securityID types.ID) error {
	if s.tx != nil {
		return nil // bound — the outermost entry point already healed
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return s.InTx(tx).syncPositionAndLots(accountID, securityID)
	})
}

// NewService creates a new Service.
func NewService(
	repo *Repository,
	accountRepo *account.Repository,
	positionRepo *PositionRepository,
	lotRepo *LotRepository,
	transactionLotRepo *TransactionLotRepository,
	priceRepo *price.Repository,
	txnRepo *transaction.Repository,
	corporateActionRepo *CorporateActionRepository,
	database *db.DB,
) *Service {
	return &Service{
		repo:                repo,
		accountRepo:         accountRepo,
		positionRepo:        positionRepo,
		lotRepo:             lotRepo,
		transactionLotRepo:  transactionLotRepo,
		priceRepo:           priceRepo,
		txnRepo:             txnRepo,
		corporateActionRepo: corporateActionRepo,
		holdingsRepo:        NewHoldingsRepository(database),
		db:                  database,
	}
}

// Deposit creates a deposit transaction that increases the cash position in an investment account.
func (s *Service) Deposit(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeDeposit, amount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create deposit: %w", err)
	}

	return txn, nil
}

// Withdrawal creates a withdrawal transaction that decreases the cash position in an investment account.
func (s *Service) Withdrawal(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Cash balance is allowed to go negative — withdrawals never block on
	// the running balance so historical data entry isn't ordering-sensitive.

	// Store as negative amount for withdrawal
	negAmount := amount.Neg()

	txn := NewTransaction(accountID, date, TransactionTypeWithdrawal, negAmount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}

	return txn, nil
}

// Interest creates an interest transaction that increases the cash position in an investment account.
func (s *Service) Interest(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeInterest, amount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create interest transaction: %w", err)
	}

	return txn, nil
}

// Fee creates a fee transaction that decreases the cash position in an investment account.
func (s *Service) Fee(accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Cash balance is allowed to go negative — see Withdrawal for rationale.

	// Store as negative amount for fee
	negAmount := amount.Neg()

	txn := NewTransaction(accountID, date, TransactionTypeFee, negAmount)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create fee transaction: %w", err)
	}

	return txn, nil
}

// Buy creates a buy transaction that purchases shares of a security.
// For non-lot-tracking accounts, it updates the aggregate position.
// For lot-tracking accounts, it creates a new lot.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) Buy(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal any stale stored position/lot state for this (account, security)
	// before we read it (no-op when corporate actions are present). The heal
	// commits in its own tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute buy fields: %w", err)
	}

	// Cash balance is allowed to go negative — bank statements often list
	// the day's sales after the day's buys, and we shouldn't require the
	// user to reorder same-date entries to get past a transient shortfall.

	// Create transaction with negative total (buy deducts cash)
	negTotal := computed.TotalAmount.Neg()
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeBuy, negTotal, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if !commission.IsZero() {
		txn.SetCommission(commission)
	}
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row, the lot/position, and the auto-price atomically: a buy
	// either fully lands or not at all (no orphan transaction on a lot failure).
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(txn); err != nil {
			return fmt.Errorf("failed to create buy transaction: %w", err)
		}

		// Update position or create lot based on account tracking mode
		if acct.TrackLots {
			lot := NewLot(accountID, securityID, shares, computed.PricePerShare, date, txn.ID)
			if err := b.lotRepo.Create(&lot); err != nil {
				return fmt.Errorf("failed to create lot: %w", err)
			}
		} else {
			pos, err := b.positionRepo.GetByAccountAndSecurity(accountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get position: %w", err)
			}
			if err := pos.AddShares(shares, computed.PricePerShare); err != nil {
				return fmt.Errorf("failed to update position: %w", err)
			}
			if err := b.positionRepo.CreateOrUpdate(pos); err != nil {
				return fmt.Errorf("failed to save position: %w", err)
			}
		}

		// Auto-create price record from transaction
		b.autoCreatePrice(securityID, date, computed.PricePerShare)
		return nil
	}); err != nil {
		return nil, err
	}

	return txn, nil
}

// SellLotAllocation specifies how many shares to sell from a specific lot.
type SellLotAllocation struct {
	LotID  types.ID
	Shares types.Quantity
}

// Sell creates a sell transaction that sells shares of a security.
// For non-lot-tracking accounts, it reduces the aggregate position.
// For lot-tracking accounts, lotAllocations specifies which lots to sell from.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) Sell(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal any stale stored position/lot state before validating. The heal
	// commits in its own tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute sell fields: %w", err)
	}

	// Create transaction with positive total (sell adds cash)
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeSell, computed.TotalAmount, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if !commission.IsZero() {
		txn.SetCommission(commission)
	}
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row, the lot/position changes, and the auto-price atomically.
	if err := s.runInTx(func(b *Service) error {
		if acct.TrackLots {
			if err := b.sellWithLots(txn, accountID, securityID, shares, lotAllocations); err != nil {
				return err
			}
		} else {
			if err := b.sellWithPosition(txn, accountID, securityID, shares); err != nil {
				return err
			}
		}

		// Auto-create price record from transaction
		b.autoCreatePrice(securityID, date, computed.PricePerShare)
		return nil
	}); err != nil {
		return nil, err
	}

	return txn, nil
}

// sellWithPosition handles sell for non-lot-tracking accounts.
func (s *Service) sellWithPosition(txn *Transaction, accountID, securityID types.ID, shares types.Quantity) error {
	pos, err := s.positionRepo.GetByAccountAndSecurity(accountID, securityID)
	if err != nil {
		return fmt.Errorf("failed to get position: %w", err)
	}

	if pos.Shares.Cmp(shares) < 0 {
		return &InsufficientSharesError{
			SecurityID: securityID.String(),
			Available:  pos.Shares,
			Requested:  shares,
		}
	}

	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create sell transaction: %w", err)
	}

	if err := pos.RemoveShares(shares); err != nil {
		return fmt.Errorf("failed to update position: %w", err)
	}

	if pos.Shares.IsZero() {
		if err := s.positionRepo.Delete(accountID, securityID); err != nil {
			return fmt.Errorf("failed to delete zero position: %w", err)
		}
	} else {
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to save position: %w", err)
		}
	}

	return nil
}

// sellWithLots handles sell for lot-tracking accounts.
func (s *Service) sellWithLots(txn *Transaction, accountID, securityID types.ID, shares types.Quantity, lotAllocations []SellLotAllocation) error {
	if len(lotAllocations) == 0 {
		return fmt.Errorf("lot allocations required for lot-tracking account")
	}

	// Validate total shares across allocations equals sell shares
	totalAllocated := types.ZeroQuantity
	for _, alloc := range lotAllocations {
		totalAllocated = totalAllocated.Add(alloc.Shares)
	}
	if totalAllocated.Cmp(shares) != 0 {
		return &LotAllocationMismatchError{
			Expected: shares,
			Actual:   totalAllocated,
		}
	}

	// Validate each lot allocation before making any changes
	lots := make([]*Lot, len(lotAllocations))
	for i, alloc := range lotAllocations {
		lot, err := s.lotRepo.GetByID(alloc.LotID)
		if err != nil {
			return &LotNotFoundError{LotID: alloc.LotID.String()}
		}
		if lot.AccountID != accountID {
			return &LotWrongAccountError{
				LotID:     alloc.LotID.String(),
				AccountID: accountID.String(),
			}
		}
		if lot.SecurityID != securityID {
			return fmt.Errorf("lot %s is for a different security", alloc.LotID)
		}
		if lot.Closed {
			return fmt.Errorf("lot %s is closed", alloc.LotID)
		}
		if lot.Shares.Cmp(alloc.Shares) < 0 {
			return &LotInsufficientSharesError{
				LotID:     alloc.LotID.String(),
				Available: lot.Shares,
				Requested: alloc.Shares,
			}
		}
		lots[i] = lot
	}

	// All validations passed — persist the transaction
	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create sell transaction: %w", err)
	}

	// Reduce lots and create junction records
	for i, alloc := range lotAllocations {
		lot := lots[i]
		if err := lot.Reduce(alloc.Shares); err != nil {
			return fmt.Errorf("failed to reduce lot: %w", err)
		}
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot: %w", err)
		}

		tl := NewTransactionLot(txn.ID, lot.ID, alloc.Shares)
		if err := s.transactionLotRepo.Create(&tl); err != nil {
			return fmt.Errorf("failed to create transaction lot: %w", err)
		}
	}

	return nil
}

// GetCashBalance computes the cash balance for an investment account by summing
// all cash-affecting transactions.
func (s *Service) GetCashBalance(accountID types.ID) (types.Money, error) {
	txns, err := s.repo.ListByAccount(accountID, TransactionFilter{})
	if err != nil {
		return types.ZeroMoney, fmt.Errorf("failed to list transactions: %w", err)
	}

	balance := types.ZeroMoney
	for _, txn := range txns {
		if txn.Type.AffectsCash() {
			balance = balance.Add(txn.TotalAmount)
		}
	}

	return balance, nil
}

// TotalSharesForSecurity sums the current shares held for a security across all
// investment/HSA accounts — open lots for lot-tracked accounts, the aggregate
// position otherwise. Used to derive a spin-off's share ratio from a statement's
// resulting-share count (ratio = resulting_shares / total_shares).
func (s *Service) TotalSharesForSecurity(securityID types.ID) (types.Quantity, error) {
	accounts, err := s.accountRepo.List(false)
	if err != nil {
		return types.ZeroQuantity, fmt.Errorf("TotalSharesForSecurity: %w", err)
	}
	total := types.ZeroQuantity
	for _, acct := range accounts {
		if !acct.Type.IsInvestmentType() {
			continue
		}
		if acct.TrackLots {
			lots, err := s.lotRepo.ListByAccountAndSecurity(acct.ID, securityID, false)
			if err != nil {
				return types.ZeroQuantity, fmt.Errorf("TotalSharesForSecurity: %w", err)
			}
			for _, l := range lots {
				total = total.Add(l.Shares)
			}
			continue
		}
		pos, err := s.positionRepo.GetByAccountAndSecurity(acct.ID, securityID)
		if err != nil {
			continue
		}
		total = total.Add(pos.Shares)
	}
	return total, nil
}

// requireInvestmentAccount verifies that the given account exists, is an
// investment account, and is not closed. It is a write-path guard — only
// mutation methods call it, so the closed check belongs here (read and
// maintenance paths use getInvestmentAccount directly and stay ungated).
func (s *Service) requireInvestmentAccount(accountID types.ID) error {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return err
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: accountID.String()}
	}
	return nil
}

// ensureAccountOpen returns an AccountClosedError when the account is closed.
// Used by mutation paths that hold only an account ID (e.g. DeleteTransaction);
// it deliberately does NOT funnel through getInvestmentAccount so read and
// maintenance paths remain ungated.
func (s *Service) ensureAccountOpen(accountID types.ID) error {
	acct, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("failed to load account for closed check: %w", err)
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: accountID.String()}
	}
	return nil
}

// SetClearedStatus marks an investment transaction cleared or pending. It is the
// service-layer chokepoint for the register's cleared toggle, so the
// closed-account freeze gate applies here (a closed account is frozen).
func (s *Service) SetClearedStatus(txnID types.ID, cleared bool) error {
	txn, err := s.repo.GetByID(txnID)
	if err != nil {
		return err
	}
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}
	if cleared {
		txn.Clear()
	} else {
		txn.MarkPending()
	}
	return s.repo.Update(txn)
}

// getInvestmentAccount retrieves and validates that the account is an investment account.
func (s *Service) getInvestmentAccount(accountID types.ID) (*account.Account, error) {
	acct, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	if !acct.Type.IsInvestmentType() {
		return nil, &account.NotInvestmentError{
			AccountID: accountID.String(),
			Type:      string(acct.Type),
		}
	}

	return acct, nil
}

// validateTransaction validates an investment transaction and returns any validation errors.
func (s *Service) validateTransaction(txn *Transaction) error {
	errors := txn.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	// Reject activity dated before the account opened (catches mistyped years
	// such as "0018" for "2018"). Corporate-action Exchange rows carry the
	// action date and are written via the repository, not this path, so they
	// are never seen here; the type guard is belt-and-suspenders.
	if txn.Type != TransactionTypeExchange {
		acct, err := s.accountRepo.GetByID(txn.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load account for date validation: %w", err)
		}
		if err := acct.ValidateTransactionDate(txn.Date); err != nil {
			return err
		}
	}
	return nil
}

// autoCreatePrice creates a price record with source=transaction for the given security+date.
// If a manual or import price already exists for that date, it does NOT overwrite it.
func (s *Service) autoCreatePrice(securityID types.ID, date types.Date, pricePerShare types.Money) {
	if s.priceRepo == nil {
		return
	}

	// Check if a price already exists for this security+date
	existing, err := s.priceRepo.GetBySecurityAndDate(securityID, date)
	if err == nil && existing != nil {
		// Price already exists — do not overwrite
		return
	}

	// Only proceed if the error was NotFoundError (no existing price)
	if err != nil {
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			// Unexpected error — silently skip price creation
			return
		}
	}

	p := price.NewPrice(securityID, date, pricePerShare, price.SourceTransaction)
	_ = s.priceRepo.Create(p)
}

// cleanupAutoPrice reconciles the auto-created (source=transaction) price row
// at (securityID, date) after a price-generating transaction has been moved
// off that date or deleted. Auto prices are keyed per (security, date) and are
// shared by every priced transaction that lands there, so this is careful:
//
//   - If no price-generating transaction remains on that date, the row is an
//     orphan and is removed. (This prevents the classic "fixed a buy's year
//     from 0018 to 2018 but the 0018 price row stayed behind" bug, which
//     stretched the price chart across ~2000 years.)
//   - If one or more remain — e.g. two same-day lots and only one was edited or
//     deleted — the row is kept and re-pointed to a surviving transaction's
//     price, so the stored daily price always reflects a transaction that
//     actually exists on that date.
//
// Manual, import, and API prices are never touched. Best-effort: any repo error
// leaves existing state untouched rather than failing the surrounding edit.
func (s *Service) cleanupAutoPrice(securityID types.ID, date types.Date) {
	if s.priceRepo == nil {
		return
	}
	existing, err := s.priceRepo.GetBySecurityAndDate(securityID, date)
	if err != nil || existing == nil {
		return // nothing on this date (or unexpected error) — leave it alone
	}
	if existing.Source != price.SourceTransaction {
		return // never disturb a manual / import / api price
	}

	// Collect the price-generating transactions still on this date, in creation
	// order (ListBySecurity returns date ASC, created_at ASC).
	txns, err := s.repo.ListBySecurity(securityID)
	if err != nil {
		return
	}
	var survivors []*Transaction
	for _, t := range txns {
		if t.Date.Equal(date) && t.Type.CreatesAutoPrice() && t.PricePerShare.Valid {
			survivors = append(survivors, t)
		}
	}

	if len(survivors) == 0 {
		_ = s.priceRepo.Delete(existing.ID)
		return
	}

	// Re-point to the earliest surviving transaction's price (preserving the
	// original first-write-wins seeding). Skip the write if already correct.
	want := survivors[0].PricePerShare.Money
	if !existing.Price.Equal(want) {
		existing.Price = want
		_ = s.priceRepo.CreateOrUpdate(existing)
	}
}

// Dividend creates a cash dividend transaction that increases the cash position.
// The security and share count are unchanged. The amount is stored as positive (adds cash).
func (s *Service) Dividend(accountID types.ID, securityID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.requireInvestmentAccount(accountID); err != nil {
		return nil, err
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	txn := NewTransaction(accountID, date, TransactionTypeDividend, amount)
	txn.SetSecurity(securityID)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create dividend transaction: %w", err)
	}

	return txn, nil
}

// ReinvestDividend creates a reinvest dividend transaction that adds shares without cash movement.
// For non-lot-tracking accounts, it updates the aggregate position.
// For lot-tracking accounts, it creates a new lot.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) ReinvestDividend(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	memo string,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal in its own committed tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields (no commission for reinvest)
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, types.ZeroMoney)
	if err != nil {
		return nil, fmt.Errorf("failed to compute reinvest dividend fields: %w", err)
	}

	// Create transaction — no cash movement, so TotalAmount stored as positive for record-keeping
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeReinvestDividend, computed.TotalAmount, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// Persist the row and the lot/position atomically.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(txn); err != nil {
			return fmt.Errorf("failed to create reinvest dividend transaction: %w", err)
		}

		// Update position or create lot based on account tracking mode
		if acct.TrackLots {
			lot := NewLot(accountID, securityID, shares, computed.PricePerShare, date, txn.ID)
			if err := b.lotRepo.Create(&lot); err != nil {
				return fmt.Errorf("failed to create lot: %w", err)
			}
		} else {
			pos, err := b.positionRepo.GetByAccountAndSecurity(accountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get position: %w", err)
			}
			if err := pos.AddShares(shares, computed.PricePerShare); err != nil {
				return fmt.Errorf("failed to update position: %w", err)
			}
			if err := b.positionRepo.CreateOrUpdate(pos); err != nil {
				return fmt.Errorf("failed to save position: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Reinvested dividends do NOT auto-create a price (see CreatesAutoPrice):
	// their per-share value is total_amount÷rounded-shares, unreliable for tiny
	// income events. The security is priced from buys/sells instead.
	return txn, nil
}

// FeeLiquidation creates a fee-via-liquidation transaction that sells shares to cover a fee.
// There is no net cash effect — the shares are sold and the proceeds pay the fee.
// For non-lot-tracking accounts, it reduces the aggregate position.
// For lot-tracking accounts, lotAllocations specifies which lots to sell from.
// At least shares + one of (totalAmount, pricePerShare) must be provided.
func (s *Service) FeeLiquidation(
	accountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	acct, err := s.getInvestmentAccount(accountID)
	if err != nil {
		return nil, err
	}
	if acct.IsClosed() {
		return nil, &account.AccountClosedError{ID: accountID.String()}
	}

	// Heal in its own committed tx before the trade tx; a bound service skips it.
	if err := s.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}

	// Smart compute missing fields
	computed, err := SmartCompute(shares, totalAmount, pricePerShare, commission)
	if err != nil {
		return nil, fmt.Errorf("failed to compute fee liquidation fields: %w", err)
	}

	// Create transaction — TotalAmount stored as positive for record-keeping (no cash effect)
	txn := NewTransactionWithSecurity(accountID, date, TransactionTypeFeeLiquidation, computed.TotalAmount, securityID, shares)
	txn.SetPricePerShare(computed.PricePerShare)
	if !commission.IsZero() {
		txn.SetCommission(commission)
	}
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return nil, err
	}

	// On a lot-tracked account with no explicit allocation, auto-allocate FIFO
	// across the security's open lots. This is computed here (not in the caller)
	// so that UpdateFeeLiquidation — which reverses the old transaction's lot
	// effect *before* calling this method — allocates against the restored,
	// post-reverse lot state rather than a stale pre-reverse snapshot. Recurring
	// fee liquidations don't warrant per-lot tax selection, so FIFO (oldest
	// first) is the sensible default; pass explicit allocations to override.
	if acct.TrackLots && len(lotAllocations) == 0 {
		openLots, err := s.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
		if err != nil {
			return nil, fmt.Errorf("FeeLiquidation: load open lots for FIFO allocation: %w", err)
		}
		lotAllocations, err = fifoLotAllocations(securityID, openLots, shares)
		if err != nil {
			return nil, err
		}
	}

	// Persist the row and the lot/position changes atomically.
	if err := s.runInTx(func(b *Service) error {
		if acct.TrackLots {
			return b.feeLiquidationWithLots(txn, accountID, securityID, shares, lotAllocations)
		}
		return b.feeLiquidationWithPosition(txn, accountID, securityID, shares)
	}); err != nil {
		return nil, err
	}

	// Fee liquidations do NOT auto-create a price (see CreatesAutoPrice): the
	// per-share value is total_amount÷rounded-shares, unreliable for tiny fees.
	// The security is priced from buys/sells instead.
	return txn, nil
}

// feeLiquidationWithPosition handles fee liquidation for non-lot-tracking accounts.
func (s *Service) feeLiquidationWithPosition(txn *Transaction, accountID, securityID types.ID, shares types.Quantity) error {
	pos, err := s.positionRepo.GetByAccountAndSecurity(accountID, securityID)
	if err != nil {
		return fmt.Errorf("failed to get position: %w", err)
	}

	if pos.Shares.Cmp(shares) < 0 {
		return &InsufficientSharesError{
			SecurityID: securityID.String(),
			Available:  pos.Shares,
			Requested:  shares,
		}
	}

	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create fee liquidation transaction: %w", err)
	}

	if err := pos.RemoveShares(shares); err != nil {
		return fmt.Errorf("failed to update position: %w", err)
	}

	if pos.Shares.IsZero() {
		if err := s.positionRepo.Delete(accountID, securityID); err != nil {
			return fmt.Errorf("failed to delete zero position: %w", err)
		}
	} else {
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return fmt.Errorf("failed to save position: %w", err)
		}
	}

	return nil
}

// feeLiquidationWithLots handles fee liquidation for lot-tracking accounts.
func (s *Service) feeLiquidationWithLots(txn *Transaction, accountID, securityID types.ID, shares types.Quantity, lotAllocations []SellLotAllocation) error {
	if len(lotAllocations) == 0 {
		return fmt.Errorf("lot allocations required for lot-tracking account")
	}

	// Validate total shares across allocations equals fee liquidation shares
	totalAllocated := types.ZeroQuantity
	for _, alloc := range lotAllocations {
		totalAllocated = totalAllocated.Add(alloc.Shares)
	}
	if totalAllocated.Cmp(shares) != 0 {
		return &LotAllocationMismatchError{
			Expected: shares,
			Actual:   totalAllocated,
		}
	}

	// Validate each lot allocation before making any changes
	lots := make([]*Lot, len(lotAllocations))
	for i, alloc := range lotAllocations {
		lot, err := s.lotRepo.GetByID(alloc.LotID)
		if err != nil {
			return &LotNotFoundError{LotID: alloc.LotID.String()}
		}
		if lot.AccountID != accountID {
			return &LotWrongAccountError{
				LotID:     alloc.LotID.String(),
				AccountID: accountID.String(),
			}
		}
		if lot.SecurityID != securityID {
			return fmt.Errorf("lot %s is for a different security", alloc.LotID)
		}
		if lot.Closed {
			return fmt.Errorf("lot %s is closed", alloc.LotID)
		}
		if lot.Shares.Cmp(alloc.Shares) < 0 {
			return &LotInsufficientSharesError{
				LotID:     alloc.LotID.String(),
				Available: lot.Shares,
				Requested: alloc.Shares,
			}
		}
		lots[i] = lot
	}

	// All validations passed — persist the transaction
	if err := s.repo.Create(txn); err != nil {
		return fmt.Errorf("failed to create fee liquidation transaction: %w", err)
	}

	// Reduce lots and create junction records
	for i, alloc := range lotAllocations {
		lot := lots[i]
		if err := lot.Reduce(alloc.Shares); err != nil {
			return fmt.Errorf("failed to reduce lot: %w", err)
		}
		if err := s.lotRepo.Update(lot); err != nil {
			return fmt.Errorf("failed to update lot: %w", err)
		}

		tl := NewTransactionLot(txn.ID, lot.ID, alloc.Shares)
		if err := s.transactionLotRepo.Create(&tl); err != nil {
			return fmt.Errorf("failed to create transaction lot: %w", err)
		}
	}

	return nil
}

// fifoLotAllocations consumes `shares` across the given open lots oldest-first
// (the caller passes them in purchase-date order, which is FIFO) and returns
// the per-lot allocations. It is used to auto-allocate a fee liquidation on a
// lot-tracked account when the caller supplies no explicit allocation. It
// returns an *InsufficientSharesError when the open lots don't cover `shares`.
func fifoLotAllocations(securityID types.ID, openLots []*Lot, shares types.Quantity) ([]SellLotAllocation, error) {
	var allocations []SellLotAllocation
	remaining := shares
	for _, lot := range openLots {
		if !remaining.IsPositive() {
			break
		}
		take := lot.Shares
		if take.Cmp(remaining) > 0 {
			take = remaining
		}
		allocations = append(allocations, SellLotAllocation{LotID: lot.ID, Shares: take})
		remaining = remaining.Sub(take)
	}
	if remaining.IsPositive() {
		return nil, &InsufficientSharesError{
			SecurityID: securityID.String(),
			Available:  shares.Sub(remaining),
			Requested:  shares,
		}
	}
	return allocations, nil
}

// CashTransferResult contains both sides of a cash transfer between
// an investment account and a regular account. For an inv↔inv transfer
// surfaced through UpdateTransferCash's dispatch, RegularTransaction is nil
// and CounterpartInvestmentTransaction carries the destination-side investment
// row instead.
type CashTransferResult struct {
	InvestmentTransaction            *Transaction
	RegularTransaction               *transaction.Transaction
	CounterpartInvestmentTransaction *Transaction
	TransferID                       types.ID
}

// TransferCash creates a cash transfer between an investment account and a regular (non-investment) account.
// When direction is "in" (deposit to investment), the regular account is debited and the investment account is credited.
// When direction is "out" (withdrawal from investment), the investment account is debited and the regular account is credited.
// Both transactions are linked by a shared transfer_id.
func (s *Service) TransferCash(investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string, categoryID types.NullableID) (*CashTransferResult, error) {
	if s.txnRepo == nil {
		return nil, fmt.Errorf("transaction repository not configured")
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Validate investment account
	if err := s.requireInvestmentAccount(investmentAccountID); err != nil {
		return nil, err
	}

	// Validate regular account exists and is not an investment account
	regularAcct, err := s.accountRepo.GetByID(regularAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get regular account: %w", err)
	}
	if regularAcct.Type.IsInvestmentType() {
		return nil, &NotRegularAccountError{
			AccountID: regularAccountID.String(),
			Type:      string(regularAcct.Type),
		}
	}
	// The regular leg of the transfer is frozen if its account is closed.
	if regularAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: regularAccountID.String()}
	}

	// Check that the two accounts are different
	if investmentAccountID == regularAccountID {
		return nil, fmt.Errorf("cannot transfer between the same account")
	}

	// The investment-side row is date-checked by validateTransaction; guard
	// the regular-side account's opening date here (it goes through txnRepo).
	if err := regularAcct.ValidateTransactionDate(date); err != nil {
		return nil, err
	}

	// Cash balance is allowed to go negative — see Withdrawal for rationale.

	transferID := types.NewID()

	// Create investment transaction (withdrawal — negative amount)
	negAmount := amount.Neg()
	invTxn := NewTransaction(investmentAccountID, date, TransactionTypeTransferCash, negAmount)
	invTxn.SetTransfer(transferID, regularAccountID)
	if memo != "" {
		invTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(invTxn); err != nil {
		return nil, err
	}

	// Create regular transaction (deposit — positive amount). An optional
	// category labels the regular-side leg only (investment_transactions has no
	// category column); the caller is responsible for rejecting system
	// categories before calling.
	regTxn := transaction.NewTransaction(regularAccountID, date, amount)
	regTxn.SetTransfer(transferID, investmentAccountID)
	if memo != "" {
		regTxn.SetMemo(memo)
	}
	if categoryID.Valid {
		regTxn.SetCategory(categoryID.ID)
	}

	// Both legs commit atomically — either both land or neither does.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(invTxn); err != nil {
			return fmt.Errorf("failed to create investment transfer transaction: %w", err)
		}
		if err := b.txnRepo.Create(regTxn); err != nil {
			return fmt.Errorf("failed to create regular transfer transaction: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &CashTransferResult{
		InvestmentTransaction: invTxn,
		RegularTransaction:    regTxn,
		TransferID:            transferID,
	}, nil
}

// DepositFromAccount transfers cash from a regular account into an investment account.
// This creates a deposit in the investment account and a withdrawal in the regular account.
func (s *Service) DepositFromAccount(investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string, categoryID types.NullableID) (*CashTransferResult, error) {
	if s.txnRepo == nil {
		return nil, fmt.Errorf("transaction repository not configured")
	}

	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	// Validate investment account
	if err := s.requireInvestmentAccount(investmentAccountID); err != nil {
		return nil, err
	}

	// Validate regular account exists and is not an investment account
	regularAcct, err := s.accountRepo.GetByID(regularAccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get regular account: %w", err)
	}
	if regularAcct.Type.IsInvestmentType() {
		return nil, &NotRegularAccountError{
			AccountID: regularAccountID.String(),
			Type:      string(regularAcct.Type),
		}
	}
	// The regular leg of the transfer is frozen if its account is closed.
	if regularAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: regularAccountID.String()}
	}

	// Check that the two accounts are different
	if investmentAccountID == regularAccountID {
		return nil, fmt.Errorf("cannot transfer between the same account")
	}

	// The investment-side row is date-checked by validateTransaction; guard
	// the regular-side account's opening date here (it goes through txnRepo).
	if err := regularAcct.ValidateTransactionDate(date); err != nil {
		return nil, err
	}

	transferID := types.NewID()

	// Create investment transaction (deposit — positive amount)
	invTxn := NewTransaction(investmentAccountID, date, TransactionTypeTransferCash, amount)
	invTxn.SetTransfer(transferID, regularAccountID)
	if memo != "" {
		invTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(invTxn); err != nil {
		return nil, err
	}

	// Create regular transaction (withdrawal — negative amount). An optional
	// category labels the regular-side leg only (investment_transactions has no
	// category column); the caller is responsible for rejecting system
	// categories before calling.
	negAmount := amount.Neg()
	regTxn := transaction.NewTransaction(regularAccountID, date, negAmount)
	regTxn.SetTransfer(transferID, investmentAccountID)
	if memo != "" {
		regTxn.SetMemo(memo)
	}
	if categoryID.Valid {
		regTxn.SetCategory(categoryID.ID)
	}

	// Both legs commit atomically — either both land or neither does.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(invTxn); err != nil {
			return fmt.Errorf("failed to create investment transfer transaction: %w", err)
		}
		if err := b.txnRepo.Create(regTxn); err != nil {
			return fmt.Errorf("failed to create regular transfer transaction: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &CashTransferResult{
		InvestmentTransaction: invTxn,
		RegularTransaction:    regTxn,
		TransferID:            transferID,
	}, nil
}

// InvestmentCashTransferResult contains both sides of a cash transfer between
// two investment accounts (e.g. an IRA-to-IRA rollover). Parallel in shape to
// ShareTransferResult, but no security is involved.
type InvestmentCashTransferResult struct {
	SourceTransaction      *Transaction
	DestinationTransaction *Transaction
	TransferID             types.ID
}

// TransferCashBetweenInvestments creates a cash transfer between two investment
// accounts. The source account is debited and the destination credited; no
// regular-account row is involved. Both legs are linked by a shared transfer_id
// and typed TransferCash. Mirrors the TransferShares pattern.
func (s *Service) TransferCashBetweenInvestments(
	sourceAccountID, destAccountID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
) (*InvestmentCashTransferResult, error) {
	if !amount.IsPositive() {
		return nil, &InvalidTransferAmountError{Amount: amount}
	}

	if sourceAccountID == destAccountID {
		return nil, fmt.Errorf("cannot transfer between the same account")
	}

	if err := s.requireInvestmentAccount(sourceAccountID); err != nil {
		return nil, err
	}
	if err := s.requireInvestmentAccount(destAccountID); err != nil {
		return nil, err
	}

	// Cash balance is allowed to go negative — see Withdrawal for rationale.

	transferID := types.NewID()

	// Source row: negative amount (cash leaving the source account).
	negAmount := amount.Neg()
	srcTxn := NewTransaction(sourceAccountID, date, TransactionTypeTransferCash, negAmount)
	srcTxn.SetTransfer(transferID, destAccountID)
	if memo != "" {
		srcTxn.SetMemo(memo)
	}
	if err := s.validateTransaction(srcTxn); err != nil {
		return nil, err
	}

	// Destination row: positive amount (cash arriving at the destination).
	dstTxn := NewTransaction(destAccountID, date, TransactionTypeTransferCash, amount)
	dstTxn.SetTransfer(transferID, sourceAccountID)
	if memo != "" {
		dstTxn.SetMemo(memo)
	}
	if err := s.validateTransaction(dstTxn); err != nil {
		return nil, err
	}

	// Both legs commit atomically — either both land or neither does.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(srcTxn); err != nil {
			return fmt.Errorf("failed to create source transfer transaction: %w", err)
		}
		if err := b.repo.Create(dstTxn); err != nil {
			return fmt.Errorf("failed to create destination transfer transaction: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &InvestmentCashTransferResult{
		SourceTransaction:      srcTxn,
		DestinationTransaction: dstTxn,
		TransferID:             transferID,
	}, nil
}

// CreateTransferCashCounterpart mints a one-sided investment.Transaction
// of type TransferCash on invAcctID, linked by the caller-supplied
// transferID to otherAcctID. The signed amount controls direction
// (positive = cash arriving, negative = cash leaving), matching the sign
// of the destination leg of TransferCash / DepositFromAccount.
//
// Used by transaction.Service to mint the investment-side counterpart
// of a transfer-line split (e.g. a paycheck → 401k contribution line)
// whose target is an investment account, replacing the malformed regular
// counterpart that older code produced. Satisfies the
// transaction.InvestmentCashCounterpartAdapter contract.
func (s *Service) CreateTransferCashCounterpart(
	invAcctID, otherAcctID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	transferID types.ID,
) (types.ID, error) {
	if err := s.requireInvestmentAccount(invAcctID); err != nil {
		return types.ID{}, err
	}

	txn := NewTransaction(invAcctID, date, TransactionTypeTransferCash, amount)
	txn.SetTransfer(transferID, otherAcctID)
	if memo != "" {
		txn.SetMemo(memo)
	}

	if err := s.validateTransaction(txn); err != nil {
		return types.ID{}, err
	}

	if err := s.repo.Create(txn); err != nil {
		return types.ID{}, fmt.Errorf("failed to create investment-side counterpart: %w", err)
	}

	return txn.ID, nil
}

// FindTransferCashCounterpart returns the investment row linked to the
// given transferID. Returns found=false (no error) if no investment-side
// row exists. reconciled reports whether the row is fully reconciled,
// which callers use to block cascading deletes/edits.
func (s *Service) FindTransferCashCounterpart(transferID types.ID) (rowID types.ID, reconciled bool, found bool, err error) {
	rows, err := s.repo.ListByTransferID(transferID)
	if err != nil {
		return types.ID{}, false, false, fmt.Errorf("failed to look up investment-side counterpart: %w", err)
	}
	if len(rows) == 0 {
		return types.ID{}, false, false, nil
	}
	row := rows[0]
	return row.ID, row.IsReconciled(), true, nil
}

// DeleteTransferCashCounterpart removes the investment row identified by
// rowID. The caller is responsible for the regular-side parent or
// counterpart cleanup; no cascade is performed here.
func (s *Service) DeleteTransferCashCounterpart(rowID types.ID) error {
	if err := s.repo.Delete(rowID); err != nil {
		return fmt.Errorf("failed to delete investment-side counterpart: %w", err)
	}
	return nil
}

// UpdateTransferCashCounterpartAmount mirrors a transfer-line amount edit
// onto the investment-side counterpart row. The caller supplies the new
// signed amount in the destination's frame of reference (positive = cash
// arriving, negative = cash leaving) — i.e. the inverse of the parent
// split's amount.
//
// The caller is responsible for checking that the row is not reconciled
// before invoking this (use FindTransferCashCounterpart's reconciled
// return). A no-op if the new amount already matches.
func (s *Service) UpdateTransferCashCounterpartAmount(rowID types.ID, newAmount types.Money) error {
	row, err := s.repo.GetByID(rowID)
	if err != nil {
		return fmt.Errorf("failed to load investment-side counterpart: %w", err)
	}
	if row.TotalAmount.Equal(newAmount) {
		return nil
	}
	row.TotalAmount = newAmount
	if err := s.repo.Update(row); err != nil {
		return fmt.Errorf("failed to update investment-side counterpart amount: %w", err)
	}
	return nil
}

// ShareTransferResult contains both sides of a share transfer between two investment accounts.
type ShareTransferResult struct {
	SourceTransaction      *Transaction
	DestinationTransaction *Transaction
	TransferID             types.ID
}

// TransferShares transfers shares of a security between two investment accounts.
// The source position is reduced and the destination position is increased with the same cost basis.
// No cash movement occurs in either account.
// Both accounts must be investment accounts and must be different.
// For lot-tracking source accounts, lotAllocations specifies which lots to transfer from.
// For lot-tracking destination accounts, new lots are created preserving original purchase_date and cost_per_share.
func (s *Service) TransferShares(
	sourceAccountID, destAccountID types.ID,
	securityID types.ID,
	date types.Date,
	shares types.Quantity,
	memo string,
	lotAllocations []SellLotAllocation,
) (*ShareTransferResult, error) {
	if !shares.IsPositive() {
		return nil, fmt.Errorf("shares must be positive, got %s", shares)
	}

	if err := s.healInOwnTx(sourceAccountID, securityID); err != nil {
		return nil, err
	}
	if err := s.healInOwnTx(destAccountID, securityID); err != nil {
		return nil, err
	}

	// Validate both accounts are investment accounts
	srcAcct, err := s.getInvestmentAccount(sourceAccountID)
	if err != nil {
		return nil, err
	}

	dstAcct, err := s.getInvestmentAccount(destAccountID)
	if err != nil {
		return nil, err
	}

	// A share transfer is frozen if either leg is a closed account.
	if srcAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: sourceAccountID.String()}
	}
	if dstAcct.IsClosed() {
		return nil, &account.AccountClosedError{ID: destAccountID.String()}
	}

	// Reject same account
	if sourceAccountID == destAccountID {
		return nil, fmt.Errorf("cannot transfer shares between the same account")
	}

	// Determine cost basis based on source account type
	var totalCostBasis types.Money
	var costPerShare types.Money
	var srcLots []*Lot

	if srcAcct.TrackLots {
		// Lot-tracking source: validate lot allocations before any persistence
		if len(lotAllocations) == 0 {
			return nil, fmt.Errorf("lot allocations required for lot-tracking account")
		}

		totalAllocated := types.ZeroQuantity
		for _, alloc := range lotAllocations {
			totalAllocated = totalAllocated.Add(alloc.Shares)
		}
		if totalAllocated.Cmp(shares) != 0 {
			return nil, &LotAllocationMismatchError{
				Expected: shares,
				Actual:   totalAllocated,
			}
		}

		// Pre-validate all lots
		srcLots = make([]*Lot, len(lotAllocations))
		totalCostBasis = types.ZeroMoney
		for i, alloc := range lotAllocations {
			lot, err := s.lotRepo.GetByID(alloc.LotID)
			if err != nil {
				return nil, &LotNotFoundError{LotID: alloc.LotID.String()}
			}
			if lot.AccountID != sourceAccountID {
				return nil, &LotWrongAccountError{
					LotID:     alloc.LotID.String(),
					AccountID: sourceAccountID.String(),
				}
			}
			if lot.SecurityID != securityID {
				return nil, fmt.Errorf("lot %s is for a different security", alloc.LotID)
			}
			if lot.Closed {
				return nil, fmt.Errorf("lot %s is closed", alloc.LotID)
			}
			if lot.Shares.Cmp(alloc.Shares) < 0 {
				return nil, &LotInsufficientSharesError{
					LotID:     alloc.LotID.String(),
					Available: lot.Shares,
					Requested: alloc.Shares,
				}
			}
			srcLots[i] = lot
			totalCostBasis = totalCostBasis.Add(lot.CostPerShare.Mul(alloc.Shares.Decimal()))
		}
	} else {
		// Non-lot-tracking source: use position average cost
		srcPos, err := s.positionRepo.GetByAccountAndSecurity(sourceAccountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source position: %w", err)
		}

		if srcPos.Shares.Cmp(shares) < 0 {
			return nil, &InsufficientSharesError{
				SecurityID: securityID.String(),
				Available:  srcPos.Shares,
				Requested:  shares,
			}
		}

		costPerShare = srcPos.AverageCostPerShare
		totalCostBasis = costPerShare.Mul(shares.Decimal())
	}

	transferID := types.NewID()

	// Create source transaction (negative total — shares leaving)
	negTotal := totalCostBasis.Neg()
	srcTxn := NewTransactionWithSecurity(sourceAccountID, date, TransactionTypeTransferShares, negTotal, securityID, shares)
	srcTxn.SetTransfer(transferID, destAccountID)
	if memo != "" {
		srcTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(srcTxn); err != nil {
		return nil, err
	}

	// Create destination transaction (positive total — shares arriving)
	dstTxn := NewTransactionWithSecurity(destAccountID, date, TransactionTypeTransferShares, totalCostBasis, securityID, shares)
	dstTxn.SetTransfer(transferID, sourceAccountID)
	if memo != "" {
		dstTxn.SetMemo(memo)
	}

	if err := s.validateTransaction(dstTxn); err != nil {
		return nil, err
	}

	// Persist both transactions and the lot/position changes on each side
	// atomically — either the whole transfer lands or none of it does.
	if err := s.runInTx(func(b *Service) error {
		if err := b.repo.Create(srcTxn); err != nil {
			return fmt.Errorf("failed to create source transfer transaction: %w", err)
		}

		if err := b.repo.Create(dstTxn); err != nil {
			return fmt.Errorf("failed to create destination transfer transaction: %w", err)
		}

		// Update source side
		if srcAcct.TrackLots {
			// Reduce source lots and create junction records
			for i, alloc := range lotAllocations {
				lot := srcLots[i]
				if err := lot.Reduce(alloc.Shares); err != nil {
					return fmt.Errorf("failed to reduce lot: %w", err)
				}
				if err := b.lotRepo.Update(lot); err != nil {
					return fmt.Errorf("failed to update lot: %w", err)
				}

				tl := NewTransactionLot(srcTxn.ID, lot.ID, alloc.Shares)
				if err := b.transactionLotRepo.Create(&tl); err != nil {
					return fmt.Errorf("failed to create transaction lot: %w", err)
				}
			}
		} else {
			// Non-lot-tracking source: reduce position
			srcPos, err := b.positionRepo.GetByAccountAndSecurity(sourceAccountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get source position: %w", err)
			}

			if err := srcPos.RemoveShares(shares); err != nil {
				return fmt.Errorf("failed to reduce source position: %w", err)
			}

			if srcPos.Shares.IsZero() {
				if err := b.positionRepo.Delete(sourceAccountID, securityID); err != nil {
					return fmt.Errorf("failed to delete zero source position: %w", err)
				}
			} else {
				if err := b.positionRepo.CreateOrUpdate(srcPos); err != nil {
					return fmt.Errorf("failed to save source position: %w", err)
				}
			}
		}

		// Update destination side
		if dstAcct.TrackLots {
			// Create new lots in destination preserving original purchase_date and cost_per_share
			for i, alloc := range lotAllocations {
				srcLot := srcLots[i]
				newLot := NewLot(destAccountID, securityID, alloc.Shares, srcLot.CostPerShare, srcLot.PurchaseDate, dstTxn.ID)
				if err := b.lotRepo.Create(&newLot); err != nil {
					return fmt.Errorf("failed to create destination lot: %w", err)
				}
			}
		} else {
			// Non-lot-tracking destination: update position
			dstPos, err := b.positionRepo.GetByAccountAndSecurity(destAccountID, securityID)
			if err != nil {
				return fmt.Errorf("failed to get destination position: %w", err)
			}

			if srcAcct.TrackLots {
				// Cost per share is the weighted average from lot allocations
				reciprocal := alpacadecimal.NewFromInt(1).Div(shares.Decimal())
				costPerShare = totalCostBasis.Mul(reciprocal)
			}

			if err := dstPos.AddShares(shares, costPerShare); err != nil {
				return fmt.Errorf("failed to update destination position: %w", err)
			}

			if err := b.positionRepo.CreateOrUpdate(dstPos); err != nil {
				return fmt.Errorf("failed to save destination position: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return &ShareTransferResult{
		SourceTransaction:      srcTxn,
		DestinationTransaction: dstTxn,
		TransferID:             transferID,
	}, nil
}

// DeleteTransaction deletes an investment transaction and cascades to its
// paired counterpart when the transaction is part of a transfer:
//   - transfer_cash: also deletes the paired regular-side row(s) in the
//     transactions table (linked by transfer_id).
//   - transfer_shares: also deletes the paired investment-side row in the
//     other investment account (also linked by transfer_id).
//
// Without this cascade the user is left with an orphaned counterpart that
// still has transfer_id set, which is what happened to the savings-side
// row when the wrong-direction cash transfer was deleted from the
// investment register.
//
// Non-transfer transactions are simply forwarded to repo.Delete.
func (s *Service) DeleteTransaction(id types.ID) error {
	txn, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("failed to load transaction for delete: %w", err)
	}

	// Exchange rows are created and unwound exclusively by the corporate-action
	// engine (mergers/spin-offs); deleting one directly would desync the
	// action's lots/positions from cost basis. Refuse and point at the proper
	// reversal path. (CA reversal deletes these via the raw repository, not here.)
	if txn.Type == TransactionTypeExchange {
		return fmt.Errorf("cannot delete a corporate-action exchange transaction directly; reverse the corporate action from the Corporate Action History view instead")
	}

	// A closed account is frozen — refuse the delete BEFORE any destructive
	// reversal runs. For transfers, block if either leg is a closed account.
	if err := s.ensureAccountOpen(txn.AccountID); err != nil {
		return err
	}
	if txn.TransferAccountID.Valid {
		if err := s.ensureAccountOpen(txn.TransferAccountID.ID); err != nil {
			return err
		}
	}

	// Reverse this transaction's effect on positions and lots BEFORE deleting
	// any rows: Repository.Delete cascades the junction rows that
	// reverseShareRemoval reads, so the reversal must run first (see
	// reverseTxnEffects' contract). This mirrors the reverse-then-apply the
	// Update* methods use — it restores a deleted sell's consumed lots, removes
	// the orphan lot a deleted buy/reinvest opened, and reverses non-lot
	// positions. Cash-only types reverse to a no-op. The reversal, the
	// counterpart cascade, and the row delete commit atomically.
	return s.runInTx(func(b *Service) error {
		if err := b.reverseTxnEffects(txn); err != nil {
			return fmt.Errorf("failed to reverse transaction effects before delete: %w", err)
		}

		if txn.TransferID.Valid {
			switch txn.Type {
			case TransactionTypeTransferCash:
				if b.txnRepo != nil {
					regs, lerr := b.txnRepo.ListByTransferID(txn.TransferID.ID)
					if lerr != nil {
						return fmt.Errorf("failed to find regular-side transfer rows: %w", lerr)
					}
					for _, r := range regs {
						if err := b.txnRepo.Delete(r.ID); err != nil {
							return fmt.Errorf("failed to delete regular-side transfer row: %w", err)
						}
					}
				}
				// inv↔inv cash transfers store the counterpart in the OTHER
				// investment account (no regular-side row exists). Cascade in the
				// investment repo the same way TransferShares does.
				if txn.TransferAccountID.Valid {
					others, lerr := b.repo.ListByAccount(txn.TransferAccountID.ID, TransactionFilter{})
					if lerr != nil {
						return fmt.Errorf("failed to list destination-account transfers: %w", lerr)
					}
					for _, o := range others {
						if o.TransferID.Valid && o.TransferID.ID == txn.TransferID.ID && o.ID != txn.ID {
							if err := b.repo.Delete(o.ID); err != nil {
								return fmt.Errorf("failed to delete paired investment cash-transfer row: %w", err)
							}
						}
					}
				}
			case TransactionTypeTransferShares:
				// The counterpart lives in the other investment account; find by transfer_id.
				others, lerr := b.repo.ListByAccount(txn.TransferAccountID.ID, TransactionFilter{})
				if lerr != nil {
					return fmt.Errorf("failed to list destination-account transfers: %w", lerr)
				}
				for _, o := range others {
					if o.TransferID.Valid && o.TransferID.ID == txn.TransferID.ID && o.ID != txn.ID {
						// Reverse the counterpart's share effect (restore source
						// lots / remove the dest lot) before its row + junctions are
						// cascaded away.
						if err := b.reverseTxnEffects(o); err != nil {
							return fmt.Errorf("failed to reverse paired share-transfer effects: %w", err)
						}
						if err := b.repo.Delete(o.ID); err != nil {
							return fmt.Errorf("failed to delete paired share-transfer row: %w", err)
						}
					}
				}
			}
		}

		if err := b.repo.Delete(id); err != nil {
			return fmt.Errorf("failed to delete investment transaction: %w", err)
		}
		// Reconcile the auto-price this transaction may have seeded: drop it if
		// the delete orphaned it, or re-point it to a surviving same-day
		// transaction.
		if txn.SecurityID.Valid && txn.Type.CreatesAutoPrice() {
			b.cleanupAutoPrice(txn.SecurityID.ID, txn.Date)
		}
		return nil
	})
}

// AccountShares is a per-account share total for a security, used by
// preview surfaces (e.g. the stock-split dialog) to show what a holding
// would look like before and after an action is applied.
type AccountShares struct {
	AccountID   types.ID
	AccountName string
	Shares      types.Quantity
}

// SharesBySecurity returns the share total each account currently holds
// for the given security. Lot-tracking accounts contribute the sum of
// their open lot shares; non-lot-tracking accounts contribute their
// stored position shares. Accounts with zero shares are omitted. Results
// are sorted by account name.
func (s *Service) SharesBySecurity(securityID types.ID) ([]AccountShares, error) {
	totals := make(map[types.ID]types.Quantity)

	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load lots: %w", err)
	}
	lotAccounts := make(map[types.ID]bool)
	for _, lot := range lots {
		totals[lot.AccountID] = totals[lot.AccountID].Add(lot.Shares)
		lotAccounts[lot.AccountID] = true
	}

	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load positions: %w", err)
	}
	for _, pos := range positions {
		// A lot-tracking account also carries an aggregate position row (a
		// derived cache); its shares already came from the lots above, so adding
		// the row too would double-count. Use the position only for non-lot
		// accounts.
		if lotAccounts[pos.AccountID] {
			continue
		}
		totals[pos.AccountID] = totals[pos.AccountID].Add(pos.Shares)
	}

	results := make([]AccountShares, 0, len(totals))
	for acctID, shares := range totals {
		if shares.IsZero() {
			continue
		}
		acct, err := s.accountRepo.GetByID(acctID)
		if err != nil {
			return nil, fmt.Errorf("failed to load account %s: %w", acctID.String(), err)
		}
		results = append(results, AccountShares{
			AccountID:   acctID,
			AccountName: acct.Name,
			Shares:      shares,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].AccountName < results[j].AccountName
	})
	return results, nil
}

// SharesBySecurityAsOf returns, per account, the shares of a security that were
// held on or before asOf — i.e. the shares a split dated asOf would actually
// adjust. Lot-tracking accounts contribute open lots purchased on/before asOf;
// non-lot accounts contribute their net ledger position as of asOf. Accounts
// holding nothing as of that date are omitted. Used by the stock-split preview
// so it reflects the date-scoped engine rather than naively scaling everything.
func (s *Service) SharesBySecurityAsOf(securityID types.ID, asOf types.Date) ([]AccountShares, error) {
	totals := make(map[types.ID]types.Quantity)
	lotAccounts := make(map[types.ID]bool)

	lots, err := s.lotRepo.GetOpenLotsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load lots: %w", err)
	}
	for _, lot := range lots {
		lotAccounts[lot.AccountID] = true
		if lot.PurchaseDate.Time().After(asOf.Time()) {
			continue // acquired after the date — not affected by a split then
		}
		totals[lot.AccountID] = totals[lot.AccountID].Add(lot.Shares)
	}

	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load positions: %w", err)
	}
	for _, pos := range positions {
		if lotAccounts[pos.AccountID] {
			continue // lot-tracking account, handled via lots above
		}
		held, err := netSharesHeldAsOf(s.repo, pos.AccountID, securityID, asOf)
		if err != nil {
			return nil, err
		}
		if !held.IsZero() && !held.IsNegative() {
			totals[pos.AccountID] = held
		}
	}

	results := make([]AccountShares, 0, len(totals))
	for acctID, shares := range totals {
		if shares.IsZero() {
			continue
		}
		acct, err := s.accountRepo.GetByID(acctID)
		if err != nil {
			return nil, fmt.Errorf("failed to load account %s: %w", acctID.String(), err)
		}
		results = append(results, AccountShares{AccountID: acctID, AccountName: acct.Name, Shares: shares})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].AccountName < results[j].AccountName
	})
	return results, nil
}

// netSharesHeldAsOf replays an account's ledger for a security and returns the
// net share count held on or before asOf.
func netSharesHeldAsOf(repo *Repository, accountID, securityID types.ID, asOf types.Date) (types.Quantity, error) {
	filter := TransactionFilter{SecurityID: &securityID, ToDate: &asOf}
	txns, err := repo.ListByAccount(accountID, filter)
	if err != nil {
		return types.ZeroQuantity, err
	}
	net := types.ZeroQuantity
	for _, t := range txns {
		if !t.Shares.Valid {
			continue
		}
		switch t.Type {
		case TransactionTypeBuy, TransactionTypeReinvestDividend:
			net = net.Add(t.Shares.Quantity)
		case TransactionTypeSell, TransactionTypeFeeLiquidation:
			net = net.Sub(t.Shares.Quantity)
		case TransactionTypeTransferShares:
			if t.TotalAmount.IsNegative() {
				net = net.Sub(t.Shares.Quantity)
			} else {
				net = net.Add(t.Shares.Quantity)
			}
		}
	}
	return net, nil
}

// InvalidTransferAmountError is returned when a transfer amount is invalid (not positive).
type InvalidTransferAmountError struct {
	Amount types.Money
}

func (e *InvalidTransferAmountError) Error() string {
	return fmt.Sprintf("transfer amount must be positive, got %s", e.Amount.String())
}

// NotRegularAccountError is returned when a cash transfer targets an investment account instead of a regular one.
type NotRegularAccountError struct {
	AccountID string
	Type      string
}

func (e *NotRegularAccountError) Error() string {
	return fmt.Sprintf("account %s is not a regular account (type: %s); use transfer between investment accounts instead", e.AccountID, e.Type)
}
