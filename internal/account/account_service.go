package account

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// Balance holds the calculated balance information for an account.
type Balance struct {
	AccountID      types.ID
	CurrentBalance types.Money
	ClearedBalance types.Money
}

// Service provides business logic for account operations.
type Service struct {
	repo      *Repository
	db        *db.DB
	invLedger InvestmentLedger
}

// InvestmentLedger reports what an investment account still holds on the
// investment ledger. Close needs it because an investment account's register
// balance is always zero: its cash and shares are rows this package cannot
// read without importing investment, which imports account.
//
// cash is the investment ledger's cash balance, the figure valuation reports
// as CashBalance. hasHoldings is true while any open lot or position still has
// shares, priced or not.
type InvestmentLedger interface {
	LedgerState(accountID types.ID) (cash types.Money, hasHoldings bool, err error)
}

// ServiceOption configures optional dependencies for the Service.
type ServiceOption func(*Service)

// WithInvestmentLedger lets Close judge an investment account by its
// investment ledger. Without it, Close refuses every investment account.
func WithInvestmentLedger(l InvestmentLedger) ServiceOption {
	return func(s *Service) {
		s.invLedger = l
	}
}

// NewService creates a new Service.
func NewService(repo *Repository, database *db.DB, opts ...ServiceOption) *Service {
	s := &Service{
		repo: repo,
		db:   database,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Create validates and creates a new account.
func (s *Service) Create(account *Account) error {
	if err := s.validateAccount(account); err != nil {
		return err
	}
	return s.repo.Create(account)
}

// GetByID retrieves an account by its ID.
func (s *Service) GetByID(id types.ID) (*Account, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves an account by its name.
func (s *Service) GetByName(name string) (*Account, error) {
	return s.repo.GetByName(name)
}

// Update validates and updates an existing account.
//
// A type change that crosses ledgers (regular ⇄ investment) is refused with a
// LedgerChangeError while the ledger the account is leaving still holds rows
// for it. Same-ledger changes (checking → savings, investment →
// hsa_investment) pass through unchanged.
func (s *Service) Update(account *Account) error {
	if err := s.validateAccount(account); err != nil {
		return err
	}
	if err := s.guardLedgerChange(account); err != nil {
		return err
	}
	return s.repo.Update(account)
}

// guardLedgerChange compares the stored type's ledger with the requested one
// and refuses when the departing ledger is not empty.
func (s *Service) guardLedgerChange(account *Account) error {
	before, err := s.repo.GetByID(account.ID)
	if err != nil {
		return err
	}
	fromInv := before.Type.IsInvestmentType()
	if fromInv == account.Type.IsInvestmentType() {
		return nil
	}
	rows, err := s.repo.CountLedgerRows(account.ID, fromInv)
	if err != nil {
		return err
	}
	if rows > 0 {
		return &LedgerChangeError{
			AccountID: account.ID.String(),
			From:      before.Type,
			To:        account.Type,
			Rows:      rows,
		}
	}
	return nil
}

// Delete removes an account. The account must have no transactions and no
// scheduled transactions referencing it.
func (s *Service) Delete(id types.ID) error {
	return s.repo.Delete(id)
}

// List returns all accounts, optionally filtered to active accounts only.
func (s *Service) List(activeOnly bool) ([]*Account, error) {
	return s.repo.List(activeOnly)
}

// BalanceAsOf returns the account's signed balance as of the given date
// (opening balance + non-void transactions dated on or before asOf, parent
// amounts only). Liabilities are negative. It is the single-account form of the
// net-worth as-of formula; the loan wizard's Edit-as-loan flow uses it to read
// the loan's live balance when it rebuilds the month-one snapshot.
func (s *Service) BalanceAsOf(id types.ID, asOf types.Date) (types.Money, error) {
	return s.repo.BalanceAsOf(id, asOf)
}

// GetBalance calculates and returns the balance information for an account.
func (s *Service) GetBalance(id types.ID) (*Balance, error) {
	// Use the account_balances view to get calculated balances
	query := `
		SELECT current_balance, cleared_balance
		FROM account_balances
		WHERE CAST(id AS VARCHAR) = ?
	`

	var currentBalance, clearedBalance types.Money
	err := s.db.Conn().QueryRow(query, id.String()).Scan(&currentBalance, &clearedBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balance: %w", err)
	}

	return &Balance{
		AccountID:      id,
		CurrentBalance: currentBalance,
		ClearedBalance: clearedBalance,
	}, nil
}

// GetAllBalances returns balance information for all accounts.
func (s *Service) GetAllBalances() (map[types.ID]*Balance, error) {
	query := `
		SELECT id, current_balance, cleared_balance
		FROM account_balances
	`

	rows, err := s.db.Conn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get account balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[types.ID]*Balance)
	for rows.Next() {
		var id types.ID
		var currentBalance, clearedBalance types.Money
		if err := rows.Scan(&id, &currentBalance, &clearedBalance); err != nil {
			return nil, fmt.Errorf("failed to scan account balance: %w", err)
		}
		balances[id] = &Balance{
			AccountID:      id,
			CurrentBalance: currentBalance,
			ClearedBalance: clearedBalance,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating account balances: %w", err)
	}

	return balances, nil
}

// Close closes an account after validating it can be closed.
// An account can only be closed if it is empty — a zero register balance, or
// for an investment account zero cash and no shares on the investment ledger —
// and closedDate must fall within [max(opening_date, latest transaction date on
// either ledger), today].
func (s *Service) Close(id types.ID, closedDate types.Date) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if !account.Active {
		return &AlreadyClosedError{ID: id.String()}
	}

	if err := s.requireEmpty(account); err != nil {
		return err
	}

	if err := s.validateCloseDate(account, closedDate); err != nil {
		return err
	}

	account.Close(closedDate)
	return s.repo.Update(account)
}

// requireEmpty returns a HasBalanceError unless the account holds nothing.
// A closed account is frozen, so closing one that still holds cash or shares
// strands them until it is reopened.
func (s *Service) requireEmpty(account *Account) error {
	if !account.Type.IsInvestmentType() {
		balance, err := s.GetBalance(account.ID)
		if err != nil {
			return err
		}
		if !balance.CurrentBalance.IsZero() {
			return &HasBalanceError{ID: account.ID.String(), Balance: balance.CurrentBalance}
		}
		return nil
	}

	if s.invLedger == nil {
		return fmt.Errorf("cannot close investment account %s: %w", account.ID, ErrNoInvestmentLedger)
	}
	cash, held, err := s.invLedger.LedgerState(account.ID)
	if err != nil {
		return fmt.Errorf("failed to read investment ledger for close: %w", err)
	}
	if !cash.IsZero() || held {
		return &HasBalanceError{ID: account.ID.String(), Balance: cash, HoldsShares: held}
	}
	return nil
}

// validateCloseDate enforces max(opening_date, latest_txn_date) <= closedDate <= today.
func (s *Service) validateCloseDate(account *Account, closedDate types.Date) error {
	today := types.Today()
	earliest := account.OpeningDate

	latest, err := s.latestTransactionDate(account.ID)
	if err != nil {
		return err
	}
	if latest.Valid && latest.Date.After(earliest) {
		earliest = latest.Date
	}

	if closedDate.Before(earliest) || closedDate.After(today) {
		return &InvalidCloseDateError{
			ID:       account.ID.String(),
			Date:     closedDate,
			Earliest: earliest,
			Today:    today,
		}
	}
	return nil
}

// latestTransactionDate returns the most recent transaction date for an
// account on either ledger, or an invalid NullableDate when it has none. An
// account keeps rows on one ledger at a time, but reading both means a close
// can never be dated before the account's last row.
func (s *Service) latestTransactionDate(id types.ID) (types.NullableDate, error) {
	var d types.NullableDate
	err := s.db.Conn().QueryRow(
		`SELECT MAX(date) FROM (
			SELECT date FROM transactions WHERE CAST(account_id AS VARCHAR) = ?
			UNION ALL
			SELECT date FROM investment_transactions WHERE CAST(account_id AS VARCHAR) = ?
		)`,
		id.String(), id.String(),
	).Scan(&d)
	if err != nil {
		return types.NullableDate{}, fmt.Errorf("failed to get latest transaction date: %w", err)
	}
	return d, nil
}

// Reopen reopens a closed account, clearing its close date.
func (s *Service) Reopen(id types.ID) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if account.Active {
		return &NotClosedError{ID: id.String()}
	}

	account.Reopen()
	return s.repo.Update(account)
}

// RestoreClosed re-closes an account to an exact prior state, bypassing the
// zero-balance and close-date validation that Close enforces. It exists so undo
// can faithfully restore a previously-closed account (including one whose close
// date is NULL, e.g. a pre-existing closed account backfilled by migration 025)
// without a backfilled-NULL date or a since-changed balance tripping validation.
func (s *Service) RestoreClosed(id types.ID, closedDate types.NullableDate) error {
	account, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	account.Active = false
	account.ClosedDate = closedDate
	return s.repo.Update(account)
}

// validateAccount validates an account and returns any validation errors.
func (s *Service) validateAccount(account *Account) error {
	errors := account.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}
