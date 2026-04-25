package price

import (
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// defaultRefreshSleep is the polite delay between consecutive provider
// requests during RefreshPrices.
const defaultRefreshSleep = 200 * time.Millisecond

// Service provides business logic for security price operations.
type Service struct {
	repo         *Repository
	secRepo      *security.Repository
	db           *db.DB
	registry     *ProviderRegistry
	refreshSleep time.Duration
}

// NewService creates a new Service.
func NewService(repo *Repository, secRepo *security.Repository, database *db.DB) *Service {
	return &Service{
		repo:         repo,
		secRepo:      secRepo,
		db:           database,
		registry:     NewProviderRegistry(),
		refreshSleep: defaultRefreshSleep,
	}
}

// ProviderRegistry returns the price provider registry for external registration.
func (s *Service) ProviderRegistry() *ProviderRegistry {
	return s.registry
}

// =============================================================================
// CRUD Operations
// =============================================================================

// AddPrice validates and creates a new security price.
func (s *Service) AddPrice(price *Price) error {
	if err := s.validatePrice(price); err != nil {
		return err
	}

	// Verify the security exists and is not hidden
	sec, err := s.secRepo.GetByID(price.SecurityID)
	if err != nil {
		return err
	}
	if sec.Hidden {
		return &HiddenSecurityError{SecurityID: price.SecurityID.String()}
	}

	err = s.repo.Create(price)
	if err != nil {
		if dupErr, ok := err.(*dberrors.DuplicateError); ok {
			return &AlreadyExistsError{
				SecurityID: price.SecurityID.String(),
				Date:       price.Date.String(),
				Detail:     dupErr.Error(),
			}
		}
		return err
	}
	return nil
}

// UpdatePrice validates and updates an existing security price (upsert by security+date).
func (s *Service) UpdatePrice(price *Price) error {
	if err := s.validatePrice(price); err != nil {
		return err
	}

	// Verify the security exists and is not hidden
	sec, err := s.secRepo.GetByID(price.SecurityID)
	if err != nil {
		return err
	}
	if sec.Hidden {
		return &HiddenSecurityError{SecurityID: price.SecurityID.String()}
	}

	return s.repo.CreateOrUpdate(price)
}

// GetCurrentPrice returns the most recent price on or before the given date.
func (s *Service) GetCurrentPrice(securityID types.ID, asOf types.Date) (*Price, error) {
	return s.repo.GetCurrentPrice(securityID, asOf)
}

// GetPriceHistory returns prices for a security within an optional date range.
func (s *Service) GetPriceHistory(securityID types.ID, from *types.Date, to *types.Date) ([]*Price, error) {
	return s.repo.GetPriceHistory(securityID, from, to)
}

// GetLatestPrices returns the most recent price per non-hidden security
// that has any prices, sorted by ticker. Used by the prices view's top-
// level list.
func (s *Service) GetLatestPrices() ([]*LatestPrice, error) {
	return s.repo.GetLatestPrices()
}

// DeletePrice removes a security price by its ID.
func (s *Service) DeletePrice(id types.ID) error {
	return s.repo.Delete(id)
}

// BulkImport imports multiple prices with optional overwrite behavior.
func (s *Service) BulkImport(prices []*Price, overwrite bool) (*BulkImportResult, error) {
	// Validate all prices and check for hidden securities before importing
	for _, p := range prices {
		if err := s.validatePrice(p); err != nil {
			return nil, err
		}
		sec, err := s.secRepo.GetByID(p.SecurityID)
		if err != nil {
			return nil, err
		}
		if sec.Hidden {
			return nil, &HiddenSecurityError{SecurityID: p.SecurityID.String()}
		}
	}
	return s.repo.BulkCreate(prices, overwrite)
}

// =============================================================================
// Validation
// =============================================================================

// validatePrice validates a security price and returns any validation errors.
func (s *Service) validatePrice(price *Price) error {
	errors := price.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	return nil
}
