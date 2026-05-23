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

	// Heal any stale stored position/lot state for this (account, security)
	// before we read it (no-op when corporate actions are present).
	if err := s.syncPositionAndLots(accountID, securityID); err != nil {
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

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create buy transaction: %w", err)
	}

	// Update position or create lot based on account tracking mode
	if acct.TrackLots {
		lot := NewLot(accountID, securityID, shares, computed.PricePerShare, date, txn.ID)
		if err := s.lotRepo.Create(&lot); err != nil {
			return nil, fmt.Errorf("failed to create lot: %w", err)
		}
	} else {
		pos, err := s.positionRepo.GetByAccountAndSecurity(accountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get position: %w", err)
		}
		if err := pos.AddShares(shares, computed.PricePerShare); err != nil {
			return nil, fmt.Errorf("failed to update position: %w", err)
		}
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return nil, fmt.Errorf("failed to save position: %w", err)
		}
	}

	// Auto-create price record from transaction
	s.autoCreatePrice(securityID, date, computed.PricePerShare)

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

	// Heal any stale stored position/lot state before validating.
	if err := s.syncPositionAndLots(accountID, securityID); err != nil {
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

	if acct.TrackLots {
		if err := s.sellWithLots(txn, accountID, securityID, shares, lotAllocations); err != nil {
			return nil, err
		}
	} else {
		if err := s.sellWithPosition(txn, accountID, securityID, shares); err != nil {
			return nil, err
		}
	}

	// Auto-create price record from transaction
	s.autoCreatePrice(securityID, date, computed.PricePerShare)

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

// requireInvestmentAccount verifies that the given account exists and is an investment account.
func (s *Service) requireInvestmentAccount(accountID types.ID) error {
	_, err := s.getInvestmentAccount(accountID)
	return err
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

	if err := s.syncPositionAndLots(accountID, securityID); err != nil {
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

	if err := s.repo.Create(txn); err != nil {
		return nil, fmt.Errorf("failed to create reinvest dividend transaction: %w", err)
	}

	// Update position or create lot based on account tracking mode
	if acct.TrackLots {
		lot := NewLot(accountID, securityID, shares, computed.PricePerShare, date, txn.ID)
		if err := s.lotRepo.Create(&lot); err != nil {
			return nil, fmt.Errorf("failed to create lot: %w", err)
		}
	} else {
		pos, err := s.positionRepo.GetByAccountAndSecurity(accountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get position: %w", err)
		}
		if err := pos.AddShares(shares, computed.PricePerShare); err != nil {
			return nil, fmt.Errorf("failed to update position: %w", err)
		}
		if err := s.positionRepo.CreateOrUpdate(pos); err != nil {
			return nil, fmt.Errorf("failed to save position: %w", err)
		}
	}

	// Auto-create price record from transaction
	s.autoCreatePrice(securityID, date, computed.PricePerShare)

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

	if err := s.syncPositionAndLots(accountID, securityID); err != nil {
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

	if acct.TrackLots {
		if err := s.feeLiquidationWithLots(txn, accountID, securityID, shares, lotAllocations); err != nil {
			return nil, err
		}
	} else {
		if err := s.feeLiquidationWithPosition(txn, accountID, securityID, shares); err != nil {
			return nil, err
		}
	}

	// Auto-create price record from transaction
	s.autoCreatePrice(securityID, date, computed.PricePerShare)

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

// CashTransferResult contains both sides of a cash transfer between
// an investment account and a regular account.
type CashTransferResult struct {
	InvestmentTransaction *Transaction
	RegularTransaction    *transaction.Transaction
	TransferID            types.ID
}

// TransferCash creates a cash transfer between an investment account and a regular (non-investment) account.
// When direction is "in" (deposit to investment), the regular account is debited and the investment account is credited.
// When direction is "out" (withdrawal from investment), the investment account is debited and the regular account is credited.
// Both transactions are linked by a shared transfer_id.
func (s *Service) TransferCash(investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string) (*CashTransferResult, error) {
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

	// Check that the two accounts are different
	if investmentAccountID == regularAccountID {
		return nil, fmt.Errorf("cannot transfer between the same account")
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

	if err := s.repo.Create(invTxn); err != nil {
		return nil, fmt.Errorf("failed to create investment transfer transaction: %w", err)
	}

	// Create regular transaction (deposit — positive amount)
	regTxn := transaction.NewTransaction(regularAccountID, date, amount)
	regTxn.SetTransfer(transferID, investmentAccountID)
	if memo != "" {
		regTxn.SetMemo(memo)
	}

	if err := s.txnRepo.Create(regTxn); err != nil {
		// Cleanup investment side on failure
		_ = s.repo.Delete(invTxn.ID)
		return nil, fmt.Errorf("failed to create regular transfer transaction: %w", err)
	}

	return &CashTransferResult{
		InvestmentTransaction: invTxn,
		RegularTransaction:    regTxn,
		TransferID:            transferID,
	}, nil
}

// DepositFromAccount transfers cash from a regular account into an investment account.
// This creates a deposit in the investment account and a withdrawal in the regular account.
func (s *Service) DepositFromAccount(investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string) (*CashTransferResult, error) {
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

	// Check that the two accounts are different
	if investmentAccountID == regularAccountID {
		return nil, fmt.Errorf("cannot transfer between the same account")
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

	if err := s.repo.Create(invTxn); err != nil {
		return nil, fmt.Errorf("failed to create investment transfer transaction: %w", err)
	}

	// Create regular transaction (withdrawal — negative amount)
	negAmount := amount.Neg()
	regTxn := transaction.NewTransaction(regularAccountID, date, negAmount)
	regTxn.SetTransfer(transferID, investmentAccountID)
	if memo != "" {
		regTxn.SetMemo(memo)
	}

	if err := s.txnRepo.Create(regTxn); err != nil {
		// Cleanup investment side on failure
		_ = s.repo.Delete(invTxn.ID)
		return nil, fmt.Errorf("failed to create regular transfer transaction: %w", err)
	}

	return &CashTransferResult{
		InvestmentTransaction: invTxn,
		RegularTransaction:    regTxn,
		TransferID:            transferID,
	}, nil
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

	if err := s.syncPositionAndLots(sourceAccountID, securityID); err != nil {
		return nil, err
	}
	if err := s.syncPositionAndLots(destAccountID, securityID); err != nil {
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

	// Persist source transaction
	if err := s.repo.Create(srcTxn); err != nil {
		return nil, fmt.Errorf("failed to create source transfer transaction: %w", err)
	}

	// Persist destination transaction
	if err := s.repo.Create(dstTxn); err != nil {
		_ = s.repo.Delete(srcTxn.ID)
		return nil, fmt.Errorf("failed to create destination transfer transaction: %w", err)
	}

	// Update source side
	if srcAcct.TrackLots {
		// Reduce source lots and create junction records
		for i, alloc := range lotAllocations {
			lot := srcLots[i]
			if err := lot.Reduce(alloc.Shares); err != nil {
				return nil, fmt.Errorf("failed to reduce lot: %w", err)
			}
			if err := s.lotRepo.Update(lot); err != nil {
				return nil, fmt.Errorf("failed to update lot: %w", err)
			}

			tl := NewTransactionLot(srcTxn.ID, lot.ID, alloc.Shares)
			if err := s.transactionLotRepo.Create(&tl); err != nil {
				return nil, fmt.Errorf("failed to create transaction lot: %w", err)
			}
		}
	} else {
		// Non-lot-tracking source: reduce position
		srcPos, err := s.positionRepo.GetByAccountAndSecurity(sourceAccountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get source position: %w", err)
		}

		if err := srcPos.RemoveShares(shares); err != nil {
			return nil, fmt.Errorf("failed to reduce source position: %w", err)
		}

		if srcPos.Shares.IsZero() {
			if err := s.positionRepo.Delete(sourceAccountID, securityID); err != nil {
				return nil, fmt.Errorf("failed to delete zero source position: %w", err)
			}
		} else {
			if err := s.positionRepo.CreateOrUpdate(srcPos); err != nil {
				return nil, fmt.Errorf("failed to save source position: %w", err)
			}
		}
	}

	// Update destination side
	if dstAcct.TrackLots {
		// Create new lots in destination preserving original purchase_date and cost_per_share
		for i, alloc := range lotAllocations {
			srcLot := srcLots[i]
			newLot := NewLot(destAccountID, securityID, alloc.Shares, srcLot.CostPerShare, srcLot.PurchaseDate, dstTxn.ID)
			if err := s.lotRepo.Create(&newLot); err != nil {
				return nil, fmt.Errorf("failed to create destination lot: %w", err)
			}
		}
	} else {
		// Non-lot-tracking destination: update position
		dstPos, err := s.positionRepo.GetByAccountAndSecurity(destAccountID, securityID)
		if err != nil {
			return nil, fmt.Errorf("failed to get destination position: %w", err)
		}

		if srcAcct.TrackLots {
			// Cost per share is the weighted average from lot allocations
			reciprocal := alpacadecimal.NewFromInt(1).Div(shares.Decimal())
			costPerShare = totalCostBasis.Mul(reciprocal)
		}

		if err := dstPos.AddShares(shares, costPerShare); err != nil {
			return nil, fmt.Errorf("failed to update destination position: %w", err)
		}

		if err := s.positionRepo.CreateOrUpdate(dstPos); err != nil {
			return nil, fmt.Errorf("failed to save destination position: %w", err)
		}
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

	if txn.TransferID.Valid {
		switch txn.Type {
		case TransactionTypeTransferCash:
			if s.txnRepo != nil {
				regs, lerr := s.txnRepo.ListByTransferID(txn.TransferID.ID)
				if lerr != nil {
					return fmt.Errorf("failed to find regular-side transfer rows: %w", lerr)
				}
				for _, r := range regs {
					if err := s.txnRepo.Delete(r.ID); err != nil {
						return fmt.Errorf("failed to delete regular-side transfer row: %w", err)
					}
				}
			}
		case TransactionTypeTransferShares:
			// The counterpart lives in the other investment account; find by transfer_id.
			others, lerr := s.repo.ListByAccount(txn.TransferAccountID.ID, TransactionFilter{})
			if lerr != nil {
				return fmt.Errorf("failed to list destination-account transfers: %w", lerr)
			}
			for _, o := range others {
				if o.TransferID.Valid && o.TransferID.ID == txn.TransferID.ID && o.ID != txn.ID {
					if err := s.repo.Delete(o.ID); err != nil {
						return fmt.Errorf("failed to delete paired share-transfer row: %w", err)
					}
				}
			}
		}
	}

	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("failed to delete investment transaction: %w", err)
	}
	return nil
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
	for _, lot := range lots {
		totals[lot.AccountID] = totals[lot.AccountID].Add(lot.Shares)
	}

	positions, err := s.positionRepo.GetPositionsBySecurity(securityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load positions: %w", err)
	}
	for _, pos := range positions {
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
