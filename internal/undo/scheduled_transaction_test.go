package undo_test

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Test Helpers
// =============================================================================

type scheduledTestEnv struct {
	database     *db.DB
	scheduledSvc *scheduled.Service
	txnSvc       *transaction.Service
	transferSvc  *transfer.Service
	accountRepo  *account.Repository
	payeeRepo    *payee.Repository
	categoryRepo *category.Repository
}

func createScheduledTestEnv(t *testing.T) *scheduledTestEnv {
	t.Helper()
	database := createTestDB(t)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	scheduledRepo := scheduled.NewRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
	scheduledSvc := scheduled.NewService(scheduledRepo, txnRepo, txnSvc, database, accountRepo)
	transferSvc := transfer.NewService(txnRepo, investment.NewRepository(database),
		splitRepo, accountRepo, categoryRepo, database)
	scheduledSvc.SetTransferPort(transferSvc)

	return &scheduledTestEnv{
		database:     database,
		scheduledSvc: scheduledSvc,
		txnSvc:       txnSvc,
		transferSvc:  transferSvc,
		accountRepo:  accountRepo,
		payeeRepo:    payeeRepo,
		categoryRepo: categoryRepo,
	}
}

func createScheduledTestAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

// pastDate returns a date in the past so scheduled transactions are due.
func pastDate() types.Date {
	return types.NewDate(2024, time.January, 1)
}

// =============================================================================
// CreateScheduledTransactionCommand Tests
// =============================================================================

func TestCreateScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("creates and then deletes scheduled transaction", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-100.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)

		cmd := undo.NewCreateScheduledTransactionCommand(env.scheduledSvc, st)

		// Execute: scheduled transaction should exist
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		retrieved, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after Execute error = %v", err)
		}
		if retrieved.Frequency != scheduled.FrequencyMonthly {
			t.Errorf("frequency = %q, want %q", retrieved.Frequency, scheduled.FrequencyMonthly)
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
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-50.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyWeekly, pastDate(), amount)

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
		if retrieved.Frequency != scheduled.FrequencyWeekly {
			t.Errorf("frequency after redo = %q, want %q", retrieved.Frequency, scheduled.FrequencyWeekly)
		}
	})
}

// =============================================================================
// EditScheduledTransactionCommand Tests
// =============================================================================

func TestEditScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("edits and then restores original state", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-100.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)
		st.SetMemo("Original memo")
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Build the edited version
		edited, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		newAmount := types.MustNewMoney("-200.00")
		edited.Amount = types.NullableMoney{Money: newAmount, Valid: true}
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
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-75.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)
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
	cmd := undo.NewDeleteScheduledTransactionCommand(nil, types.NewID())
	if cmd.Description() != "Delete scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Delete scheduled transaction")
	}
}

// =============================================================================
// SkipScheduledTransactionCommand Tests
// =============================================================================

func TestSkipScheduledTransactionCommand_ExecuteAndUndo(t *testing.T) {
	t.Run("skips and then restores schedule state", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-100.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)
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
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-50.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)
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
	cmd := undo.NewSkipScheduledTransactionCommand(nil, types.NewID())
	if cmd.Description() != "Skip scheduled transaction" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Skip scheduled transaction")
	}
}

func TestSkipScheduledTransactionCommand_WithManager(t *testing.T) {
	t.Run("skip via manager undo/redo cycle", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-60.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastDate(), amount)
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
