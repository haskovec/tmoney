package service

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// PayeeService provides business logic for payee operations.
type PayeeService struct {
	repo *repository.PayeeRepository
	db   *db.DB
}

// NewPayeeService creates a new PayeeService.
func NewPayeeService(repo *repository.PayeeRepository, database *db.DB) *PayeeService {
	return &PayeeService{
		repo: repo,
		db:   database,
	}
}

// Create validates and creates a new payee.
func (s *PayeeService) Create(payee *models.Payee) error {
	if err := s.validatePayee(payee); err != nil {
		return err
	}
	return s.repo.Create(payee)
}

// GetByID retrieves a payee by its ID.
func (s *PayeeService) GetByID(id models.ID) (*models.Payee, error) {
	return s.repo.GetByID(id)
}

// GetByName retrieves a payee by its name.
func (s *PayeeService) GetByName(name string) (*models.Payee, error) {
	return s.repo.GetByName(name)
}

// Update validates and updates an existing payee.
func (s *PayeeService) Update(payee *models.Payee) error {
	if err := s.validatePayee(payee); err != nil {
		return err
	}
	return s.repo.Update(payee)
}

// Delete removes a payee. The payee must have no transactions.
func (s *PayeeService) Delete(id models.ID) error {
	return s.repo.Delete(id)
}

// List returns all payees ordered by name.
func (s *PayeeService) List() ([]*models.Payee, error) {
	return s.repo.List()
}

// =============================================================================
// Auto-create Operations
// =============================================================================

// GetOrCreate retrieves an existing payee by name, or creates a new one if not found.
// This is the primary method for auto-creating payees when adding transactions.
// If the payee is created, it will have no default category.
func (s *PayeeService) GetOrCreate(name string) (*models.Payee, bool, error) {
	// Try to find existing payee
	payee, err := s.repo.GetByName(name)
	if err == nil {
		return payee, false, nil
	}

	// Check if error is "not found" - that's expected for auto-create
	if _, ok := err.(*repository.NotFoundError); !ok {
		return nil, false, fmt.Errorf("failed to check existing payee: %w", err)
	}

	// Create new payee
	payee = models.NewPayee(name)
	if err := s.repo.Create(payee); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return payee, true, nil
}

// GetOrCreateWithCategory retrieves an existing payee by name, or creates a new one
// with the specified default category if not found.
func (s *PayeeService) GetOrCreateWithCategory(name string, categoryID models.ID) (*models.Payee, bool, error) {
	// Try to find existing payee
	payee, err := s.repo.GetByName(name)
	if err == nil {
		return payee, false, nil
	}

	// Check if error is "not found" - that's expected for auto-create
	if _, ok := err.(*repository.NotFoundError); !ok {
		return nil, false, fmt.Errorf("failed to check existing payee: %w", err)
	}

	// Create new payee with category
	payee = models.NewPayeeWithCategory(name, categoryID)
	if err := s.repo.Create(payee); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return payee, true, nil
}

// =============================================================================
// Alias Matching Operations
// =============================================================================

// FindPayeeByPattern searches all aliases for a match against the input string.
// Returns the matching payee, or nil if no match is found.
// Matching is case-insensitive.
// This method is useful for import scenarios where payee names may not match exactly.
func (s *PayeeService) FindPayeeByPattern(input string) (*models.Payee, error) {
	return s.repo.FindPayeeByPattern(input)
}

// ResolvePayee attempts to find a payee by:
// 1. Exact name match
// 2. Alias pattern match
// If no match is found, returns nil (caller can then use GetOrCreate if desired).
func (s *PayeeService) ResolvePayee(name string) (*models.Payee, error) {
	// Try exact name match first
	payee, err := s.repo.GetByName(name)
	if err == nil {
		return payee, nil
	}

	// Check if error is not "not found" - that's a real error
	if _, ok := err.(*repository.NotFoundError); !ok {
		return nil, fmt.Errorf("failed to find payee by name: %w", err)
	}

	// Try alias pattern match
	payee, err = s.repo.FindPayeeByPattern(name)
	if err != nil {
		return nil, fmt.Errorf("failed to find payee by pattern: %w", err)
	}

	return payee, nil
}

// ResolveOrCreate attempts to find a payee by name or alias pattern.
// If not found, creates a new payee with the given name.
// Returns the payee and whether it was newly created.
func (s *PayeeService) ResolveOrCreate(name string) (*models.Payee, bool, error) {
	// Try to resolve (by name or alias)
	payee, err := s.ResolvePayee(name)
	if err != nil {
		return nil, false, err
	}
	if payee != nil {
		return payee, false, nil
	}

	// Create new payee
	payee = models.NewPayee(name)
	if err := s.repo.Create(payee); err != nil {
		return nil, false, fmt.Errorf("failed to create payee: %w", err)
	}

	return payee, true, nil
}

// =============================================================================
// Default Category Operations
// =============================================================================

// SetDefaultCategory sets the default category for a payee.
func (s *PayeeService) SetDefaultCategory(payeeID, categoryID models.ID) error {
	payee, err := s.repo.GetByID(payeeID)
	if err != nil {
		return err
	}

	payee.SetDefaultCategory(categoryID)
	return s.repo.Update(payee)
}

// ClearDefaultCategory removes the default category from a payee.
func (s *PayeeService) ClearDefaultCategory(payeeID models.ID) error {
	payee, err := s.repo.GetByID(payeeID)
	if err != nil {
		return err
	}

	payee.ClearDefaultCategory()
	return s.repo.Update(payee)
}

// GetDefaultCategory returns the default category for a payee, or nil if not set.
func (s *PayeeService) GetDefaultCategory(payeeID models.ID) (*models.ID, error) {
	payee, err := s.repo.GetByID(payeeID)
	if err != nil {
		return nil, err
	}

	if !payee.HasDefaultCategory() {
		return nil, nil
	}

	return &payee.DefaultCategoryID.ID, nil
}

// =============================================================================
// Alias Management Operations
// =============================================================================

// CreateAlias creates a new alias for a payee.
func (s *PayeeService) CreateAlias(alias *models.Alias) error {
	if err := s.validateAlias(alias); err != nil {
		return err
	}
	return s.repo.CreateAlias(alias)
}

// GetAliasByID retrieves an alias by its ID.
func (s *PayeeService) GetAliasByID(id models.ID) (*models.Alias, error) {
	return s.repo.GetAliasByID(id)
}

// GetAliasesByPayee retrieves all aliases for a payee.
func (s *PayeeService) GetAliasesByPayee(payeeID models.ID) ([]*models.Alias, error) {
	return s.repo.GetAliasesByPayee(payeeID)
}

// UpdateAlias updates an existing alias.
func (s *PayeeService) UpdateAlias(alias *models.Alias) error {
	if err := s.validateAlias(alias); err != nil {
		return err
	}
	return s.repo.UpdateAlias(alias)
}

// DeleteAlias removes an alias.
func (s *PayeeService) DeleteAlias(id models.ID) error {
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
func (s *PayeeService) MergePayees(sourceID, targetID models.ID) error {
	// Cannot merge into itself
	if sourceID == targetID {
		return &PayeeMergeSameError{ID: sourceID.String()}
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

	// Update all references to use target payee
	// Note: DuckDB doesn't support transactions, so we do best-effort updates

	// Update transactions
	_, err = s.db.Conn().Exec(`
		UPDATE transactions
		SET payee_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
		WHERE CAST(payee_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update transactions: %w", err)
	}

	// Update scheduled transactions
	_, err = s.db.Conn().Exec(`
		UPDATE scheduled_transactions
		SET payee_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP
		WHERE CAST(payee_id AS VARCHAR) = ?
	`, targetID.String(), sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to update scheduled transactions: %w", err)
	}

	// Reassign aliases from source to target
	aliases, err := s.repo.GetAliasesByPayee(sourceID)
	if err != nil {
		return fmt.Errorf("failed to get source aliases: %w", err)
	}

	for _, alias := range aliases {
		// Create an exact alias for the source payee name so it can still be matched
		// This preserves the historical mapping
		alias.PayeeID = targetID
		if err := s.repo.UpdateAlias(alias); err != nil {
			return fmt.Errorf("failed to reassign alias %s: %w", alias.Pattern, err)
		}
	}

	// Create an exact alias from source name to target (if source name differs)
	if source.Name != "" {
		exactAlias := models.NewExactAlias(targetID, source.Name)
		// Only create if not a duplicate pattern
		if err := s.repo.CreateAlias(exactAlias); err != nil {
			// Ignore duplicate pattern error - the alias might already exist
			if _, ok := err.(*repository.DuplicateError); !ok {
				return fmt.Errorf("failed to create name alias: %w", err)
			}
		}
	}

	// Delete the source payee (should now have no transactions)
	// Force delete by first removing transaction references (already done above)
	_, err = s.db.Conn().Exec(`DELETE FROM payees WHERE CAST(id AS VARCHAR) = ?`, sourceID.String())
	if err != nil {
		return fmt.Errorf("failed to delete source payee: %w", err)
	}

	return nil
}

// =============================================================================
// Validation Helpers
// =============================================================================

// validatePayee validates a payee and returns any validation errors.
func (s *PayeeService) validatePayee(payee *models.Payee) error {
	errors := payee.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}

// validateAlias validates an alias and returns any validation errors.
func (s *PayeeService) validateAlias(alias *models.Alias) error {
	errors := alias.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
