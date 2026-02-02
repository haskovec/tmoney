package repository

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

// =============================================================================
// Scheduled Transaction CRUD Tests
// =============================================================================

func TestScheduledTransactionRepository_Create(t *testing.T) {
	t.Run("creates valid scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		// Create account first
		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		err := repo.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != account.ID {
			t.Errorf("Expected account ID %v, got %v", account.ID, retrieved.AccountID)
		}
		if retrieved.Frequency != models.FrequencyMonthly {
			t.Errorf("Expected frequency 'monthly', got %q", retrieved.Frequency)
		}
	})

	t.Run("creates scheduled transaction with all fields", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		payeeRepo := NewPayeeRepository(database)
		categoryRepo := NewCategoryRepository(database)
		repo := NewScheduledTransactionRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		category := models.NewCategory("Utilities", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create full scheduled transaction
		startDate := models.Today()
		amount := models.MustNewMoney("-125.50")
		st := models.NewScheduledTransactionFull(account.ID, models.FrequencyMonthly, startDate, amount, payee.ID, category.ID, "Monthly electric bill")

		err := repo.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if !retrieved.PayeeID.Valid || retrieved.PayeeID.ID != payee.ID {
			t.Error("Expected payee ID to be set")
		}
		if !retrieved.CategoryID.Valid || retrieved.CategoryID.ID != category.ID {
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
		repo := NewScheduledTransactionRepository(database)

		fakeAccountID := models.NewID()
		startDate := models.Today()
		st := models.NewScheduledTransaction(fakeAccountID, models.FrequencyMonthly, startDate)

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent account")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects non-existent payee", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		st.PayeeID = models.NullableID{ID: models.NewID(), Valid: true} // Non-existent payee

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent payee")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects non-existent category", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		st.CategoryID = models.NullableID{ID: models.NewID(), Valid: true} // Non-existent category

		err := repo.Create(st)
		if err == nil {
			t.Error("Create() expected error for non-existent category")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, startDate)
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
		if retrieved.Frequency != models.FrequencyWeekly {
			t.Errorf("Expected frequency 'weekly', got %q", retrieved.Frequency)
		}
	})

	t.Run("returns NotFoundError for non-existent scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewScheduledTransactionRepository(database)

		fakeID := models.NewID()
		_, err := repo.GetByID(fakeID)
		if err == nil {
			t.Error("GetByID() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionRepository_List(t *testing.T) {
	t.Run("returns empty list for empty database", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewScheduledTransactionRepository(database)

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
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		// Create schedules with different next dates
		date1 := models.Today().AddDays(10) // 10 days from now
		date2 := models.Today().AddDays(1)  // 1 day from now
		date3 := models.Today().AddDays(5)  // 5 days from now

		st1 := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, date1)
		st2 := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, date2)
		st3 := models.NewScheduledTransaction(account.ID, models.FrequencyDaily, date3)

		for _, s := range []*models.ScheduledTransaction{st1, st2, st3} {
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

func TestScheduledTransactionRepository_ListByAccount(t *testing.T) {
	t.Run("returns scheduled transactions for specific account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account1 := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		account2 := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account1); err != nil {
			t.Fatalf("Create account1 error = %v", err)
		}
		if err := accountRepo.Create(account2); err != nil {
			t.Fatalf("Create account2 error = %v", err)
		}

		startDate := models.Today()
		st1 := models.NewScheduledTransaction(account1.ID, models.FrequencyMonthly, startDate)
		st2 := models.NewScheduledTransaction(account1.ID, models.FrequencyWeekly, startDate)
		st3 := models.NewScheduledTransaction(account2.ID, models.FrequencyDaily, startDate)

		for _, s := range []*models.ScheduledTransaction{st1, st2, st3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		schedules, err := repo.ListByAccount(account1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(schedules) != 2 {
			t.Errorf("Expected 2 schedules for account1, got %d", len(schedules))
		}

		for _, s := range schedules {
			if s.AccountID != account1.ID {
				t.Errorf("Expected account ID %v, got %v", account1.ID, s.AccountID)
			}
		}
	})
}

func TestScheduledTransactionRepository_ListDue(t *testing.T) {
	t.Run("returns only due scheduled transactions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		pastDate := models.Today().AddDays(-5)  // 5 days ago
		today := models.Today()                 // today
		futureDate := models.Today().AddDays(5) // 5 days from now

		stPast := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, pastDate)
		stToday := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, today)
		stFuture := models.NewScheduledTransaction(account.ID, models.FrequencyDaily, futureDate)

		for _, s := range []*models.ScheduledTransaction{stPast, stToday, stFuture} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		dueSchedules, err := repo.ListDue()
		if err != nil {
			t.Fatalf("ListDue() error = %v", err)
		}

		// Should only return past and today (not future)
		if len(dueSchedules) != 2 {
			t.Errorf("Expected 2 due schedules, got %d", len(dueSchedules))
		}

		// Check that future schedule is not included
		for _, s := range dueSchedules {
			if s.ID == stFuture.ID {
				t.Error("Future schedule should not be in due list")
			}
		}
	})
}

func TestScheduledTransactionRepository_ListUpcoming(t *testing.T) {
	t.Run("returns scheduled transactions within days", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		date3days := models.Today().AddDays(3)
		date10days := models.Today().AddDays(10)

		st3days := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, date3days)
		st10days := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, date10days)

		for _, s := range []*models.ScheduledTransaction{st3days, st10days} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		upcoming, err := repo.ListUpcoming(7) // Within 7 days
		if err != nil {
			t.Fatalf("ListUpcoming() error = %v", err)
		}

		if len(upcoming) != 1 {
			t.Errorf("Expected 1 upcoming schedule within 7 days, got %d", len(upcoming))
		}
		if len(upcoming) > 0 && upcoming[0].ID != st3days.ID {
			t.Errorf("Expected schedule st3days")
		}
	})
}

func TestScheduledTransactionRepository_Update(t *testing.T) {
	t.Run("updates existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the scheduled transaction
		st.SetMemo("Updated memo")
		st.SetInterval(2)
		amount := models.MustNewMoney("-100.00")
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
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		// Don't create it, just try to update

		err := repo.Update(st)
		if err == nil {
			t.Error("Update() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("rejects update with non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Change to non-existent account
		st.AccountID = models.NewID()
		err := repo.Update(st)
		if err == nil {
			t.Error("Update() expected error for non-existent account")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionRepository_Delete(t *testing.T) {
	t.Run("deletes existing scheduled transaction", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		startDate := models.Today()
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
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
		repo := NewScheduledTransactionRepository(database)

		fakeID := models.NewID()
		err := repo.Delete(fakeID)
		if err == nil {
			t.Error("Delete() expected error for non-existent scheduled transaction")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Count Tests
// =============================================================================

func TestScheduledTransactionRepository_CountByAccount(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account1 := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		account2 := models.NewAccount("Savings", models.AccountTypeSavings, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account1); err != nil {
			t.Fatalf("Create account1 error = %v", err)
		}
		if err := accountRepo.Create(account2); err != nil {
			t.Fatalf("Create account2 error = %v", err)
		}

		startDate := models.Today()
		st1 := models.NewScheduledTransaction(account1.ID, models.FrequencyMonthly, startDate)
		st2 := models.NewScheduledTransaction(account1.ID, models.FrequencyWeekly, startDate)
		st3 := models.NewScheduledTransaction(account2.ID, models.FrequencyDaily, startDate)

		for _, s := range []*models.ScheduledTransaction{st1, st2, st3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		count, err := repo.CountByAccount(account1.ID)
		if err != nil {
			t.Fatalf("CountByAccount() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected count 2, got %d", count)
		}
	})
}

func TestScheduledTransactionRepository_CountByCategory(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		categoryRepo := NewCategoryRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		category := models.NewCategory("Utilities", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		startDate := models.Today()
		st1 := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		st1.CategoryID = models.NullableID{ID: category.ID, Valid: true}

		st2 := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, startDate)
		st2.CategoryID = models.NullableID{ID: category.ID, Valid: true}

		st3 := models.NewScheduledTransaction(account.ID, models.FrequencyDaily, startDate)
		// st3 has no category

		for _, s := range []*models.ScheduledTransaction{st1, st2, st3} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		count, err := repo.CountByCategory(category.ID)
		if err != nil {
			t.Fatalf("CountByCategory() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected count 2, got %d", count)
		}
	})
}

func TestScheduledTransactionRepository_CountByPayee(t *testing.T) {
	t.Run("returns correct count", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		payeeRepo := NewPayeeRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		startDate := models.Today()
		st1 := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, startDate)
		st1.PayeeID = models.NullableID{ID: payee.ID, Valid: true}

		st2 := models.NewScheduledTransaction(account.ID, models.FrequencyWeekly, startDate)
		// st2 has no payee

		for _, s := range []*models.ScheduledTransaction{st1, st2} {
			if err := repo.Create(s); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		count, err := repo.CountByPayee(payee.ID)
		if err != nil {
			t.Fatalf("CountByPayee() error = %v", err)
		}
		if count != 1 {
			t.Errorf("Expected count 1, got %d", count)
		}
	})
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestScheduledTransactionRepository_FullLifecycle(t *testing.T) {
	t.Run("full scheduled transaction lifecycle", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		payeeRepo := NewPayeeRepository(database)
		categoryRepo := NewCategoryRepository(database)
		repo := NewScheduledTransactionRepository(database)

		// Create dependencies
		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Create payee error = %v", err)
		}

		category := models.NewCategory("Utilities", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Create category error = %v", err)
		}

		// Create scheduled transaction
		startDate := models.Today()
		amount := models.MustNewMoney("-125.50")
		st := models.NewScheduledTransactionFull(account.ID, models.FrequencyMonthly, startDate, amount, payee.ID, category.ID, "Monthly electric bill")
		st.SetOccurrences(12) // 1 year

		if err := repo.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify creation
		retrieved, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Occurrences.Valid || retrieved.Occurrences.Int64 != 12 {
			t.Error("Expected occurrences to be 12")
		}
		if !retrieved.OccurrencesRemaining.Valid || retrieved.OccurrencesRemaining.Int64 != 12 {
			t.Error("Expected occurrences_remaining to be 12")
		}

		// Simulate advancing the schedule (posting a transaction)
		retrieved.AdvanceSchedule()
		if err := repo.Update(retrieved); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		// Verify advancement
		updated, err := repo.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID after update error = %v", err)
		}
		if !updated.OccurrencesRemaining.Valid || updated.OccurrencesRemaining.Int64 != 11 {
			t.Errorf("Expected occurrences_remaining to be 11, got %d", updated.OccurrencesRemaining.Int64)
		}

		// Delete
		if err := repo.Delete(st.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify deletion
		_, err = repo.GetByID(st.ID)
		if err == nil {
			t.Error("Expected error after delete")
		}
	})
}

func TestScheduledTransactionRepository_AllFrequencies(t *testing.T) {
	t.Run("supports all frequency types", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewScheduledTransactionRepository(database)

		now := time.Now()
		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD",
			models.ZeroMoney, models.NewDate(now.Year(), now.Month(), now.Day()))
		if err := accountRepo.Create(account); err != nil {
			t.Fatalf("Create account error = %v", err)
		}

		frequencies := models.AllFrequencies()
		startDate := models.Today()

		for _, freq := range frequencies {
			t.Run(string(freq), func(t *testing.T) {
				st := models.NewScheduledTransaction(account.ID, freq, startDate)
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
