package scheduled

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return database
}

// =============================================================================
// Scheduled Transaction CRUD Tests
// =============================================================================

func TestRepository_Create(t *testing.T) {
	t.Run("creates valid scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		// Create account first
		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		err := repo.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != acct.ID {
			t.Errorf("Expected account ID %v, got %v", acct.ID, retrieved.AccountID)
		}
		if retrieved.Frequency != FrequencyMonthly {
			t.Errorf("Expected frequency 'monthly', got %q", retrieved.Frequency)
		}
	})

	t.Run("creates scheduled transaction with all fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		payeeRepo := payee.NewRepository(database)
		categoryRepo := category.NewRepository(database)
		repo := NewRepository(database)

		// Create dependencies
		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		py := payee.NewPayee("Electric Company")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		cat := category.NewCategory("Utilities", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create full scheduled transaction
		startDate := types.Today()
		amount := types.MustNewMoney("-125.50")
		st := NewTransactionFull(acct.ID, FrequencyMonthly, startDate, amount, py.ID, cat.ID, "Monthly electric bill")

		err := repo.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if !retrieved.PayeeID.Valid || retrieved.PayeeID.ID != py.ID {
			t.Error("Expected payee ID to be set")
		}
		if !retrieved.CategoryID.Valid || retrieved.CategoryID.ID != cat.ID {
			t.Error("Expected category ID to be set")
		}
		if !retrieved.Amount.Valid {
			t.Error("Expected amount to be set")
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Monthly electric bill" {
			t.Error("Expected memo to be set")
		}
	})

	t.Run("rejects non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		fakeAccountID := types.NewID()
		startDate := types.Today()
		st := NewTransaction(fakeAccountID, FrequencyMonthly, startDate)

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent account")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		st.PayeeID = types.NullableID{ID: types.NewID(), Valid: true} // Non-existent payee

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent payee")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		st.CategoryID = types.NullableID{ID: types.NewID(), Valid: true} // Non-existent category

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent category")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyWeekly, startDate)
		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != st.ID {
			t.Errorf("Expected ID %v, got %v", st.ID, retrieved.ID)
		}
		if retrieved.Frequency != FrequencyWeekly {
			t.Errorf("Expected frequency 'weekly', got %q", retrieved.Frequency)
		}
	})

	t.Run("returns NotFoundError for non-existent scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		fakeID := types.NewID()
		_, err := repo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_List(t *testing.T) {
	t.Run("returns empty list for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		schedules, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(schedules) != 0 {
			t.Errorf("Expected 0 schedules, got %d", len(schedules))
		}
	})

	t.Run("returns all scheduled transactions ordered by next_date", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create schedules with different next dates
		date1 := types.Today().AddDays(10) // 10 days from now
		date2 := types.Today().AddDays(1)  // 1 day from now
		date3 := types.Today().AddDays(5)  // 5 days from now

		st1 := NewTransaction(acct.ID, FrequencyMonthly, date1)
		st2 := NewTransaction(acct.ID, FrequencyWeekly, date2)
		st3 := NewTransaction(acct.ID, FrequencyDaily, date3)

		for _, s := range []*Transaction{st1, st2, st3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		schedules, err := repo.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(schedules) != 3 {
			t.Fatalf("Expected 3 schedules, got %d", len(schedules))
		}

		// Should be ordered by next_date ascending
		if schedules[0].ID != st2.ID {
			t.Errorf("Expected first schedule to be st2 (earliest date)")
		}
		if schedules[1].ID != st3.ID {
			t.Errorf("Expected second schedule to be st3")
		}
		if schedules[2].ID != st1.ID {
			t.Errorf("Expected third schedule to be st1 (latest date)")
		}
	})
}

func TestRepository_ListByAccount(t *testing.T) {
	t.Run("returns scheduled transactions for specific account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct1 := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		acct2 := account.NewAccount("Savings", account.TypeSavings, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct1); err != nil {
			t.Fatalf("Create account1 error = %v", err)
		}
		if err := accountRepo.Create(acct2); err != nil {
			t.Fatalf("Create account2 error = %v", err)
		}

		startDate := types.Today()
		st1 := NewTransaction(acct1.ID, FrequencyMonthly, startDate)
		st2 := NewTransaction(acct1.ID, FrequencyWeekly, startDate)
		st3 := NewTransaction(acct2.ID, FrequencyDaily, startDate)

		for _, s := range []*Transaction{st1, st2, st3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		schedules, err := repo.ListByAccount(acct1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(schedules) != 2 {
			t.Errorf("Expected 2 schedules for account1, got %d", len(schedules))
		}

		for _, s := range schedules {
			if s.AccountID != acct1.ID {
				t.Errorf("Expected account ID %v, got %v", acct1.ID, s.AccountID)
			}
		}
	})
}

func TestRepository_Update(t *testing.T) {
	t.Run("updates existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the scheduled transaction
		st.SetMemo("Updated memo")
		st.SetInterval(2)
		amount := types.MustNewMoney("-100.00")
		st.SetAmount(amount)

		if err := repo.Update(st); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Updated memo" {
			t.Error("Expected memo to be updated")
		}
		if retrieved.Interval != 2 {
			t.Errorf("Expected interval 2, got %d", retrieved.Interval)
		}
		if !retrieved.Amount.Valid {
			t.Error("Expected amount to be set")
		}
	})

	t.Run("returns NotFoundError for non-existent scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		// Don't create it, just try to update

		err := repo.Update(st)
		if err == nil {
			t.Error("Update() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_Delete(t *testing.T) {
	t.Run("deletes existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := types.Today()
		st := NewTransaction(acct.ID, FrequencyMonthly, startDate)
		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := repo.Delete(st.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := repo.GetByID(st.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("returns NotFoundError for non-existent scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		fakeID := types.NewID()
		err := repo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_AllFrequencies(t *testing.T) {
	t.Run("supports all frequency types", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		now := time.Now()
		acct := account.NewAccount("Checking", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		frequencies := AllFrequencies()
		startDate := types.Today()

		for _, freq := range frequencies {
			t.Run(string(freq), func(t *testing.T) {
				st := NewTransaction(acct.ID, freq, startDate)
				if err := repo.Create(st); err != nil {
					t.Fatalf("Create() error = %v", err)
				}

				retrieved, err := repo.GetByID(st.ID)
				if err != nil {
					t.Fatalf("GetByID() error = %v", err)
				}
				if retrieved.Frequency != freq {
					t.Errorf("Expected frequency %q, got %q", freq, retrieved.Frequency)
				}
			})
		}
	})
}
