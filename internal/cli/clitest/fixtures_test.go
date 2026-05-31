package clitest

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// reopen reopens a fixture's database from disk, proving the fixture wrote a
// valid file and closed it cleanly. The reopened handle is closed on cleanup.
func reopen(t *testing.T, path string) *db.DB {
	t.Helper()
	if path == "" {
		t.Fatal("fixture returned an empty database path")
	}
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("failed to reopen fixture database %q: %v", path, err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestCreateTestDBWithSecurity(t *testing.T) {
	path, sec := CreateTestDBWithSecurity(t)
	if sec == nil || sec.Ticker != "AAPL" {
		t.Fatalf("expected returned AAPL security, got %+v", sec)
	}

	got, err := security.NewRepository(reopen(t, path)).GetByTicker(sec.Ticker, sec.Currency)
	if err != nil {
		t.Fatalf("security not persisted: %v", err)
	}
	if got.Name != "Apple Inc." {
		t.Errorf("persisted security name = %q, want %q", got.Name, "Apple Inc.")
	}
}

func TestCreateTestDBWithSecurityAndPrices(t *testing.T) {
	path, sec := CreateTestDBWithSecurityAndPrices(t)
	if sec == nil || sec.Ticker != "AAPL" {
		t.Fatalf("expected returned AAPL security, got %+v", sec)
	}

	priceRepo := price.NewRepository(reopen(t, path))
	got, err := priceRepo.GetBySecurityAndDate(sec.ID, types.MustParseDate("2024-01-15"))
	if err != nil {
		t.Fatalf("price for 2024-01-15 not persisted: %v", err)
	}
	if !got.Price.Equal(types.MustNewMoney("150.00")) {
		t.Errorf("persisted price = %s, want 150.00", got.Price)
	}
}

func TestCreateInvestmentTestDB(t *testing.T) {
	path := CreateInvestmentTestDB(t, true)

	acct, err := account.NewRepository(reopen(t, path)).GetByName("Brokerage")
	if err != nil {
		t.Fatalf("investment account not persisted: %v", err)
	}
	if acct.Type != account.TypeInvestment {
		t.Errorf("account type = %v, want %v", acct.Type, account.TypeInvestment)
	}
	if !acct.TrackLots {
		t.Error("trackLots=true was not persisted on the account")
	}
}

func TestCreateCorporateActionTestDB(t *testing.T) {
	path := CreateCorporateActionTestDB(t, true, true)

	secRepo := security.NewRepository(reopen(t, path))
	for _, ticker := range []string{"AAPL", "GOOG"} {
		if _, err := secRepo.GetByTicker(ticker, "USD"); err != nil {
			t.Errorf("security %q not persisted: %v", ticker, err)
		}
	}
}

func TestPtrMoney(t *testing.T) {
	got := PtrMoney("12.34")
	if got == nil {
		t.Fatal("PtrMoney returned nil")
	}
	if !got.Equal(types.MustNewMoney("12.34")) {
		t.Errorf("PtrMoney(%q) = %s, want 12.34", "12.34", got)
	}
}
