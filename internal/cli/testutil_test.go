package cli

import (
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestDBWithSecurity creates a test DB and inserts a security, returning the path.
func createTestDBWithSecurity(t *testing.T) (string, *security.Security) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	repo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	sec.SetExchange("NASDAQ")
	if err := repo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}

	database.Close()
	return dbPath, sec
}

// createTestDBWithSecurityAndPrices creates a test DB with a security plus three sample prices.
func createTestDBWithSecurityAndPrices(t *testing.T) (string, *security.Security) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	sec.AssetClass = security.AssetClassLargeCapStock
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create test security: %v", err)
	}

	priceRepo := price.NewRepository(database)
	p1 := price.NewPrice(sec.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("150.00"), price.SourceManual)
	if err := priceRepo.Create(p1); err != nil {
		t.Fatalf("failed to create price 1: %v", err)
	}
	p2 := price.NewPrice(sec.ID, types.MustParseDate("2024-02-15"), types.MustNewMoney("160.50"), price.SourceTransaction)
	if err := priceRepo.Create(p2); err != nil {
		t.Fatalf("failed to create price 2: %v", err)
	}
	p3 := price.NewPrice(sec.ID, types.MustParseDate("2024-03-15"), types.MustNewMoney("170.25"), price.SourceImport)
	if err := priceRepo.Create(p3); err != nil {
		t.Fatalf("failed to create price 3: %v", err)
	}

	database.Close()
	return dbPath, sec
}

// createInvestmentTestDB creates a test database with an investment account, a deposit for cash,
// and a security. Returns the dbPath. The database is closed after setup.
func createInvestmentTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "invest.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = trackLots
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("50000"), "initial deposit")
	if err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	database.Close()
	return dbPath
}

// ptrMoney is a helper to create a pointer to Money.
func ptrMoney(s string) *types.Money {
	m := types.MustNewMoney(s)
	return &m
}

// createCorporateActionTestDB creates a DB with an investment account holding shares.
// Returns the DB path. If withSecondSecurity is true, also creates a "GOOG" security.
func createCorporateActionTestDB(t *testing.T, trackLots bool, withSecondSecurity bool) string {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "corp.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	acct.TrackLots = trackLots
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}

	secRepo := security.NewRepository(database)
	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := secRepo.Create(sec); err != nil {
		t.Fatalf("failed to create security: %v", err)
	}

	if withSecondSecurity {
		sec2 := security.NewSecurity("GOOG", "Alphabet Inc.", security.TypeStock)
		if err := secRepo.Create(sec2); err != nil {
			t.Fatalf("failed to create second security: %v", err)
		}
	}

	svc := app.NewServices(database)
	_, err = svc.Investment.Deposit(acct.ID, types.Today(), types.MustNewMoney("100000"), "initial deposit")
	if err != nil {
		t.Fatalf("failed to deposit cash: %v", err)
	}

	totalAmount := types.MustNewMoney("15000")
	_, err = svc.Investment.Buy(acct.ID, sec.ID, types.Today(), types.MustNewQuantity("100"), &totalAmount, nil, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("failed to buy shares: %v", err)
	}

	database.Close()
	return dbPath
}
