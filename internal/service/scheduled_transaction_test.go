package service

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func createTestScheduledTransactionService(t *testing.T) (*ScheduledTransactionService, *repository.AccountRepository, *repository.PayeeRepository, *repository.CategoryRepository) {
	t.Helper()
	database := createTestDB(t)
	stRepo := repository.NewScheduledTransactionRepository(database)
	txnRepo := repository.NewTransactionRepository(database)
	accountRepo := repository.NewAccountRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)

	svc := NewScheduledTransactionService(stRepo, txnRepo, database)
	return svc, accountRepo, payeeRepo, categoryRepo
}

func createTestAccountForScheduled(t *testing.T, repo *repository.AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(name, models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return account
}

func TestNewScheduledTransactionService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _, _, _ := createTestScheduledTransactionService(t)
		if svc == nil {
			t.Error("NewScheduledTransactionService should not return nil")
		}
	})
}

func TestScheduledTransactionService_Create(t *testing.T) {
	t.Run("creates valid scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)

		err := svc.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Amount.Money.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), retrieved.Amount.Money.String())
		}
	})

	t.Run("validates scheduled transaction before creating", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		// Invalid frequency
		st := models.NewScheduledTransaction(account.ID, models.Frequency("invalid"), models.Today())

		err := svc.Create(st)
		if err == nil {
			t.Error("Create() expected error for invalid frequency")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("creates scheduled transaction with payee and category", func(t *testing.T) {
		svc, accountRepo, payeeRepo, categoryRepo := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		category := models.NewCategory("Utilities", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-150.00")
		st := models.NewScheduledTransactionFull(account.ID, models.FrequencyMonthly, models.Today(), amount, payee.ID, category.ID, "Electric bill")

		err := svc.Create(st)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, _ := svc.GetByID(st.ID)
		if !retrieved.HasPayee() {
			t.Error("Expected payee to be set")
		}
		if !retrieved.HasCategory() {
			t.Error("Expected category to be set")
		}
	})
}

func TestScheduledTransactionService_Update(t *testing.T) {
	t.Run("updates scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the amount
		newAmount, _ := models.NewMoney("-75.00")
		st.SetAmount(newAmount)
		if err := svc.Update(st); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, _ := svc.GetByID(st.ID)
		if !retrieved.Amount.Money.Equal(newAmount) {
			t.Errorf("Expected amount %s, got %s", newAmount.String(), retrieved.Amount.Money.String())
		}
	})

	t.Run("validates scheduled transaction before updating", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Invalid update: bad interval
		st.Interval = 0
		err := svc.Update(st)
		if err == nil {
			t.Error("Update() expected error for zero interval")
		}
	})
}

func TestScheduledTransactionService_Delete(t *testing.T) {
	t.Run("deletes scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(st.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(st.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})
}

func TestScheduledTransactionService_List(t *testing.T) {
	t.Run("lists all scheduled transactions", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount1, _ := models.NewMoney("-50.00")
		amount2, _ := models.NewMoney("-75.00")

		if err := svc.Create(models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount1)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyWeekly, models.Today(), amount2)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sts, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(sts) != 2 {
			t.Errorf("Expected 2 scheduled transactions, got %d", len(sts))
		}
	})

	t.Run("lists scheduled transactions by account", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account1 := createTestAccountForScheduled(t, accountRepo, "Checking")
		account2 := createTestAccountForScheduled(t, accountRepo, "Savings")

		amount, _ := models.NewMoney("-50.00")

		if err := svc.Create(models.NewScheduledTransactionWithAmount(account1.ID, models.FrequencyMonthly, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(models.NewScheduledTransactionWithAmount(account2.ID, models.FrequencyMonthly, models.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sts, err := svc.ListByAccount(account1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(sts) != 1 {
			t.Errorf("Expected 1 scheduled transaction for account1, got %d", len(sts))
		}
	})
}

func TestScheduledTransactionService_ListDue(t *testing.T) {
	t.Run("lists due scheduled transactions", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")

		// Create one due today
		st1 := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Create one in the future
		futureDate := models.Today().AddDays(30)
		st2 := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, futureDate, amount)
		if err := svc.Create(st2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		due, err := svc.ListDue()
		if err != nil {
			t.Fatalf("ListDue() error = %v", err)
		}
		if len(due) != 1 {
			t.Errorf("Expected 1 due scheduled transaction, got %d", len(due))
		}
	})
}

func TestScheduledTransactionService_Post(t *testing.T) {
	t.Run("posts scheduled transaction with fixed amount", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		txn, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		// Verify transaction was created
		if txn == nil {
			t.Fatal("Post() should return a transaction")
		}
		if !txn.Amount.Equal(amount) {
			t.Errorf("Expected transaction amount %s, got %s", amount.String(), txn.Amount.String())
		}
		if txn.AccountID != account.ID {
			t.Error("Transaction should be in the correct account")
		}

		// Verify schedule was advanced
		updated, _ := svc.GetByID(st.ID)
		if updated.NextDate == originalNextDate {
			t.Error("Schedule should have advanced to next date")
		}
	})

	t.Run("posts scheduled transaction with provided amount", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Post with a different amount
		overrideAmount, _ := models.NewMoney("-75.00")
		txn, err := svc.Post(st.ID, &overrideAmount)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if !txn.Amount.Equal(overrideAmount) {
			t.Errorf("Expected override amount %s, got %s", overrideAmount.String(), txn.Amount.String())
		}
	})

	t.Run("posts scheduled transaction with payee and category", func(t *testing.T) {
		svc, accountRepo, payeeRepo, categoryRepo := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		category := models.NewCategory("Utilities", models.CategoryTypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := models.NewMoney("-150.00")
		st := models.NewScheduledTransactionFull(account.ID, models.FrequencyMonthly, models.Today(), amount, payee.ID, category.ID, "Electric bill")
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if !txn.HasPayee() || txn.PayeeID.ID != payee.ID {
			t.Error("Transaction should have payee from scheduled transaction")
		}
		if !txn.HasCategory() || txn.CategoryID.ID != category.ID {
			t.Error("Transaction should have category from scheduled transaction")
		}
		if !txn.Memo.Valid || txn.Memo.String != "Electric bill" {
			t.Error("Transaction should have memo from scheduled transaction")
		}
	})

	t.Run("rejects posting completed schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		st.SetOccurrences(1)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Post the first (and only) occurrence
		_, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("First Post() error = %v", err)
		}

		// Try to post again
		_, err = svc.Post(st.ID, nil)
		if err == nil {
			t.Error("Post() expected error for completed schedule")
		}
		if _, ok := err.(*ScheduledTransactionCompletedError); !ok {
			t.Errorf("Expected ScheduledTransactionCompletedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects posting variable amount without estimate", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		// Create scheduled transaction without amount (variable)
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		_, err := svc.Post(st.ID, nil)
		if err == nil {
			t.Error("Post() expected error for variable amount without estimate")
		}
		if _, ok := err.(*ScheduledTransactionAmountRequiredError); !ok {
			t.Errorf("Expected ScheduledTransactionAmountRequiredError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionService_PostWithDate(t *testing.T) {
	t.Run("posts scheduled transaction with specific date", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		customDate := models.Today().AddDays(-5)
		txn, err := svc.PostWithDate(st.ID, customDate, nil)
		if err != nil {
			t.Fatalf("PostWithDate() error = %v", err)
		}

		if txn.Date != customDate {
			t.Errorf("Expected transaction date %s, got %s", customDate.String(), txn.Date.String())
		}
	})
}

func TestScheduledTransactionService_Skip(t *testing.T) {
	t.Run("skips scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		err := svc.Skip(st.ID)
		if err != nil {
			t.Fatalf("Skip() error = %v", err)
		}

		// Verify schedule was advanced
		updated, _ := svc.GetByID(st.ID)
		if updated.NextDate == originalNextDate {
			t.Error("Schedule should have advanced to next date")
		}
	})

	t.Run("rejects skipping completed schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		st.SetOccurrences(1)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Skip the first (and only) occurrence
		err := svc.Skip(st.ID)
		if err != nil {
			t.Fatalf("First Skip() error = %v", err)
		}

		// Try to skip again
		err = svc.Skip(st.ID)
		if err == nil {
			t.Error("Skip() expected error for completed schedule")
		}
		if _, ok := err.(*ScheduledTransactionCompletedError); !ok {
			t.Errorf("Expected ScheduledTransactionCompletedError, got %T: %v", err, err)
		}
	})

	t.Run("decrements occurrences remaining", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		st.SetOccurrences(3)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Skip once
		if err := svc.Skip(st.ID); err != nil {
			t.Fatalf("Skip() error = %v", err)
		}

		updated, _ := svc.GetByID(st.ID)
		if !updated.OccurrencesRemaining.Valid || updated.OccurrencesRemaining.Int64 != 2 {
			t.Errorf("Expected 2 occurrences remaining, got %d", updated.OccurrencesRemaining.Int64)
		}
	})
}

func TestScheduledTransactionService_EstimateAmount(t *testing.T) {
	t.Run("returns fixed amount when set", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("EstimateAmount() error = %v", err)
		}
		if estimated == nil {
			t.Fatal("EstimateAmount() should return amount for fixed schedule")
		}
		if !estimated.Equal(amount) {
			t.Errorf("Expected %s, got %s", amount.String(), estimated.String())
		}
	})

	t.Run("returns nil when no estimate count set", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("EstimateAmount() error = %v", err)
		}
		if estimated != nil {
			t.Error("EstimateAmount() should return nil when no estimate count set")
		}
	})

	t.Run("estimates from past transactions", func(t *testing.T) {
		database := createTestDB(t)
		stRepo := repository.NewScheduledTransactionRepository(database)
		txnRepo := repository.NewTransactionRepository(database)
		accountRepo := repository.NewAccountRepository(database)
		payeeRepo := repository.NewPayeeRepository(database)

		svc := NewScheduledTransactionService(stRepo, txnRepo, database)

		account := createTestAccountForScheduled(t, accountRepo, "Checking")
		payee := models.NewPayee("Electric Company")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		// Create past transactions to this payee
		amount1, _ := models.NewMoney("-100.00")
		amount2, _ := models.NewMoney("-120.00")
		amount3, _ := models.NewMoney("-110.00")

		date1, _ := models.ParseDate("2024-01-15")
		date2, _ := models.ParseDate("2024-02-15")
		date3, _ := models.ParseDate("2024-03-15")

		txn1 := models.NewTransactionWithPayee(account.ID, date1, amount1, payee.ID)
		txn2 := models.NewTransactionWithPayee(account.ID, date2, amount2, payee.ID)
		txn3 := models.NewTransactionWithPayee(account.ID, date3, amount3, payee.ID)

		if err := txnRepo.Create(txn1); err != nil {
			t.Fatalf("Failed to create txn1: %v", err)
		}
		if err := txnRepo.Create(txn2); err != nil {
			t.Fatalf("Failed to create txn2: %v", err)
		}
		if err := txnRepo.Create(txn3); err != nil {
			t.Fatalf("Failed to create txn3: %v", err)
		}

		// Create scheduled transaction with estimate count
		st := models.NewScheduledTransaction(account.ID, models.FrequencyMonthly, models.Today())
		st.SetPayee(payee.ID)
		st.SetAmountEstimateCount(3)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("EstimateAmount() error = %v", err)
		}
		if estimated == nil {
			t.Fatal("EstimateAmount() should return estimate")
		}

		// Average of -100, -120, -110 = -110
		expectedAvg, _ := models.NewMoney("-110.00")
		if !estimated.Equal(expectedAvg) {
			t.Errorf("Expected average %s, got %s", expectedAvg.String(), estimated.String())
		}
	})
}

func TestScheduledTransactionService_IsDue(t *testing.T) {
	t.Run("returns true for due schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isDue, err := svc.IsDue(st.ID)
		if err != nil {
			t.Fatalf("IsDue() error = %v", err)
		}
		if !isDue {
			t.Error("IsDue() should return true for schedule due today")
		}
	})

	t.Run("returns false for future schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		futureDate := models.Today().AddDays(30)
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, futureDate, amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isDue, err := svc.IsDue(st.ID)
		if err != nil {
			t.Fatalf("IsDue() error = %v", err)
		}
		if isDue {
			t.Error("IsDue() should return false for future schedule")
		}
	})
}

func TestScheduledTransactionService_IsCompleted(t *testing.T) {
	t.Run("returns true for completed schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		st.SetOccurrences(1)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Post the only occurrence
		if _, err := svc.Post(st.ID, nil); err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		isCompleted, err := svc.IsCompleted(st.ID)
		if err != nil {
			t.Fatalf("IsCompleted() error = %v", err)
		}
		if !isCompleted {
			t.Error("IsCompleted() should return true for completed schedule")
		}
	})

	t.Run("returns false for active schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, models.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isCompleted, err := svc.IsCompleted(st.ID)
		if err != nil {
			t.Fatalf("IsCompleted() error = %v", err)
		}
		if isCompleted {
			t.Error("IsCompleted() should return false for indefinite schedule")
		}
	})
}

func TestScheduledTransactionService_GetNextDate(t *testing.T) {
	t.Run("returns next date", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		startDate := models.Today()
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, startDate, amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		nextDate, err := svc.GetNextDate(st.ID)
		if err != nil {
			t.Fatalf("GetNextDate() error = %v", err)
		}
		if nextDate != startDate {
			t.Errorf("Expected next date %s, got %s", startDate.String(), nextDate.String())
		}
	})
}

func TestScheduledTransactionService_CalculateNextDate(t *testing.T) {
	t.Run("calculates next occurrence without modifying schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		account := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := models.NewMoney("-50.00")
		startDate := models.Today()
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, startDate, amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		calculatedNext, err := svc.CalculateNextDate(st.ID)
		if err != nil {
			t.Fatalf("CalculateNextDate() error = %v", err)
		}

		// Next date should be calculated
		if calculatedNext == startDate {
			t.Error("CalculateNextDate() should calculate the next occurrence after current")
		}

		// Original schedule should not be modified
		retrieved, _ := svc.GetByID(st.ID)
		if retrieved.NextDate != startDate {
			t.Error("CalculateNextDate() should not modify the schedule")
		}
	})
}
