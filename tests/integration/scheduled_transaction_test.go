package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// createScheduledTestService creates a test database with a ScheduledTransactionService.
func createScheduledTestService(t *testing.T) (*scheduled.Service, *db.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "tmoney-scheduled-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	stRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	svc := scheduled.NewService(stRepo, txnRepo, txnSvc, database)

	cleanup := func() {
		database.Close()
		os.RemoveAll(tempDir)
	}

	return svc, database, cleanup
}

func TestScheduledTransactionCreate(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.NewDate(2024, 1, 1))
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("creates valid scheduled transaction", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 1),
			types.MustNewMoney("-100.00"),
		)
		st.SetMemo("Monthly rent")

		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create scheduled transaction: %v", err)
		}

		retrieved, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve: %v", err)
		}
		if retrieved.Frequency != scheduled.FrequencyMonthly {
			t.Errorf("Expected frequency 'monthly', got %q", retrieved.Frequency)
		}
		if !retrieved.Amount.Valid || !retrieved.Amount.Money.Equal(types.MustNewMoney("-100.00")) {
			t.Errorf("Expected amount -100.00, got %v", retrieved.Amount)
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Monthly rent" {
			t.Errorf("Expected memo 'Monthly rent', got %v", retrieved.Memo)
		}
	})

	t.Run("creates scheduled transaction with payee and category", func(t *testing.T) {
		payeeRepo := payee.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		payee := payee.NewPayee("Landlord")
		if err := payeeRepo.Create(payee); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		category := category.NewCategory("Housing", category.TypeExpense)
		if err := categoryRepo.Create(category); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		st := scheduled.NewTransactionFull(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-1500.00"),
			payee.ID,
			category.ID,
			"Rent payment",
		)

		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		retrieved, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve: %v", err)
		}
		if !retrieved.HasPayee() || retrieved.PayeeID.ID != payee.ID {
			t.Errorf("Expected payee %s, got %v", payee.ID.String(), retrieved.PayeeID)
		}
		if !retrieved.HasCategory() || retrieved.CategoryID.ID != category.ID {
			t.Errorf("Expected category %s, got %v", category.ID.String(), retrieved.CategoryID)
		}
	})
}

func TestScheduledTransactionCRUD(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	st := scheduled.NewTransactionWithAmount(
		account.ID,
		scheduled.FrequencyWeekly,
		types.NewDate(2024, 1, 1),
		types.MustNewMoney("-25.00"),
	)
	if err := svc.Create(st); err != nil {
		t.Fatalf("Failed to create: %v", err)
	}

	t.Run("update scheduled transaction", func(t *testing.T) {
		st.SetAmount(types.MustNewMoney("-30.00"))
		st.SetMemo("Updated memo")
		if err := svc.Update(st); err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		retrieved, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve: %v", err)
		}
		if !retrieved.Amount.Money.Equal(types.MustNewMoney("-30.00")) {
			t.Errorf("Expected amount -30.00, got %s", retrieved.Amount.Money.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Updated memo" {
			t.Errorf("Expected memo 'Updated memo', got %v", retrieved.Memo)
		}
	})

	t.Run("list scheduled transactions", func(t *testing.T) {
		all, err := svc.List()
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}
		if len(all) < 1 {
			t.Errorf("Expected at least 1 scheduled transaction, got %d", len(all))
		}
	})

	t.Run("list by account", func(t *testing.T) {
		byAccount, err := svc.ListByAccount(account.ID)
		if err != nil {
			t.Fatalf("Failed to list by account: %v", err)
		}
		if len(byAccount) < 1 {
			t.Errorf("Expected at least 1 scheduled transaction for account, got %d", len(byAccount))
		}
	})

	t.Run("delete scheduled transaction", func(t *testing.T) {
		deleteMe := scheduled.NewTransactionWithAmount(
			account.ID, scheduled.FrequencyDaily,
			types.NewDate(2024, 6, 1), types.MustNewMoney("-5.00"),
		)
		if err := svc.Create(deleteMe); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		if err := svc.Delete(deleteMe.ID); err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		_, err := svc.GetByID(deleteMe.ID)
		if err == nil {
			t.Error("Expected error when getting deleted scheduled transaction")
		}
	})
}

func TestScheduledTransactionPost(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	account := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("5000.00"), types.NewDate(2024, 1, 1))
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("post creates real transaction and advances schedule", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-500.00"),
		)
		st.SetMemo("Rent")

		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Post with the scheduled amount (pass nil to use the fixed amount)
		txn, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Failed to post: %v", err)
		}

		// Verify the created transaction
		if txn == nil {
			t.Fatal("Expected a transaction, got nil")
		}
		if !txn.Amount.Equal(types.MustNewMoney("-500.00")) {
			t.Errorf("Expected amount -500.00, got %s", txn.Amount.String())
		}
		if txn.AccountID != account.ID {
			t.Errorf("Expected account ID %s, got %s", account.ID.String(), txn.AccountID.String())
		}
		if !txn.Memo.Valid || txn.Memo.String != "Rent" {
			t.Errorf("Expected memo 'Rent', got %v", txn.Memo)
		}

		// Verify the transaction exists in the database
		_, err = txnRepo.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("Created transaction should exist in DB: %v", err)
		}

		// Verify the schedule advanced
		updated, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to get updated schedule: %v", err)
		}
		// Next date should have advanced from Jan 15 to Feb 15
		expectedNextDate := types.NewDate(2024, 2, 15)
		if !updated.NextDate.Equal(expectedNextDate) {
			t.Errorf("Expected next date %v, got %v", expectedNextDate, updated.NextDate)
		}
	})

	t.Run("post with custom amount overrides scheduled amount", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 3, 1),
			types.MustNewMoney("-100.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		customAmount := types.MustNewMoney("-75.00")
		txn, err := svc.Post(st.ID, &customAmount)
		if err != nil {
			t.Fatalf("Failed to post with custom amount: %v", err)
		}

		if !txn.Amount.Equal(types.MustNewMoney("-75.00")) {
			t.Errorf("Expected custom amount -75.00, got %s", txn.Amount.String())
		}
	})

	t.Run("post with fixed occurrences decrements remaining", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 4, 1),
			types.MustNewMoney("-50.00"),
		)
		st.SetOccurrences(3)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Post first occurrence
		_, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Failed to post first: %v", err)
		}

		updated, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to get updated: %v", err)
		}
		if !updated.OccurrencesRemaining.Valid || updated.OccurrencesRemaining.Int64 != 2 {
			t.Errorf("Expected 2 remaining occurrences, got %v", updated.OccurrencesRemaining)
		}
	})

	t.Run("post rejects completed schedule", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 5, 1),
			types.MustNewMoney("-10.00"),
		)
		st.SetOccurrences(1)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Post the only occurrence
		_, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Failed to post: %v", err)
		}

		// Try to post again - should fail
		_, err = svc.Post(st.ID, nil)
		if err == nil {
			t.Error("Expected error when posting completed schedule")
		}
		if _, ok := err.(*scheduled.CompletedError); !ok {
			t.Errorf("Expected ScheduledTransactionCompletedError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionPostWithDate(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("creates transaction with specified date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-200.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		customDate := types.NewDate(2024, 1, 20)
		txn, err := svc.PostWithDate(st.ID, customDate, nil)
		if err != nil {
			t.Fatalf("Failed to post with date: %v", err)
		}

		if !txn.Date.Equal(customDate) {
			t.Errorf("Expected date 2024-01-20, got %v", txn.Date)
		}
	})
}

func TestScheduledTransactionSkip(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("skip advances schedule without creating transaction", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyWeekly,
			types.NewDate(2024, 1, 1),
			types.MustNewMoney("-25.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Count transactions before
		txns, err := txnRepo.ListByAccount(account.ID)
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}
		countBefore := len(txns)

		// Skip
		if err := svc.Skip(st.ID); err != nil {
			t.Fatalf("Failed to skip: %v", err)
		}

		// Count transactions after - should be same
		txns, err = txnRepo.ListByAccount(account.ID)
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}
		if len(txns) != countBefore {
			t.Errorf("Expected %d transactions (no new ones), got %d", countBefore, len(txns))
		}

		// Verify the schedule advanced
		updated, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("Failed to get updated: %v", err)
		}
		expectedNextDate := types.NewDate(2024, 1, 8) // Weekly: Jan 1 + 7 days
		if !updated.NextDate.Equal(expectedNextDate) {
			t.Errorf("Expected next date %v, got %v", expectedNextDate, updated.NextDate)
		}
	})

	t.Run("skip rejects completed schedule", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 6, 1),
			types.MustNewMoney("-10.00"),
		)
		st.SetOccurrences(1)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Skip the only occurrence to complete
		if err := svc.Skip(st.ID); err != nil {
			t.Fatalf("Failed to skip: %v", err)
		}

		// Try to skip again
		err := svc.Skip(st.ID)
		if err == nil {
			t.Error("Expected error when skipping completed schedule")
		}
		if _, ok := err.(*scheduled.CompletedError); !ok {
			t.Errorf("Expected ScheduledTransactionCompletedError, got %T: %v", err, err)
		}
	})
}

func TestScheduledTransactionDueAndUpcoming(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create a scheduled transaction that is due (next_date in the past)
	pastDue := scheduled.NewTransactionWithAmount(
		account.ID,
		scheduled.FrequencyMonthly,
		types.NewDate(2020, 1, 1), // Far in the past
		types.MustNewMoney("-100.00"),
	)
	if err := svc.Create(pastDue); err != nil {
		t.Fatalf("Failed to create past due: %v", err)
	}

	// Create a scheduled transaction far in the future
	future := scheduled.NewTransactionWithAmount(
		account.ID,
		scheduled.FrequencyYearly,
		types.NewDate(2099, 12, 31), // Far in the future
		types.MustNewMoney("-50.00"),
	)
	if err := svc.Create(future); err != nil {
		t.Fatalf("Failed to create future: %v", err)
	}

	t.Run("list due returns past-due scheduled transactions", func(t *testing.T) {
		due, err := svc.ListDue()
		if err != nil {
			t.Fatalf("Failed to list due: %v", err)
		}

		found := false
		for _, st := range due {
			if st.ID == pastDue.ID {
				found = true
			}
		}
		if !found {
			t.Error("Expected past-due scheduled transaction in due list")
		}

		// Future should not be in the due list
		for _, st := range due {
			if st.ID == future.ID {
				t.Error("Future scheduled transaction should not be due")
			}
		}
	})

	t.Run("IsDue returns true for past-due", func(t *testing.T) {
		isDue, err := svc.IsDue(pastDue.ID)
		if err != nil {
			t.Fatalf("Failed to check IsDue: %v", err)
		}
		if !isDue {
			t.Error("Expected past-due transaction to be due")
		}
	})

	t.Run("IsDue returns false for future", func(t *testing.T) {
		isDue, err := svc.IsDue(future.ID)
		if err != nil {
			t.Fatalf("Failed to check IsDue: %v", err)
		}
		if isDue {
			t.Error("Expected future transaction to not be due")
		}
	})
}

func TestScheduledTransactionNextDateCalculation(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("monthly next date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-100.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		nextDate, err := svc.CalculateNextDate(st.ID)
		if err != nil {
			t.Fatalf("Failed to calculate next date: %v", err)
		}

		expected := types.NewDate(2024, 2, 15)
		if !nextDate.Equal(expected) {
			t.Errorf("Expected next date %v, got %v", expected, nextDate)
		}
	})

	t.Run("weekly next date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyWeekly,
			types.NewDate(2024, 1, 1),
			types.MustNewMoney("-25.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		nextDate, err := svc.CalculateNextDate(st.ID)
		if err != nil {
			t.Fatalf("Failed to calculate: %v", err)
		}

		expected := types.NewDate(2024, 1, 8)
		if !nextDate.Equal(expected) {
			t.Errorf("Expected next date %v, got %v", expected, nextDate)
		}
	})

	t.Run("biweekly next date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyBiweekly,
			types.NewDate(2024, 1, 5),
			types.MustNewMoney("-50.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		nextDate, err := svc.CalculateNextDate(st.ID)
		if err != nil {
			t.Fatalf("Failed to calculate: %v", err)
		}

		expected := types.NewDate(2024, 1, 19)
		if !nextDate.Equal(expected) {
			t.Errorf("Expected next date %v, got %v", expected, nextDate)
		}
	})

	t.Run("yearly next date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyYearly,
			types.NewDate(2024, 3, 15),
			types.MustNewMoney("-1000.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		nextDate, err := svc.CalculateNextDate(st.ID)
		if err != nil {
			t.Fatalf("Failed to calculate: %v", err)
		}

		expected := types.NewDate(2025, 3, 15)
		if !nextDate.Equal(expected) {
			t.Errorf("Expected next date %v, got %v", expected, nextDate)
		}
	})

	t.Run("GetNextDate returns current next date", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 6, 1),
			types.MustNewMoney("-200.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		nextDate, err := svc.GetNextDate(st.ID)
		if err != nil {
			t.Fatalf("Failed to get next date: %v", err)
		}

		// Should be the start date since no posts have happened
		expected := types.NewDate(2024, 6, 1)
		if !nextDate.Equal(expected) {
			t.Errorf("Expected next date %v, got %v", expected, nextDate)
		}
	})
}

func TestScheduledTransactionIsCompleted(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	account := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	t.Run("indefinite schedule is not completed", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 1, 1),
			types.MustNewMoney("-100.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		completed, err := svc.IsCompleted(st.ID)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if completed {
			t.Error("Indefinite schedule should not be completed")
		}
	})

	t.Run("schedule with used-up occurrences is completed", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 2, 1),
			types.MustNewMoney("-10.00"),
		)
		st.SetOccurrences(2)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Post twice to exhaust occurrences
		for i := range 2 {
			_, err := svc.Post(st.ID, nil)
			if err != nil {
				t.Fatalf("Failed to post occurrence %d: %v", i+1, err)
			}
		}

		completed, err := svc.IsCompleted(st.ID)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if !completed {
			t.Error("Schedule with exhausted occurrences should be completed")
		}
	})
}

func TestScheduledTransactionEstimateAmount(t *testing.T) {
	svc, database, cleanup := createScheduledTestService(t)
	defer cleanup()

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	account := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("10000.00"), types.NewDate(2024, 1, 1))
	if err := accountRepo.Create(account); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	payee := payee.NewPayee("Electric Company")
	if err := payeeRepo.Create(payee); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	t.Run("estimates from recent transactions", func(t *testing.T) {
		// Create some past transactions to this payee
		amounts := []string{"-100.00", "-120.00", "-110.00"}
		months := []time.Month{time.January, time.February, time.March}
		for i, amtStr := range amounts {
			txn := transaction.NewTransactionWithPayee(
				account.ID,
				types.NewDate(2024, months[i], 15),
				types.MustNewMoney(amtStr),
				payee.ID,
			)
			if err := txnRepo.Create(txn); err != nil {
				t.Fatalf("Failed to create transaction: %v", err)
			}
		}

		// Create a variable-amount scheduled transaction
		st := scheduled.NewTransaction(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 4, 15),
		)
		st.SetPayee(payee.ID)
		st.SetAmountEstimateCount(3)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("Failed to estimate: %v", err)
		}
		if estimated == nil {
			t.Fatal("Expected an estimated amount, got nil")
		}

		// Average of -100, -120, -110 = -110
		expectedAvg := types.MustNewMoney("-110.00")
		if !estimated.Equal(expectedAvg) {
			t.Errorf("Expected estimated amount %s, got %s", expectedAvg.String(), estimated.String())
		}
	})

	t.Run("returns fixed amount when set", func(t *testing.T) {
		st := scheduled.NewTransactionWithAmount(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 5, 1),
			types.MustNewMoney("-75.00"),
		)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("Failed to estimate: %v", err)
		}
		if estimated == nil {
			t.Fatal("Expected amount, got nil")
		}
		if !estimated.Equal(types.MustNewMoney("-75.00")) {
			t.Errorf("Expected -75.00, got %s", estimated.String())
		}
	})

	t.Run("returns nil when no payee and no amount", func(t *testing.T) {
		st := scheduled.NewTransaction(
			account.ID,
			scheduled.FrequencyMonthly,
			types.NewDate(2024, 6, 1),
		)
		st.SetAmountEstimateCount(3)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		estimated, err := svc.EstimateAmount(st.ID)
		if err != nil {
			t.Fatalf("Failed to estimate: %v", err)
		}
		if estimated != nil {
			t.Errorf("Expected nil estimate (no payee), got %s", estimated.String())
		}
	})
}
