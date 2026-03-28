package service

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// SecurityService provides business logic for security operations.
type SecurityService struct {
	repo *repository.SecurityRepository
	db   *db.DB
}

// NewSecurityService creates a new SecurityService.
func NewSecurityService(repo *repository.SecurityRepository, database *db.DB) *SecurityService {
	return &SecurityService{
		repo: repo,
		db:   database,
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create validates and creates a new security.
func (s *SecurityService) Create(security *models.Security) error {
	if err := s.validateSecurity(security); err != nil {
		return err
	}
	return s.repo.Create(security)
}

// GetByID retrieves a security by its ID.
func (s *SecurityService) GetByID(id models.ID) (*models.Security, error) {
	return s.repo.GetByID(id)
}

// GetByTicker retrieves a security by its ticker symbol and optional currency.
func (s *SecurityService) GetByTicker(ticker string, currency string) (*models.Security, error) {
	return s.repo.GetByTicker(ticker, currency)
}

// Update validates and updates an existing security.
func (s *SecurityService) Update(security *models.Security) error {
	if err := s.validateSecurity(security); err != nil {
		return err
	}
	return s.repo.Update(security)
}

// Delete removes a security. If the security has prices or transactions,
// returns a SecurityHasDependentsError suggesting hiding instead.
func (s *SecurityService) Delete(id models.ID) error {
	err := s.repo.Delete(id)
	if err != nil {
		if depErr, ok := err.(*repository.HasDependentsError); ok {
			return &SecurityHasDependentsError{
				ID:         depErr.ID,
				Dependents: depErr.Dependents,
				Count:      depErr.Count,
			}
		}
		return err
	}
	return nil
}

// List returns securities matching the given filter.
func (s *SecurityService) List(filter repository.SecurityFilter) ([]*models.Security, error) {
	return s.repo.List(filter)
}

// =============================================================================
// Hide/Unhide Operations
// =============================================================================

// Hide marks a security as hidden.
// Fails if the security has open positions (stub: always succeeds until positions are built).
func (s *SecurityService) Hide(id models.ID) error {
	security, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if security.Hidden {
		return &SecurityAlreadyHiddenError{ID: id.String()}
	}

	// Placeholder: check for open positions. Always allows hiding until positions are built.
	if !security.CanHide() {
		return &SecurityHasOpenPositionsError{ID: id.String()}
	}

	security.Hide()
	return s.repo.Update(security)
}

// Unhide marks a security as visible.
func (s *SecurityService) Unhide(id models.ID) error {
	security, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if !security.Hidden {
		return &SecurityNotHiddenError{ID: id.String()}
	}

	security.Unhide()
	return s.repo.Update(security)
}

// =============================================================================
// Search Operations
// =============================================================================

// Search finds securities matching a partial ticker or name (case-insensitive).
func (s *SecurityService) Search(query string) ([]*models.Security, error) {
	if strings.TrimSpace(query) == "" {
		return s.repo.List(repository.SecurityFilter{})
	}

	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"

	sqlQuery := `
		SELECT id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		FROM securities
		WHERE LOWER(ticker) LIKE ? OR LOWER(name) LIKE ?
		ORDER BY ticker
	`

	rows, err := s.db.Conn().Query(sqlQuery, pattern, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to search securities: %w", err)
	}
	defer rows.Close()

	securities := make([]*models.Security, 0)
	for rows.Next() {
		sec := &models.Security{}
		err := rows.Scan(
			&sec.ID,
			&sec.Ticker,
			&sec.Name,
			&sec.SecurityType,
			&sec.AssetClass,
			&sec.Currency,
			&sec.Exchange,
			&sec.Hidden,
			&sec.CreatedAt,
			&sec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan security: %w", err)
		}
		securities = append(securities, sec)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating securities: %w", err)
	}

	return securities, nil
}

// =============================================================================
// Validation
// =============================================================================

// validateSecurity validates a security and returns any validation errors.
func (s *SecurityService) validateSecurity(security *models.Security) error {
	errors := security.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
