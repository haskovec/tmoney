package payee

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for payee operations.
type Service struct {
	repo *Repository
	db   *db.DB
	tx   db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
func NewService(repo *Repository, database *db.DB) *Service {
	return &Service{
		repo: repo,
		db:   database,
	}
}

// InTx returns a copy of the service bound to tx, with its repository rebound to
// the same transaction so all writes join one atomic unit. The original service
// is unchanged and remains safe for non-transactional use.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.repo = s.repo.WithTx(tx)
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not query
// the pool directly — the transaction pins the single connection
// (SetMaxOpenConns(1)), so a pool read inside a tx deadlocks.
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

// Create validates and creates a new payee.
func (s *Service) Create(payee *Payee) error {
	if err := s.validatePayee(payee); err != nil {
		return err
	}
	return s.repo.Create(payee)
}

// GetByID retrieves a payee by its ID.
func (s *Service) GetByID(id types.ID) (*Payee, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves a payee by its name.
func (s *Service) GetByName(name string) (*Payee, error) {
	return s.repo.GetByName(name)
}

// Update validates and updates an existing payee.
func (s *Service) Update(payee *Payee) error {
	if err := s.validatePayee(payee); err != nil {
		return err
	}
	return s.repo.Update(payee)
}

// Delete removes a payee. The payee must have no transactions.
func (s *Service) Delete(id types.ID) error {
	return s.repo.Delete(id)
}

// List returns all payees ordered by name.
func (s *Service) List() ([]*Payee, error) {
	return s.repo.List()
}

// =============================================================================
// Auto-create Operations
// =============================================================================

// GetOrCreate retrieves an existing payee by name, or creates a new one if not found.
// This is the primary method for auto-creating payees when adding transactions.
// If the payee is created, it will have no default category.
func (s *Service) GetOrCreate(name string) (*Payee, bool, error) {
	// Try to find existing payee
	p, err := s.repo.GetByName(name)
	if err == nil {
		return p, false, nil
	}

	// Check if error is "not found" - that's expected for auto-create
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return nil, false, fmt.Errorf("failed to check existing payee: %w", err)
	}

	// Create new payee
	p = NewPayee(name)
	if err := s.repo.Create(p); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return p, true, nil
}

// GetOrCreateWithCategory retrieves an existing payee by name, or creates a new one
// with the specified default category if not found.
func (s *Service) GetOrCreateWithCategory(name string, categoryID types.ID) (*Payee, bool, error) {
	// Try to find existing payee
	p, err := s.repo.GetByName(name)
	if err == nil {
		return p, false, nil
	}

	// Check if error is "not found" - that's expected for auto-create
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return nil, false, fmt.Errorf("failed to check existing payee: %w", err)
	}

	// Create new payee with category
	p = NewPayeeWithCategory(name, categoryID)
	if err := s.repo.Create(p); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return p, true, nil
}

// =============================================================================
// Alias Matching Operations
// =============================================================================

// FindPayeeByPattern searches all aliases for a match against the input string.
// Returns the matching payee, or nil if no match is found.
// Matching is case-insensitive.
// This method is useful for import scenarios where payee names may not match exactly.
func (s *Service) FindPayeeByPattern(input string) (*Payee, error) {
	return s.repo.FindPayeeByPattern(input)
}

// ResolvePayee attempts to find a payee by:
// 1. Exact name match
// 2. Alias pattern match
// If no match is found, returns nil (caller can then use GetOrCreate if desired).
func (s *Service) ResolvePayee(name string) (*Payee, error) {
	// Try exact name match first
	p, err := s.repo.GetByName(name)
	if err == nil {
		return p, nil
	}

	// Check if error is not "not found" - that's a real error
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		return nil, fmt.Errorf("failed to find payee by name: %w", err)
	}

	// Try alias pattern match
	p, err = s.repo.FindPayeeByPattern(name)
	if err != nil {
		return nil, fmt.Errorf("failed to find payee by pattern: %w", err)
	}

	return p, nil
}

// ResolveOrCreate attempts to find a payee by name or alias pattern.
// If not found, creates a new payee with the given name.
// Returns the payee and whether it was newly created.
func (s *Service) ResolveOrCreate(name string) (*Payee, bool, error) {
	// Try to resolve (by name or alias)
	p, err := s.ResolvePayee(name)
	if err != nil {
		return nil, false, err
	}
	if p != nil {
		return p, false, nil
	}

	// Create new payee
	p = NewPayee(name)
	if err := s.repo.Create(p); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return p, true, nil
}

// =============================================================================
// Default Category Operations
// =============================================================================

// SetDefaultCategory sets the default category for a payee.
func (s *Service) SetDefaultCategory(payeeID, categoryID types.ID) error {
	p, err := s.repo.GetByID(payeeID)
	if err != nil {
		return err
	}

	p.SetDefaultCategory(categoryID)
	return s.repo.Update(p)
}

// ClearDefaultCategory removes the default category from a payee.
func (s *Service) ClearDefaultCategory(payeeID types.ID) error {
	p, err := s.repo.GetByID(payeeID)
	if err != nil {
		return err
	}

	p.ClearDefaultCategory()
	return s.repo.Update(p)
}

// GetDefaultCategory returns the default category for a payee, or nil if not set.
func (s *Service) GetDefaultCategory(payeeID types.ID) (*types.ID, error) {
	p, err := s.repo.GetByID(payeeID)
	if err != nil {
		return nil, err
	}

	if !p.HasDefaultCategory() {
		return nil, nil
	}

	return &p.DefaultCategoryID.ID, nil
}

// =============================================================================
// Alias Management Operations
// =============================================================================

// CreateAlias creates a new alias for a payee.
func (s *Service) CreateAlias(alias *Alias) error {
	if err := s.validateAlias(alias); err != nil {
		return err
	}
	return s.repo.CreateAlias(alias)
}

// GetAliasByID retrieves an alias by its ID.
func (s *Service) GetAliasByID(id types.ID) (*Alias, error) {
	return s.repo.GetAliasByID(id)
}

// GetAliasesByPayee retrieves all aliases for a payee.
func (s *Service) GetAliasesByPayee(payeeID types.ID) ([]*Alias, error) {
	return s.repo.GetAliasesByPayee(payeeID)
}

// UpdateAlias updates an existing alias.
func (s *Service) UpdateAlias(alias *Alias) error {
	if err := s.validateAlias(alias); err != nil {
		return err
	}
	return s.repo.UpdateAlias(alias)
}

// DeleteAlias removes an alias.
func (s *Service) DeleteAlias(id types.ID) error {
	return s.repo.DeleteAlias(id)
}

// =============================================================================
// Merge Operations
// =============================================================================

// MergePayees merges the source payee into the target payee.
// All transactions, scheduled transactions, and aliases using the source payee
// will be updated to use the target payee.
// The source payee is then deleted.
//
// Rules:
// - Cannot merge a payee into itself
func (s *Service) MergePayees(sourceID, targetID types.ID) error {
	// Cannot merge into itself
	if sourceID == targetID {
		return &MergeSameError{ID: sourceID.String()}
	}

	// Get both payees
	source, err := s.repo.GetByID(sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source payee: %w", err)
	}

	_, err = s.repo.GetByID(targetID)
	if err != nil {
		return fmt.Errorf("failed to get target payee: %w", err)
	}

	// Update all references to use target payee.
	// Note: DuckDB UPDATE can trigger primary key violations due to internal
	// delete+re-insert mechanism. Use CREATE TEMP TABLE + DELETE + INSERT pattern.
	// The whole reassignment runs in one transaction: a mid-reassignment failure
	// rolls the entire set back, so references can never be left split between
	// source and target (the "best-effort" hazard this replaces). The temp-table
	// DDL is safe inside an explicit tx on the pinned duckdb-go version (verified
	// by spike), and each temp table is dropped before the tx ends so a re-run
	// finds no stale connection-scoped table.
	//
	// The source-payee delete is deliberately NOT inside this transaction. DuckDB
	// forbids deleting a key in the same transaction that earlier dereferenced it:
	// moving every reference off source and then deleting source in one tx fails
	// with "Violates foreign key constraint because key ... is still referenced"
	// (verified for the temp-table reinsert form too). So the delete runs as a
	// separate trailing statement. The residual non-atomicity is benign: if the
	// delete fails after the reassignment commits, source is left as an orphan
	// payee with zero references, which a re-run of the merge cleans up.
	if err := s.runInTx(func(b *Service) error {
		// Update transactions: reassign payee_id from source to target
		if _, err := b.q().Exec(`
			CREATE TEMPORARY TABLE _merge_txns AS
			SELECT id, account_id, date, amount, CAST(? AS UUID) AS payee_id,
				category_id, memo, check_number, status, transfer_id,
				transfer_account_id, bank_reference_id, created_at, CURRENT_TIMESTAMP AS updated_at
			FROM transactions
			WHERE CAST(payee_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to stage transaction updates: %w", err)
		}

		if _, err := b.q().Exec(`
			DELETE FROM transactions WHERE CAST(payee_id AS VARCHAR) = ?
		`, sourceID.String()); err != nil {
			return fmt.Errorf("failed to delete source transactions for merge: %w", err)
		}

		if _, err := b.q().Exec(`INSERT INTO transactions SELECT * FROM _merge_txns`); err != nil {
			return fmt.Errorf("failed to re-insert merged transactions: %w", err)
		}

		if _, err := b.q().Exec(`DROP TABLE IF EXISTS _merge_txns`); err != nil {
			return fmt.Errorf("failed to drop temp table: %w", err)
		}

		// Update scheduled transactions: reassign payee_id from source to target
		if _, err := b.q().Exec(`
			CREATE TEMPORARY TABLE _merge_st AS
			SELECT id, account_id, CAST(? AS UUID) AS payee_id, category_id,
				amount, memo, frequency, interval, start_date, end_date,
				occurrences, day_of_month, secondary_day_of_month, next_date,
				occurrences_remaining, amount_estimate_count,
				auto_post, post_lead_days,
				created_at, CURRENT_TIMESTAMP AS updated_at,
				transfer_account_id
			FROM scheduled_transactions
			WHERE CAST(payee_id AS VARCHAR) = ?
		`, targetID.String(), sourceID.String()); err != nil {
			return fmt.Errorf("failed to stage scheduled transaction updates: %w", err)
		}

		if _, err := b.q().Exec(`
			DELETE FROM scheduled_transactions WHERE CAST(payee_id AS VARCHAR) = ?
		`, sourceID.String()); err != nil {
			return fmt.Errorf("failed to delete source scheduled transactions for merge: %w", err)
		}

		if _, err := b.q().Exec(`INSERT INTO scheduled_transactions SELECT * FROM _merge_st`); err != nil {
			return fmt.Errorf("failed to re-insert merged scheduled transactions: %w", err)
		}

		if _, err := b.q().Exec(`DROP TABLE IF EXISTS _merge_st`); err != nil {
			return fmt.Errorf("failed to drop temp table: %w", err)
		}

		// Reassign aliases from source to target
		aliases, err := b.repo.GetAliasesByPayee(sourceID)
		if err != nil {
			return fmt.Errorf("failed to get source aliases: %w", err)
		}

		for _, alias := range aliases {
			// Create an exact alias for the source payee name so it can still be matched
			// This preserves the historical mapping
			alias.PayeeID = targetID
			if err := b.repo.UpdateAlias(alias); err != nil {
				return fmt.Errorf("failed to reassign alias %s: %w", alias.Pattern, err)
			}
		}

		// Create an exact alias from source name to target (if source name differs)
		if source.Name != "" {
			exactAlias := NewExactAlias(targetID, source.Name)
			// Only create if not a duplicate pattern
			if err := b.repo.CreateAlias(exactAlias); err != nil {
				// Ignore duplicate pattern error - the alias might already exist
				if _, ok := err.(*dberrors.DuplicateError); !ok {
					return fmt.Errorf("failed to create name alias: %w", err)
				}
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// Delete the source payee — now dereferenced — in its own statement (see the
	// transaction comment above for why it cannot join the reassignment tx).
	if _, err := s.q().Exec(`DELETE FROM payees WHERE CAST(id AS VARCHAR) = ?`, sourceID.String()); err != nil {
		return fmt.Errorf("failed to delete source payee: %w", err)
	}

	return nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validatePayee validates a payee and returns any validation errors.
func (s *Service) validatePayee(payee *Payee) error {
	errors := payee.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateAlias validates an alias and returns any validation errors.
func (s *Service) validateAlias(alias *Alias) error {
	errors := alias.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}
