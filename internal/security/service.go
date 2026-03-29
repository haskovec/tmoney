package security

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// Service provides business logic for security operations.
type Service struct {
	repo *Repository
	db   *db.DB
}

// NewService creates a new Service.
func NewService(repo *Repository, database *db.DB) *Service {
	return &Service{
		repo: repo,
		db:   database,
	}
}

// =============================================================================
// CRUD Operations
// =============================================================================

// Create validates and creates a new security.
func (s *Service) Create(security *Security) error {
	if err := s.validateSecurity(security); err != nil {
		return err
	}
	return s.repo.Create(security)
}

// GetByID retrieves a security by its ID.
func (s *Service) GetByID(id types.ID) (*Security, error) {
	return s.repo.GetByID(id)
}

// GetByTicker retrieves a security by its ticker symbol and optional currency.
func (s *Service) GetByTicker(ticker string, currency string) (*Security, error) {
	return s.repo.GetByTicker(ticker, currency)
}

// Update validates and updates an existing security.
func (s *Service) Update(security *Security) error {
	if err := s.validateSecurity(security); err != nil {
		return err
	}
	return s.repo.Update(security)
}

// Delete removes a security. If the security has prices or transactions,
// returns a HasDependentsError suggesting hiding instead.
func (s *Service) Delete(id types.ID) error {
	err := s.repo.Delete(id)
	if err != nil {
		if depErr, ok := err.(*dberrors.HasDependentsError); ok {
			return &HasDependentsError{
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
func (s *Service) List(filter Filter) ([]*Security, error) {
	return s.repo.List(filter)
}

// =============================================================================
// Hide/Unhide Operations
// =============================================================================

// Hide marks a security as hidden.
// Fails if the security has open positions (stub: always succeeds until positions are built).
func (s *Service) Hide(id types.ID) error {
	security, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if security.Hidden {
		return &AlreadyHiddenError{ID: id.String()}
	}

	// Placeholder: check for open positions. Always allows hiding until positions are built.
	if !security.CanHide() {
		return &HasOpenPositionsError{ID: id.String()}
	}

	security.Hide()
	return s.repo.Update(security)
}

// Unhide marks a security as visible.
func (s *Service) Unhide(id types.ID) error {
	security, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if !security.Hidden {
		return &NotHiddenError{ID: id.String()}
	}

	security.Unhide()
	return s.repo.Update(security)
}

// =============================================================================
// Search Operations
// =============================================================================

// Search finds securities matching a partial ticker or name (case-insensitive).
func (s *Service) Search(query string) ([]*Security, error) {
	if strings.TrimSpace(query) == "" {
		return s.repo.List(Filter{})
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

	securities := make([]*Security, 0)
	for rows.Next() {
		sec := &Security{}
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
func (s *Service) validateSecurity(security *Security) error {
	errors := security.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}
