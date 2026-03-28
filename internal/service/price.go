package service

import (
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// PriceService provides business logic for security price operations.
type PriceService struct {
	repo     *repository.PriceRepository
	secRepo  *repository.SecurityRepository
	db       *db.DB
	registry *PriceProviderRegistry
}

// NewPriceService creates a new PriceService.
func NewPriceService(repo *repository.PriceRepository, secRepo *repository.SecurityRepository, database *db.DB) *PriceService {
	return &PriceService{
		repo:     repo,
		secRepo:  secRepo,
		db:       database,
		registry: NewPriceProviderRegistry(),
	}
}

// ProviderRegistry returns the price provider registry for external registration.
func (s *PriceService) ProviderRegistry() *PriceProviderRegistry {
	return s.registry
}

// =============================================================================
// CRUD Operations
// =============================================================================

// AddPrice validates and creates a new security price.
func (s *PriceService) AddPrice(price *models.SecurityPrice) error {
	if err := s.validatePrice(price); err != nil {
		return err
	}

	// Verify the security exists
	_, err := s.secRepo.GetByID(price.SecurityID)
	if err != nil {
		return err
	}

	err = s.repo.Create(price)
	if err != nil {
		if dupErr, ok := err.(*repository.DuplicateError); ok {
			return &PriceAlreadyExistsError{
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
func (s *PriceService) UpdatePrice(price *models.SecurityPrice) error {
	if err := s.validatePrice(price); err != nil {
		return err
	}

	// Verify the security exists
	_, err := s.secRepo.GetByID(price.SecurityID)
	if err != nil {
		return err
	}

	return s.repo.CreateOrUpdate(price)
}

// GetCurrentPrice returns the most recent price on or before the given date.
func (s *PriceService) GetCurrentPrice(securityID models.ID, asOf models.Date) (*models.SecurityPrice, error) {
	return s.repo.GetCurrentPrice(securityID, asOf)
}

// GetPriceHistory returns prices for a security within an optional date range.
func (s *PriceService) GetPriceHistory(securityID models.ID, from *models.Date, to *models.Date) ([]*models.SecurityPrice, error) {
	return s.repo.GetPriceHistory(securityID, from, to)
}

// DeletePrice removes a security price by its ID.
func (s *PriceService) DeletePrice(id models.ID) error {
	return s.repo.Delete(id)
}

// BulkImport imports multiple prices with optional overwrite behavior.
func (s *PriceService) BulkImport(prices []*models.SecurityPrice, overwrite bool) (*repository.BulkImportResult, error) {
	// Validate all prices before importing
	for _, price := range prices {
		if err := s.validatePrice(price); err != nil {
			return nil, err
		}
	}
	return s.repo.BulkCreate(prices, overwrite)
}

// =============================================================================
// Validation
// =============================================================================

// validatePrice validates a security price and returns any validation errors.
func (s *PriceService) validatePrice(price *models.SecurityPrice) error {
	errors := price.Validate()
	if errors.HasErrors() {
		return &ServiceValidationError{Errors: errors}
	}
	return nil
}
