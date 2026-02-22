package undo_test

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

type scheduledTestEnv struct {
	scheduledSvc *service.ScheduledTransactionService
	txnSvc       *service.TransactionService
	accountRepo  *repository.AccountRepository
	payeeRepo    *repository.PayeeRepository
	categoryRepo *repository.CategoryRepository
}

func createScheduledTestEnv(t *testing.T) *scheduledTestEnv {
	t.Helper()
	database := createTestDB(t)
	txnRepo := repository.NewTransactionRepository(database)
	splitRepo := repository.NewSplitRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	payeeRepo := repository.NewPayeeRepository(database)
	accountRepo := repository.NewAccountRepository(database)
	categoryRepo := repository.NewCategoryRepository(database)
	scheduledRepo := repository.NewScheduledTransactionRepository(database)

	txnSvc := service.NewTransactionService(txnRepo, splitRepo, transferRepo, payeeRepo, database)
	scheduledSvc := service.NewScheduledTransactionService(scheduledRepo, txnRepo, database)

	return &scheduledTestEnv{
		scheduledSvc: scheduledSvc,
		txnSvc:       txnSvc,
		accountRepo:  accountRepo,
		payeeRepo:    payeeRepo,
		categoryRepo: categoryRepo,
	}
}

func createScheduledTestAccount(t *testing.T, repo *repository.AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(name, models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return account
}

func createScheduledTestPayee(t *testing.T, repo *repository.PayeeRepository, name string) *models.Payee {
	t.Helper()
	payee := models.NewPayee(name)
	if err := repo.Create(payee); err != nil {
		t.Fatalf("Failed to create test payee: %v", err)
	}
	return payee
}

func createScheduledTestCategory(t *testing.T, repo *repository.CategoryRepository, name string) *models.Category {
	t.Helper()
	cat := models.NewCategory(name, models.CategoryTypeExpense)
	if err := repo.Create(cat); err != nil {
		t.Fatalf("Failed to create test category: %v", err)
	}
	return cat
}

// pastDate returns a date in the past so scheduled transactions are due.
func pastDate() models.Date {
	return models.NewDate(2024, time.January, 1)
}

// =============================================================================
// CreateScheduledTransactionCommand Tests
// =============================================================================

func TestCreateScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes scheduled transaction", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-100.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)

		cmd := undo.NewCreateScheduledTransactionCommand(env.scheduledSvc, st)

		// Execute: scheduled transaction should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Frequency != models.FrequencyMonthly {
			t.Errorf("frequency = %q, want %q", retrieved.Frequency, models.FrequencyMonthly)
		}

		// Undo: scheduled transaction should be gone
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.scheduledSvc.GetByID(st.ID)
		if err == nil {
			t.Error("expected error after Undo (scheduled transaction should be deleted)")
		}
	})
}

func TestCreateScheduledTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewCreateScheduledTransactionCommand(nil, nil)
	if cmd.Description() != "Create scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Create scheduled transaction")
	}
}

func TestCreateScheduledTransactionCommand_WithManager(t *testing.T) {
	t.Run("works with undo manager execute and undo", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyWeekly, pastDate(), amount)

		mgr := undo.NewManager()
		cmd := undo.NewCreateScheduledTransactionCommand(env.scheduledSvc, st)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		if !mgr.CanUndo() {
			t.Error("should be able to undo after execute")
		}

		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Create scheduled transaction" {
			t.Errorf("undo desc = %q, want %q", desc, "Create scheduled transaction")
		}

		_, err = env.scheduledSvc.GetByID(st.ID)
		if err == nil {
			t.Error("scheduled transaction should not exist after undo")
		}

		// Redo should recreate
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Create scheduled transaction" {
			t.Errorf("redo desc = %q, want %q", desc, "Create scheduled transaction")
		}

		retrieved, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if retrieved.Frequency != models.FrequencyWeekly {
			t.Errorf("frequency after redo = %q, want %q", retrieved.Frequency, models.FrequencyWeekly)
		}
	})
}

// =============================================================================
// EditScheduledTransactionCommand Tests
// =============================================================================

func TestEditScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-100.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		st.SetMemo("Original memo")
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		newAmount := models.MustNewMoney("-200.00")
		edited.Amount = models.NullableMoney{Money: newAmount, Valid: true}
		edited.SetMemo("Updated memo")

		cmd := undo.NewEditScheduledTransactionCommand(env.scheduledSvc, edited)

		// Execute: should be edited
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if !retrieved.Amount.Money.Equal(newAmount) {
			t.Errorf("amount after edit = %s, want %s", retrieved.Amount.Money.String(), newAmount.String())
		}
		if retrieved.Memo.String != "Updated memo" {
			t.Errorf("memo after edit = %q, want %q", retrieved.Memo.String, "Updated memo")
		}

		// Undo: should restore original
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Money.Equal(amount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.Money.String(), amount.String())
		}
		if restored.Memo.String != "Original memo" {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, "Original memo")
		}
	})
}

func TestEditScheduledTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewEditScheduledTransactionCommand(nil, nil)
	if cmd.Description() != "Edit scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Edit scheduled transaction")
	}
}

// =============================================================================
// DeleteScheduledTransactionCommand Tests
// =============================================================================

func TestDeleteScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("deletes and then recreates scheduled transaction", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-75.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		st.SetMemo("Test memo")
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewDeleteScheduledTransactionCommand(env.scheduledSvc, st.ID)

		// Execute: scheduled transaction should be gone
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		_, err := env.scheduledSvc.GetByID(st.ID)
		if err == nil {
			t.Error("expected error after Execute (scheduled transaction should be deleted)")
		}

		// Undo: scheduled transaction should be back
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restored, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if !restored.Amount.Money.Equal(amount) {
			t.Errorf("amount after undo = %s, want %s", restored.Amount.Money.String(), amount.String())
		}
		if restored.Memo.String != "Test memo" {
			t.Errorf("memo after undo = %q, want %q", restored.Memo.String, "Test memo")
		}
	})
}

func TestDeleteScheduledTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewDeleteScheduledTransactionCommand(nil, models.NewID())
	if cmd.Description() != "Delete scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete scheduled transaction")
	}
}

// =============================================================================
// PostScheduledTransactionCommand Tests
// =============================================================================

func TestPostScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("posts scheduled transaction and undoes by deleting transaction and restoring schedule", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-150.00")
		startDate := pastDate()
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, startDate, amount)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		cmd := undo.NewPostScheduledTransactionCommand(env.scheduledSvc, env.txnSvc, st.ID, nil)

		// Execute: should create transaction and advance schedule
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		// Verify transaction was created
		createdTxn := cmd.CreatedTransaction()
		if createdTxn == nil {
			t.Fatal("CreatedTransaction() should not be nil after Execute")
		}

		retrieved, err := env.txnSvc.GetByID(createdTxn.ID)
		if err != nil {
			t.Fatalf("GetByID(txn) after Execute error = %v", err)
		}
		if !retrieved.Amount.Equal(amount) {
			t.Errorf("transaction amount = %s, want %s", retrieved.Amount.String(), amount.String())
		}

		// Verify schedule was advanced
		advancedST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID(st) after Execute error = %v", err)
		}
		if advancedST.NextDate == originalNextDate {
			t.Error("schedule should have been advanced after post")
		}

		// Undo: transaction should be deleted and schedule restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// Transaction should be gone
		_, err = env.txnSvc.GetByID(createdTxn.ID)
		if err == nil {
			t.Error("transaction should not exist after undo")
		}

		// Schedule should be restored to original state
		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID(st) after Undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}
	})
}

func TestPostScheduledTransactionCommand_WithOverrideAmount(t *testing.T) {
	t.Run("posts with override amount", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		scheduledAmount := models.MustNewMoney("-100.00")
		overrideAmount := models.MustNewMoney("-125.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), scheduledAmount)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewPostScheduledTransactionCommand(env.scheduledSvc, env.txnSvc, st.ID, &overrideAmount)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		createdTxn := cmd.CreatedTransaction()
		retrieved, err := env.txnSvc.GetByID(createdTxn.ID)
		if err != nil {
			t.Fatalf("GetByID(txn) error = %v", err)
		}
		if !retrieved.Amount.Equal(overrideAmount) {
			t.Errorf("transaction amount = %s, want %s", retrieved.Amount.String(), overrideAmount.String())
		}

		// Undo
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(createdTxn.ID)
		if err == nil {
			t.Error("transaction should not exist after undo")
		}
	})
}

func TestPostScheduledTransactionCommand_WithPayeeAndCategory(t *testing.T) {
	t.Run("posts with payee and category copied to transaction", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")
		payee := createScheduledTestPayee(t, env.payeeRepo, "Electric Co")
		category := createScheduledTestCategory(t, env.categoryRepo, "Utilities")

		amount := models.MustNewMoney("-80.00")
		st := models.NewScheduledTransactionFull(account.ID, models.FrequencyMonthly, pastDate(), amount, payee.ID, category.ID, "Monthly electric bill")
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		cmd := undo.NewPostScheduledTransactionCommand(env.scheduledSvc, env.txnSvc, st.ID, nil)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		createdTxn := cmd.CreatedTransaction()
		retrieved, err := env.txnSvc.GetByID(createdTxn.ID)
		if err != nil {
			t.Fatalf("GetByID(txn) error = %v", err)
		}
		if !retrieved.PayeeID.Valid || retrieved.PayeeID.ID != payee.ID {
			t.Error("transaction should have payee from scheduled transaction")
		}
		if !retrieved.CategoryID.Valid || retrieved.CategoryID.ID != category.ID {
			t.Error("transaction should have category from scheduled transaction")
		}
		if retrieved.Memo.String != "Monthly electric bill" {
			t.Errorf("transaction memo = %q, want %q", retrieved.Memo.String, "Monthly electric bill")
		}

		// Undo should clean up
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		_, err = env.txnSvc.GetByID(createdTxn.ID)
		if err == nil {
			t.Error("transaction should not exist after undo")
		}
	})
}

func TestPostScheduledTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewPostScheduledTransactionCommand(nil, nil, models.NewID(), nil)
	if cmd.Description() != "Post scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Post scheduled transaction")
	}
}

func TestPostScheduledTransactionCommand_CreatedTransactionNilBeforeExecute(t *testing.T) {
	cmd := undo.NewPostScheduledTransactionCommand(nil, nil, models.NewID(), nil)
	if cmd.CreatedTransaction() != nil {
		t.Error("CreatedTransaction() should be nil before Execute")
	}
}

func TestPostScheduledTransactionCommand_WithManager(t *testing.T) {
	t.Run("post via manager undo/redo cycle", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-200.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		mgr := undo.NewManager()
		cmd := undo.NewPostScheduledTransactionCommand(env.scheduledSvc, env.txnSvc, st.ID, nil)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		createdTxn := cmd.CreatedTransaction()

		// Verify post happened
		_, err := env.txnSvc.GetByID(createdTxn.ID)
		if err != nil {
			t.Fatalf("transaction should exist after execute: %v", err)
		}

		// Undo
		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Post scheduled transaction" {
			t.Errorf("undo desc = %q, want %q", desc, "Post scheduled transaction")
		}

		// Transaction gone, schedule restored
		_, err = env.txnSvc.GetByID(createdTxn.ID)
		if err == nil {
			t.Error("transaction should not exist after undo")
		}

		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID(st) after undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}

		// Redo should re-post
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Post scheduled transaction" {
			t.Errorf("redo desc = %q, want %q", desc, "Post scheduled transaction")
		}

		// Transaction should exist again (but with a potentially new ID from re-execute)
		advancedST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID(st) after redo error = %v", err)
		}
		if advancedST.NextDate == originalNextDate {
			t.Error("schedule should be advanced after redo")
		}
	})
}

// =============================================================================
// SkipScheduledTransactionCommand Tests
// =============================================================================

func TestSkipScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("skips and then restores schedule state", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-100.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		cmd := undo.NewSkipScheduledTransactionCommand(env.scheduledSvc, st.ID)

		// Execute: schedule should advance
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		skippedST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if skippedST.NextDate == originalNextDate {
			t.Error("schedule should have been advanced after skip")
		}

		// Undo: schedule should be restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}
	})
}

func TestSkipScheduledTransactionCommand_WithOccurrences(t *testing.T) {
	t.Run("skip restores occurrences remaining on undo", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-50.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		st.SetOccurrences(3)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalRemaining := st.OccurrencesRemaining.Int64

		cmd := undo.NewSkipScheduledTransactionCommand(env.scheduledSvc, st.ID)

		// Execute: occurrences should decrement
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		skippedST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if skippedST.OccurrencesRemaining.Int64 >= originalRemaining {
			t.Error("occurrences_remaining should have decreased after skip")
		}

		// Undo: occurrences should be restored
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Undo error = %v", err)
		}
		if restoredST.OccurrencesRemaining.Int64 != originalRemaining {
			t.Errorf("occurrences_remaining after undo = %d, want %d", restoredST.OccurrencesRemaining.Int64, originalRemaining)
		}
	})
}

func TestSkipScheduledTransactionCommand_Description(t *testing.T) {
	cmd := undo.NewSkipScheduledTransactionCommand(nil, models.NewID())
	if cmd.Description() != "Skip scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Skip scheduled transaction")
	}
}

func TestSkipScheduledTransactionCommand_WithManager(t *testing.T) {
	t.Run("skip via manager undo/redo cycle", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		account := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := models.MustNewMoney("-60.00")
		st := models.NewScheduledTransactionWithAmount(account.ID, models.FrequencyMonthly, pastDate(), amount)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		mgr := undo.NewManager()
		cmd := undo.NewSkipScheduledTransactionCommand(env.scheduledSvc, st.ID)

		if err := mgr.Execute(cmd); err != nil {
			t.Fatalf("Manager.Execute() error = %v", err)
		}

		// Verify skip
		skippedST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after execute error = %v", err)
		}
		if skippedST.NextDate == originalNextDate {
			t.Error("schedule should be advanced after skip")
		}

		// Undo
		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Skip scheduled transaction" {
			t.Errorf("undo desc = %q, want %q", desc, "Skip scheduled transaction")
		}

		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}

		// Redo
		desc, err = mgr.Redo()
		if err != nil {
			t.Fatalf("Manager.Redo() error = %v", err)
		}
		if desc != "Skip scheduled transaction" {
			t.Errorf("redo desc = %q, want %q", desc, "Skip scheduled transaction")
		}

		redoST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after redo error = %v", err)
		}
		if redoST.NextDate == originalNextDate {
			t.Error("schedule should be advanced after redo")
		}
	})
}
