package security

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/types"
)

// OpenLotChecker checks if a security has open lots across any account.
type OpenLotChecker interface {
	HasOpenLots(securityID types.ID) (bool, error)
}

// OpenPositionChecker checks if a security has non-zero positions across any account.
type OpenPositionChecker interface {
	HasOpenPositions(securityID types.ID) (bool, error)
}

// Service provides business logic for security operations.
type Service struct {
	repo            *Repository
	db              *db.DB
	lotChecker      OpenLotChecker
	positionChecker OpenPositionChecker
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

// ServiceOption configures optional dependencies for the security Service.
type ServiceOption func(*Service)

// WithLotChecker sets the lot checker for position validation.
func WithLotChecker(checker OpenLotChecker) ServiceOption {
	return func(s *Service) {
		s.lotChecker = checker
	}
}

// WithPositionChecker sets the position checker for position validation.
func WithPositionChecker(checker OpenPositionChecker) ServiceOption {
	return func(s *Service) {
		s.positionChecker = checker
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
// Fails if any account holds shares of this security (via lots or positions).
func (s *Service) Hide(id types.ID) error {
	sec, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	if sec.Hidden {
		return &AlreadyHiddenError{ID: id.String()}
	}

	// Check for open lots (lot-tracking accounts).
	if s.lotChecker != nil {
		hasLots, err := s.lotChecker.HasOpenLots(id)
		if err != nil {
			return fmt.Errorf("failed to check open lots: %w", err)
		}
		if hasLots {
			return &HasOpenPositionsError{ID: id.String()}
		}
	}

	// Check for non-zero positions (non-lot-tracking accounts).
	if s.positionChecker != nil {
		hasPositions, err := s.positionChecker.HasOpenPositions(id)
		if err != nil {
			return fmt.Errorf("failed to check open positions: %w", err)
		}
		if hasPositions {
			return &HasOpenPositionsError{ID: id.String()}
		}
	}

	sec.Hide()
	return s.repo.Update(sec)
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
// Hidden securities are excluded from search results.
func (s *Service) Search(query string) ([]*Security, error) {
	if strings.TrimSpace(query) == "" {
		excludeHidden := true
		return s.repo.List(Filter{ExcludeHidden: &excludeHidden})
	}

	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"

	sqlQuery := `
		SELECT id, ticker, name, security_type, asset_class, currency,
			exchange, hidden, created_at, updated_at
		FROM securities
		WHERE (LOWER(ticker) LIKE ? OR LOWER(name) LIKE ?)
			AND hidden = FALSE
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
