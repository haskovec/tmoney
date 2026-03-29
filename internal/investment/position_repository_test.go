package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestPositionData creates a position with required supporting data (account, security).
func createTestPositionData(t *testing.T, accountRepo *account.Repository, secRepo *security.Repository, posRepo *PositionRepository,
) (*account.Account, *security.Security, *Position) {
	t.Helper()

	acct := createInvestmentAccountForTest(t, accountRepo)
	sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

	pos := NewPositionWithShares(
		acct.ID,
		sec.ID,
		types.MustNewQuantity("10"),
		types.MustNewMoney("185.00"),
	)
	if err := posRepo.CreateOrUpdate(&pos); err != nil {
		t.Fatalf("Failed to create position: %v", err)
	}

	return acct, sec, &pos
}

// =============================================================================
// SM-060: PositionRepository.CreateOrUpdate
// =============================================================================

func TestPositionRepository_CreateOrUpdate(t *testing.T) {
	t.Run("creates a new position and verifies all fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		pos := NewPositionWithShares(
			acct.ID,
			sec.ID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
		)

		err := posRepo.CreateOrUpdate(&pos)
		if err != nil {
			t.Fatalf("CreateOrUpdate() error = %v", err)
		}

		// Verify by reading back
		retrieved, err := posRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if retrieved.AccountID != acct.ID {
			t.Errorf("Expected account_id %v, got %v", acct.ID, retrieved.AccountID)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.Shares.String() != "10" {
			t.Errorf("Expected shares 10, got %q", retrieved.Shares.String())
		}
		if retrieved.AverageCostPerShare.String() != "185" {
			t.Errorf("Expected average_cost_per_share 185, got %q", retrieved.AverageCostPerShare.String())
		}
	})

	t.Run("updates existing position for same account+security (upsert)", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create initial position
		pos := NewPositionWithShares(
			acct.ID,
			sec.ID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
		)
		if err := posRepo.CreateOrUpdate(&pos); err != nil {
			t.Fatalf("CreateOrUpdate() initial error = %v", err)
		}

		// Add shares and upsert
		if err := pos.AddShares(types.MustNewQuantity("5"), types.MustNewMoney("190.00")); err != nil {
			t.Fatalf("AddShares() error = %v", err)
		}

		err := posRepo.CreateOrUpdate(&pos)
		if err != nil {
			t.Fatalf("CreateOrUpdate() upsert error = %v", err)
		}

		// Verify updated values
		retrieved, err := posRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if retrieved.Shares.String() != "15" {
			t.Errorf("Expected shares 15, got %q", retrieved.Shares.String())
		}
		// Weighted average: (10*185 + 5*190) / 15 = 2800/15 ~ 186.666...
		// Check it's not the old value
		if retrieved.AverageCostPerShare.String() == "185" {
			t.Error("Expected average_cost_per_share to be updated from 185")
		}
	})

	t.Run("verifies account_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		fakeAccountID := types.NewID()

		pos := NewPositionWithShares(
			fakeAccountID,
			sec.ID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
		)

		err := posRepo.CreateOrUpdate(&pos)
		if err == nil {
			t.Fatal("Expected error for invalid account_id foreign key, got nil")
		}
	})

	t.Run("verifies security_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		fakeSecurityID := types.NewID()

		pos := NewPositionWithShares(
			acct.ID,
			fakeSecurityID,
			types.MustNewQuantity("10"),
			types.MustNewMoney("185.00"),
		)

		err := posRepo.CreateOrUpdate(&pos)
		if err == nil {
			t.Fatal("Expected error for invalid security_id foreign key, got nil")
		}
	})
}

// =============================================================================
// SM-061: PositionRepository.GetByAccountAndSecurity
// =============================================================================

func TestPositionRepository_GetByAccountAndSecurity(t *testing.T) {
	t.Run("returns existing position", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct, sec, original := createTestPositionData(t, accountRepo, secRepo, posRepo)

		retrieved, err := posRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if retrieved.ID != original.ID {
			t.Errorf("Expected ID %v, got %v", original.ID, retrieved.ID)
		}
		if retrieved.Shares.String() != "10" {
			t.Errorf("Expected shares 10, got %q", retrieved.Shares.String())
		}
	})

	t.Run("returns zero-value position when not found (not error)", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		retrieved, err := posRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("Expected no error for missing position, got %v", err)
		}
		if retrieved.Shares.String() != "0" {
			t.Errorf("Expected zero shares for missing position, got %q", retrieved.Shares.String())
		}
		if retrieved.AverageCostPerShare.String() != "0" {
			t.Errorf("Expected zero average cost for missing position, got %q", retrieved.AverageCostPerShare.String())
		}
		if retrieved.AccountID != acct.ID {
			t.Errorf("Expected account_id %v, got %v", acct.ID, retrieved.AccountID)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
	})
}

// =============================================================================
// SM-062: PositionRepository.ListByAccount
// =============================================================================

func TestPositionRepository_ListByAccount(t *testing.T) {
	t.Run("lists all positions for account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		pos1 := NewPositionWithShares(acct.ID, aapl.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"))
		pos2 := NewPositionWithShares(acct.ID, msft.ID, types.MustNewQuantity("20"), types.MustNewMoney("350.00"))

		for _, pos := range []*Position{&pos1, &pos2} {
			if err := posRepo.CreateOrUpdate(pos); err != nil {
				t.Fatalf("CreateOrUpdate() error = %v", err)
			}
		}

		results, err := posRepo.ListByAccount(acct.ID, false)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 positions, got %d", len(results))
		}
	})

	t.Run("excludes zero-share positions when requested", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		// Position with shares
		pos1 := NewPositionWithShares(acct.ID, aapl.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"))
		// Position with zero shares
		pos2 := NewPosition(acct.ID, msft.ID)

		for _, pos := range []*Position{&pos1, &pos2} {
			if err := posRepo.CreateOrUpdate(pos); err != nil {
				t.Fatalf("CreateOrUpdate() error = %v", err)
			}
		}

		// Exclude zero-share positions
		results, err := posRepo.ListByAccount(acct.ID, true)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 non-zero position, got %d", len(results))
		}
		if results[0].SecurityID != aapl.ID {
			t.Error("Expected AAPL position")
		}
	})

	t.Run("includes zero-share positions when not excluded", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		msft := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		pos1 := NewPositionWithShares(acct.ID, aapl.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"))
		pos2 := NewPosition(acct.ID, msft.ID)

		for _, pos := range []*Position{&pos1, &pos2} {
			if err := posRepo.CreateOrUpdate(pos); err != nil {
				t.Fatalf("CreateOrUpdate() error = %v", err)
			}
		}

		results, err := posRepo.ListByAccount(acct.ID, false)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 positions (including zero-share), got %d", len(results))
		}
	})

	t.Run("returns empty slice when no positions exist", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)

		results, err := posRepo.ListByAccount(acct.ID, false)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("Expected 0 positions, got %d", len(results))
		}
	})

	t.Run("does not return positions for other accounts", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		account1 := createInvestmentAccountForTest(t, accountRepo)
		account2 := account.NewAccount("Roth IRA", account.TypeInvestment, "USD", types.MustNewMoney("0.00"), types.NewDate(2024, 1, 1))
		if err := accountRepo.Create(account2); err != nil {
			t.Fatalf("Failed to create account2: %v", err)
		}

		aapl := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		pos1 := NewPositionWithShares(account1.ID, aapl.ID, types.MustNewQuantity("10"), types.MustNewMoney("185.00"))
		pos2 := NewPositionWithShares(account2.ID, aapl.ID, types.MustNewQuantity("20"), types.MustNewMoney("190.00"))

		for _, pos := range []*Position{&pos1, &pos2} {
			if err := posRepo.CreateOrUpdate(pos); err != nil {
				t.Fatalf("CreateOrUpdate() error = %v", err)
			}
		}

		results, err := posRepo.ListByAccount(account1.ID, false)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 position for account1, got %d", len(results))
		}
		if results[0].AccountID != account1.ID {
			t.Error("Expected position from account1")
		}
	})
}

// =============================================================================
// SM-063: PositionRepository.Delete
// =============================================================================

func TestPositionRepository_Delete(t *testing.T) {
	t.Run("deletes existing position", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct, sec, _ := createTestPositionData(t, accountRepo, secRepo, posRepo)

		err := posRepo.Delete(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify deleted -- should return zero-value position
		retrieved, err := posRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if retrieved.Shares.String() != "0" {
			t.Errorf("Expected zero shares after delete, got %q", retrieved.Shares.String())
		}
	})

	t.Run("returns NotFoundError for non-existent position", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		secRepo := security.NewRepository(database)
		posRepo := NewPositionRepository(database)

		acct := createInvestmentAccountForTest(t, accountRepo)
		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		err := posRepo.Delete(acct.ID, sec.ID)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		_, ok := err.(*dberrors.NotFoundError)
		if !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}
