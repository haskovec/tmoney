package investment

import (
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// Automatic price rows.
//
// A trade at a price the price history does not have implies a quote, so one is
// created alongside the transaction and removed again when that transaction is
// edited away or deleted — but only when nothing else still depends on it.

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
