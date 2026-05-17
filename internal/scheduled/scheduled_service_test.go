package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestScheduledTransactionService(t *testing.T) (*Service, *account.Repository, *payee.Repository, *category.Repository) {
	t.Helper()
	database := createTestDB(t)
	stRepo := NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

	svc := NewService(stRepo, txnRepo, txnSvc, database)
	return svc, accountRepo, payeeRepo, categoryRepo
}

func createTestAccountForScheduled(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

func TestNewService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _, _, _ := createTestScheduledTransactionService(t)
		if svc == nil {
			t.Error("NewService should not return nil")
		}
	})
}

func TestService_Create(t *testing.T) {
	t.Run("creates valid scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)

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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		// Invalid frequency
		st := NewTransaction(acct.ID, Frequency("invalid"), types.Today())

		err := svc.Create(st)
		if err == nil {
			t.Error("Create() expected error for invalid frequency")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("creates scheduled transaction with payee and category", func(t *testing.T) {
		svc, accountRepo, payeeRepo, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		py := payee.NewPayee("Electric Company")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		cat := category.NewCategory("Utilities", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-150.00")
		st := NewTransactionFull(acct.ID, FrequencyMonthly, types.Today(), amount, py.ID, cat.ID, "Electric bill")

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

func TestService_Update(t *testing.T) {
	t.Run("updates scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the amount
		newAmount, _ := types.NewMoney("-75.00")
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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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

func TestService_Delete(t *testing.T) {
	t.Run("deletes scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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

func TestService_List(t *testing.T) {
	t.Run("lists all scheduled transactions", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount1, _ := types.NewMoney("-50.00")
		amount2, _ := types.NewMoney("-75.00")

		if err := svc.Create(NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount1)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransactionWithAmount(acct.ID, FrequencyWeekly, types.Today(), amount2)); err != nil {
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
		acct1 := createTestAccountForScheduled(t, accountRepo, "Checking")
		acct2 := createTestAccountForScheduled(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("-50.00")

		if err := svc.Create(NewTransactionWithAmount(acct1.ID, FrequencyMonthly, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransactionWithAmount(acct2.ID, FrequencyMonthly, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		sts, err := svc.ListByAccount(acct1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(sts) != 1 {
			t.Errorf("Expected 1 scheduled transaction for account1, got %d", len(sts))
		}
	})
}

func TestService_Post(t *testing.T) {
	t.Run("posts scheduled transaction with fixed amount", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		if txn.AccountID != acct.ID {
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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Post with a different amount
		overrideAmount, _ := types.NewMoney("-75.00")
		txn, err := svc.Post(st.ID, &overrideAmount)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}

		if !txn.Amount.Equal(overrideAmount) {
			t.Errorf("Expected override amount %s, got %s", overrideAmount.String(), txn.Amount.String())
		}
	})

	t.Run("rejects posting completed schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		if _, ok := err.(*CompletedError); !ok {
			t.Errorf("Expected CompletedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects posting variable amount without estimate", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		// Create scheduled transaction without amount (variable)
		st := NewTransaction(acct.ID, FrequencyMonthly, types.Today())
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		_, err := svc.Post(st.ID, nil)
		if err == nil {
			t.Error("Post() expected error for variable amount without estimate")
		}
		if _, ok := err.(*AmountRequiredError); !ok {
			t.Errorf("Expected AmountRequiredError, got %T: %v", err, err)
		}
	})
}

func TestService_PostWithDate(t *testing.T) {
	t.Run("posts scheduled transaction with specific date", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		customDate := types.Today().AddDays(-5)
		txn, err := svc.PostWithDate(st.ID, customDate, nil)
		if err != nil {
			t.Fatalf("PostWithDate() error = %v", err)
		}

		if txn.Date != customDate {
			t.Errorf("Expected transaction date %s, got %s", customDate.String(), txn.Date.String())
		}
	})
}

func TestService_Skip(t *testing.T) {
	t.Run("skips scheduled transaction", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		if _, ok := err.(*CompletedError); !ok {
			t.Errorf("Expected CompletedError, got %T: %v", err, err)
		}
	})

	t.Run("decrements occurrences remaining", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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

func TestService_EstimateAmount(t *testing.T) {
	t.Run("returns fixed amount when set", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		st := NewTransaction(acct.ID, FrequencyMonthly, types.Today())
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
		stRepo := NewRepository(database)
		txnRepo := transaction.NewRepository(database)
		splitRepo := transaction.NewSplitRepository(database)
		transferRepo := transaction.NewTransferRepository(database, txnRepo)
		accountRepo := account.NewRepository(database)
		payeeRepo := payee.NewRepository(database)
		txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, database)

		svc := NewService(stRepo, txnRepo, txnSvc, database)

		acct := createTestAccountForScheduled(t, accountRepo, "Checking")
		py := payee.NewPayee("Electric Company")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		// Create past transactions to this payee
		amount1, _ := types.NewMoney("-100.00")
		amount2, _ := types.NewMoney("-120.00")
		amount3, _ := types.NewMoney("-110.00")

		date1, _ := types.ParseDate("2024-01-15")
		date2, _ := types.ParseDate("2024-02-15")
		date3, _ := types.ParseDate("2024-03-15")

		txn1 := transaction.NewTransactionWithPayee(acct.ID, date1, amount1, py.ID)
		txn2 := transaction.NewTransactionWithPayee(acct.ID, date2, amount2, py.ID)
		txn3 := transaction.NewTransactionWithPayee(acct.ID, date3, amount3, py.ID)

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
		st := NewTransaction(acct.ID, FrequencyMonthly, types.Today())
		st.SetPayee(py.ID)
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
		expectedAvg, _ := types.NewMoney("-110.00")
		if !estimated.Equal(expectedAvg) {
			t.Errorf("Expected average %s, got %s", expectedAvg.String(), estimated.String())
		}
	})
}

func TestService_IsDue(t *testing.T) {
	t.Run("returns true for due schedule", func(t *testing.T) {
		svc, accountRepo, _, _ := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
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
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		futureDate := types.Today().AddDays(30)
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, futureDate, amount)
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

func TestService_AutoPost(t *testing.T) {
	t.Run("auto-posts due transaction with fixed amount", func(t *testing.T) {
		svc, accountRepo, payeeRepo, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		py := payee.NewPayee("Landlord")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}
		cat := category.NewCategory("Housing", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-1500.00")
		st := NewTransactionFull(
			acct.ID, FrequencyMonthly, types.Today(), amount,
			py.ID, cat.ID, "Monthly rent",
		)
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		summary, err := svc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}

		if summary.PostedCount != 1 {
			t.Errorf("Expected 1 posted, got %d", summary.PostedCount)
		}
		if summary.SkippedCount != 0 {
			t.Errorf("Expected 0 skipped, got %d", summary.SkippedCount)
		}
		if len(summary.Results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(summary.Results))
		}

		result := summary.Results[0]
		if len(result.Transactions) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(result.Transactions))
		}

		txn := result.Transactions[0]
		if !txn.Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), txn.Amount.String())
		}
	})

	t.Run("returns empty summary when nothing to auto-post", func(t *testing.T) {
		svc, _, _, _ := createTestScheduledTransactionService(t)

		summary, err := svc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}

		if summary.PostedCount != 0 {
			t.Errorf("Expected 0 posted, got %d", summary.PostedCount)
		}
		if summary.SkippedCount != 0 {
			t.Errorf("Expected 0 skipped, got %d", summary.SkippedCount)
		}
		if len(summary.Results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(summary.Results))
		}
	})

	t.Run("auto-posts multi-line schedule with transfer-line counterpart", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")
		retirement := createTestAccountForScheduled(t, accountRepo, "401k")

		incCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(incCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("800.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		retire, _ := types.NewMoney("-200.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, gross),
			NewTransferSplit(st.ID, retirement.ID, retire),
		}
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create multi-line: %v", err)
		}

		summary, err := svc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost: %v", err)
		}
		if summary.PostedCount != 1 {
			t.Fatalf("PostedCount = %d, want 1", summary.PostedCount)
		}
		if len(summary.Results) != 1 || len(summary.Results[0].Transactions) != 1 {
			t.Fatalf("expected one auto-post result with one transaction")
		}
		txn := summary.Results[0].Transactions[0]
		if !txn.Amount.Equal(net) {
			t.Errorf("parent amount = %s, want %s", txn.Amount.String(), net.String())
		}

		splitRepo := transaction.NewSplitRepository(svc.db)
		gotSplits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction: %v", err)
		}
		if len(gotSplits) != 2 {
			t.Fatalf("split count = %d, want 2", len(gotSplits))
		}

		// Paired transfer counterpart must exist in the retirement account.
		var transferID types.NullableID
		for _, sp := range gotSplits {
			if sp.TransferAccountID.Valid {
				transferID = sp.TransferID
			}
		}
		if !transferID.Valid {
			t.Fatal("transfer-line split has no TransferID")
		}
		txnRepo := transaction.NewRepository(svc.db)
		paired, err := txnRepo.ListByTransferID(transferID.ID)
		if err != nil {
			t.Fatalf("ListByTransferID: %v", err)
		}
		if len(paired) != 1 || paired[0].AccountID != retirement.ID {
			t.Errorf("expected one paired counterpart in retirement account, got %+v", paired)
		}

		// Schedule next_date must have advanced.
		updated, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if updated.NextDate == st.NextDate {
			t.Error("schedule did not advance after multi-line auto-post")
		}
	})
}

// =============================================================================
// Multi-line scheduled-template tests (MS-015)
// =============================================================================

func TestScheduledService_CreateMultiLine_RoundTrip(t *testing.T) {
	t.Run("paycheck-shaped multi-line template persists and reloads", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")
		retirement := createTestAccountForScheduled(t, accountRepo, "401k")

		salaryCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(salaryCat); err != nil {
			t.Fatalf("Create salary category: %v", err)
		}
		federalCat := category.NewCategory("Federal Tax", category.TypeExpense)
		if err := categoryRepo.Create(federalCat); err != nil {
			t.Fatalf("Create federal category: %v", err)
		}

		// Parent represents the net deposit: 5000 - 800 - 200 = 4000.
		net, _ := types.NewMoney("4000.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)

		gross, _ := types.NewMoney("5000.00")
		federal, _ := types.NewMoney("-800.00")
		retire, _ := types.NewMoney("-200.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, salaryCat.ID, gross),
			NewCategorizedSplit(st.ID, federalCat.ID, federal),
			NewTransferSplit(st.ID, retirement.ID, retire),
		}

		if err := svc.Create(st); err != nil {
			t.Fatalf("Create multi-line: %v", err)
		}

		got, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if len(got.Splits) != 3 {
			t.Fatalf("Splits length = %d, want 3", len(got.Splits))
		}
		if !got.Splits.Total().Equal(net) {
			t.Errorf("signed sum = %s, want %s", got.Splits.Total().String(), net.String())
		}

		var sawSalary, sawFederal, sawTransfer bool
		for _, sp := range got.Splits {
			switch {
			case sp.CategoryID.Valid && sp.CategoryID.ID == salaryCat.ID:
				sawSalary = true
				if !sp.Amount.Equal(gross) {
					t.Errorf("salary amount = %s, want %s", sp.Amount.String(), gross.String())
				}
			case sp.CategoryID.Valid && sp.CategoryID.ID == federalCat.ID:
				sawFederal = true
				if !sp.Amount.Equal(federal) {
					t.Errorf("federal amount = %s, want %s", sp.Amount.String(), federal.String())
				}
			case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == retirement.ID:
				sawTransfer = true
				if !sp.Amount.Equal(retire) {
					t.Errorf("retirement amount = %s, want %s", sp.Amount.String(), retire.String())
				}
			}
		}
		if !sawSalary || !sawFederal || !sawTransfer {
			t.Errorf("missing reloaded splits: salary=%v federal=%v transfer=%v",
				sawSalary, sawFederal, sawTransfer)
		}
	})

	t.Run("update replaces child splits", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		incCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(incCat); err != nil {
			t.Fatalf("Create income category: %v", err)
		}
		expCat := category.NewCategory("Federal Tax", category.TypeExpense)
		if err := categoryRepo.Create(expCat); err != nil {
			t.Fatalf("Create expense category: %v", err)
		}

		net, _ := types.NewMoney("900.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		tax, _ := types.NewMoney("-100.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, gross),
			NewCategorizedSplit(st.ID, expCat.ID, tax),
		}
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}

		// Replace splits with a different shape (still balanced).
		newNet, _ := types.NewMoney("750.00")
		st.SetAmount(newNet)
		newGross, _ := types.NewMoney("1000.00")
		newTax, _ := types.NewMoney("-250.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, newGross),
			NewCategorizedSplit(st.ID, expCat.ID, newTax),
		}
		if err := svc.Update(st); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID after update: %v", err)
		}
		if len(got.Splits) != 2 {
			t.Fatalf("Splits length after update = %d, want 2", len(got.Splits))
		}
		if !got.Splits.Total().Equal(newNet) {
			t.Errorf("signed sum after update = %s, want %s",
				got.Splits.Total().String(), newNet.String())
		}
	})

	t.Run("imbalanced split rejected", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		cat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("100.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		// Single line of 200 does not net to parent 100.
		off, _ := types.NewMoney("200.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, cat.ID, off),
		}

		err := svc.Create(st)
		if err == nil {
			t.Fatal("expected validation error for imbalanced split, got nil")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("expected ServiceValidationError, got %T: %v", err, err)
		}

		// Parent should not have been persisted on validation failure.
		_, getErr := svc.GetByID(st.ID)
		if getErr == nil {
			t.Error("imbalanced create should not persist the parent")
		}
	})

	t.Run("self-transfer line rejected", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		cat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("500.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		// Transfer-line targeting the parent's own account: self-transfer.
		selfXfer, _ := types.NewMoney("-500.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, cat.ID, gross),
			NewTransferSplit(st.ID, acct.ID, selfXfer),
		}

		err := svc.Create(st)
		if err == nil {
			t.Fatal("expected error for self-transfer line, got nil")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("expected ServiceValidationError, got %T: %v", err, err)
		}
	})
}

func TestScheduledService_PostMultiLine_CreatesTransactionWithSplits(t *testing.T) {
	t.Run("posts multi-line schedule with paired transfer counterpart", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")
		retirement := createTestAccountForScheduled(t, accountRepo, "401k")

		salaryCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(salaryCat); err != nil {
			t.Fatalf("Create salary category: %v", err)
		}
		federalCat := category.NewCategory("Federal Tax", category.TypeExpense)
		if err := categoryRepo.Create(federalCat); err != nil {
			t.Fatalf("Create federal category: %v", err)
		}

		// Paycheck-shape template: gross +5000, federal -800, transfer -200 to 401k,
		// net deposit +4000 lands in Checking.
		net, _ := types.NewMoney("4000.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("5000.00")
		federal, _ := types.NewMoney("-800.00")
		retire, _ := types.NewMoney("-200.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, salaryCat.ID, gross),
			NewCategorizedSplit(st.ID, federalCat.ID, federal),
			NewTransferSplit(st.ID, retirement.ID, retire),
		}
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create multi-line schedule: %v", err)
		}

		txn, err := svc.Post(st.ID, nil)
		if err != nil {
			t.Fatalf("Post multi-line: %v", err)
		}
		if txn == nil {
			t.Fatal("Post returned nil transaction")
		}
		if txn.AccountID != acct.ID {
			t.Errorf("parent account = %v, want %v", txn.AccountID, acct.ID)
		}
		if !txn.Amount.Equal(net) {
			t.Errorf("parent amount = %s, want %s", txn.Amount.String(), net.String())
		}
		if txn.HasCategory() {
			t.Error("multi-line parent transaction should have no scalar category")
		}

		// Reload splits from the repository to mirror what the UI sees.
		splitRepo := transaction.NewSplitRepository(svc.db)
		gotSplits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction: %v", err)
		}
		if len(gotSplits) != 3 {
			t.Fatalf("split count = %d, want 3", len(gotSplits))
		}

		var sawSalary, sawFederal, transferSplit *transaction.Split
		for _, sp := range gotSplits {
			switch {
			case !sp.CategoryID.IsNil() && sp.CategoryID == salaryCat.ID:
				sp := sp
				sawSalary = sp
			case !sp.CategoryID.IsNil() && sp.CategoryID == federalCat.ID:
				sp := sp
				sawFederal = sp
			case sp.TransferAccountID.Valid && sp.TransferAccountID.ID == retirement.ID:
				sp := sp
				transferSplit = sp
			}
		}
		if sawSalary == nil || !sawSalary.Amount.Equal(gross) {
			t.Errorf("salary split missing or wrong amount: %+v", sawSalary)
		}
		if sawFederal == nil || !sawFederal.Amount.Equal(federal) {
			t.Errorf("federal split missing or wrong amount: %+v", sawFederal)
		}
		if transferSplit == nil {
			t.Fatal("transfer-line split not found on posted transaction")
		}
		if !transferSplit.Amount.Equal(retire) {
			t.Errorf("transfer split amount = %s, want %s",
				transferSplit.Amount.String(), retire.String())
		}
		if !transferSplit.TransferID.Valid {
			t.Fatal("transfer-line split has no TransferID set")
		}

		// Paired counter-transaction should exist in the retirement account.
		txnRepo := transaction.NewRepository(svc.db)
		paired, err := txnRepo.ListByTransferID(transferSplit.TransferID.ID)
		if err != nil {
			t.Fatalf("ListByTransferID: %v", err)
		}
		if len(paired) != 1 {
			t.Fatalf("paired transactions = %d, want 1", len(paired))
		}
		pair := paired[0]
		if pair.AccountID != retirement.ID {
			t.Errorf("paired account = %v, want %v", pair.AccountID, retirement.ID)
		}
		want := retire.Neg()
		if !pair.Amount.Equal(want) {
			t.Errorf("paired amount = %s, want %s", pair.Amount.String(), want.String())
		}
		if pair.TransferAccountID.ID != acct.ID {
			t.Errorf("paired transfer_account_id = %v, want %v", pair.TransferAccountID.ID, acct.ID)
		}
	})

	t.Run("Post on multi-line schedule rejects amount override", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		incCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(incCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}
		expCat := category.NewCategory("Tax", category.TypeExpense)
		if err := categoryRepo.Create(expCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("900.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		tax, _ := types.NewMoney("-100.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, gross),
			NewCategorizedSplit(st.ID, expCat.ID, tax),
		}
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}

		override, _ := types.NewMoney("999.00")
		if _, err := svc.Post(st.ID, &override); err == nil {
			t.Fatal("expected amount-override on multi-line Post to be rejected, got nil")
		}
	})
}

func TestScheduledService_PostMultiLine_AdvancesSchedule(t *testing.T) {
	t.Run("next_date advances per cadence after multi-line Post", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		incCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(incCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}
		expCat := category.NewCategory("Tax", category.TypeExpense)
		if err := categoryRepo.Create(expCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("900.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		tax, _ := types.NewMoney("-100.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, gross),
			NewCategorizedSplit(st.ID, expCat.ID, tax),
		}
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}
		original := st.NextDate
		expectedNext := st.CalculateNextDate()

		if _, err := svc.Post(st.ID, nil); err != nil {
			t.Fatalf("Post: %v", err)
		}

		updated, err := svc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if updated.NextDate == original {
			t.Error("schedule next_date did not advance after multi-line Post")
		}
		if updated.NextDate != expectedNext {
			t.Errorf("next_date = %s, want %s",
				updated.NextDate.String(), expectedNext.String())
		}
		// Children should still be present on the template (the post creates a
		// real transaction; it does not consume the template's splits).
		if len(updated.Splits) != 2 {
			t.Errorf("template splits after post = %d, want 2", len(updated.Splits))
		}
	})

	t.Run("PostWithDate posts multi-line at custom date", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")
		retirement := createTestAccountForScheduled(t, accountRepo, "401k")

		incCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(incCat); err != nil {
			t.Fatalf("Create category: %v", err)
		}

		net, _ := types.NewMoney("800.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), net)
		gross, _ := types.NewMoney("1000.00")
		retire, _ := types.NewMoney("-200.00")
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, incCat.ID, gross),
			NewTransferSplit(st.ID, retirement.ID, retire),
		}
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create: %v", err)
		}

		customDate := types.Today().AddDays(-5)
		txn, err := svc.PostWithDate(st.ID, customDate, nil)
		if err != nil {
			t.Fatalf("PostWithDate: %v", err)
		}
		if txn.Date != customDate {
			t.Errorf("parent date = %s, want %s", txn.Date.String(), customDate.String())
		}

		// Paired counterpart inherits the parent's date.
		splitRepo := transaction.NewSplitRepository(svc.db)
		gotSplits, err := splitRepo.ListByTransaction(txn.ID)
		if err != nil {
			t.Fatalf("ListByTransaction: %v", err)
		}
		var transferID types.NullableID
		for _, sp := range gotSplits {
			if sp.TransferAccountID.Valid {
				transferID = sp.TransferID
			}
		}
		if !transferID.Valid {
			t.Fatal("transfer-line split has no transfer_id")
		}
		txnRepo := transaction.NewRepository(svc.db)
		paired, err := txnRepo.ListByTransferID(transferID.ID)
		if err != nil {
			t.Fatalf("ListByTransferID: %v", err)
		}
		if len(paired) != 1 {
			t.Fatalf("paired count = %d, want 1", len(paired))
		}
		if paired[0].Date != customDate {
			t.Errorf("paired date = %s, want %s",
				paired[0].Date.String(), customDate.String())
		}
	})
}

func TestScheduledService_ValidateMultiLine_RejectsBothShapes(t *testing.T) {
	t.Run("scalar category plus child splits rejected", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		parentCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(parentCat); err != nil {
			t.Fatalf("Create parent category: %v", err)
		}
		lineCat := category.NewCategory("Bonus", category.TypeIncome)
		if err := categoryRepo.Create(lineCat); err != nil {
			t.Fatalf("Create line category: %v", err)
		}

		amount, _ := types.NewMoney("100.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
		// Set scalar category AND child splits — the forbidden mixed shape.
		st.SetCategory(parentCat.ID)
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, lineCat.ID, amount),
		}

		err := svc.Create(st)
		if err == nil {
			t.Fatal("expected mixed-shape validation error, got nil")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("expected ServiceValidationError, got %T: %v", err, err)
		}

		// Parent must not have been persisted.
		_, getErr := svc.GetByID(st.ID)
		if getErr == nil {
			t.Error("mixed-shape create should not persist the parent")
		}
	})

	t.Run("update from single-line to mixed shape rejected", func(t *testing.T) {
		svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)
		acct := createTestAccountForScheduled(t, accountRepo, "Checking")

		parentCat := category.NewCategory("Salary", category.TypeIncome)
		if err := categoryRepo.Create(parentCat); err != nil {
			t.Fatalf("Create parent category: %v", err)
		}
		lineCat := category.NewCategory("Bonus", category.TypeIncome)
		if err := categoryRepo.Create(lineCat); err != nil {
			t.Fatalf("Create line category: %v", err)
		}

		amount, _ := types.NewMoney("100.00")
		st := NewTransactionWithAmount(acct.ID, FrequencyMonthly, types.Today(), amount)
		st.SetCategory(parentCat.ID)
		if err := svc.Create(st); err != nil {
			t.Fatalf("Create single-line: %v", err)
		}

		// Now try to update by attaching splits while leaving category set.
		st.Splits = SplitCollection{
			NewCategorizedSplit(st.ID, lineCat.ID, amount),
		}
		err := svc.Update(st)
		if err == nil {
			t.Fatal("expected mixed-shape validation error on update, got nil")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("expected ServiceValidationError, got %T: %v", err, err)
		}
	})
}
