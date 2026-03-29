package undo_test

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// AutoPostCommand Tests
// =============================================================================

func TestAutoPostCommand_Description(t *testing.T) {
	summary := &scheduled.AutoPostSummary{PostedCount: 3}
	cmd := undo.NewAutoPostCommand(nil, nil, summary)
	want := "Auto-post 3 scheduled transaction(s)"
	if cmd.Description() != want {
		t.Errorf("Description() = %q, want %q", cmd.Description(), want)
	}
}

func TestAutoPostCommand_DescriptionSingle(t *testing.T) {
	summary := &scheduled.AutoPostSummary{PostedCount: 1}
	cmd := undo.NewAutoPostCommand(nil, nil, summary)
	want := "Auto-post 1 scheduled transaction(s)"
	if cmd.Description() != want {
		t.Errorf("Description() = %q, want %q", cmd.Description(), want)
	}
}

func TestAutoPostCommand_ExecuteIsNoop(t *testing.T) {
	summary := &scheduled.AutoPostSummary{PostedCount: 0}
	cmd := undo.NewAutoPostCommand(nil, nil, summary)
	if err := cmd.Execute(); err != nil {
		t.Errorf("Execute() should be no-op, got error = %v", err)
	}
}

func TestAutoPostCommand_UndoDeletesTransactionsAndRestoresSchedule(t *testing.T) {
	t.Run("undoes single auto-posted transaction", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		// Create a scheduled transaction and auto-post it
		amount := types.MustNewMoney("-500.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), amount)
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		// Run auto-post (this creates transactions and modifies the schedule)
		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}
		if summary.PostedCount != 1 {
			t.Fatalf("Expected 1 posted, got %d", summary.PostedCount)
		}

		// Verify the transaction was created
		txn := summary.Results[0].Transactions[0]
		_, err = env.txnSvc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("Transaction should exist after auto-post: %v", err)
		}

		// Verify schedule was advanced
		updatedST, _ := env.scheduledSvc.GetByID(st.ID)
		if updatedST.NextDate == originalNextDate {
			t.Fatal("Schedule should have been advanced after auto-post")
		}

		// Create the undo command and undo
		cmd := undo.NewAutoPostCommand(env.txnSvc, env.scheduledSvc, summary)
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// Transaction should be deleted
		_, err = env.txnSvc.GetByID(txn.ID)
		if err == nil {
			t.Error("Transaction should not exist after undo")
		}

		// Schedule should be restored
		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}
	})
}

func TestAutoPostCommand_UndoMultipleOverdueOccurrences(t *testing.T) {
	t.Run("undoes multiple overdue occurrences", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-100.00")
		// Start date 90 days ago to trigger multiple overdue posts
		pastStartDate := types.NewDate(time.Now().AddDate(0, 0, -90).Year(),
			time.Now().AddDate(0, 0, -90).Month(),
			time.Now().AddDate(0, 0, -90).Day())
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, pastStartDate, amount)
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		// Run auto-post
		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}
		if summary.PostedCount < 3 {
			t.Fatalf("Expected at least 3 posted, got %d", summary.PostedCount)
		}

		// Collect all created transaction IDs
		var txnIDs []types.ID
		for _, result := range summary.Results {
			for _, txn := range result.Transactions {
				txnIDs = append(txnIDs, txn.ID)
			}
		}

		// Verify all transactions exist
		for _, id := range txnIDs {
			if _, err := env.txnSvc.GetByID(id); err != nil {
				t.Fatalf("Transaction %s should exist: %v", id.String(), err)
			}
		}

		// Undo the entire auto-post session
		cmd := undo.NewAutoPostCommand(env.txnSvc, env.scheduledSvc, summary)
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// All transactions should be deleted
		for _, id := range txnIDs {
			if _, err := env.txnSvc.GetByID(id); err == nil {
				t.Errorf("Transaction %s should not exist after undo", id.String())
			}
		}

		// Schedule should be restored to original state
		restoredST, err := env.scheduledSvc.GetByID(st.ID)
		if err != nil {
			t.Fatalf("GetByID() after undo error = %v", err)
		}
		if restoredST.NextDate != originalNextDate {
			t.Errorf("next_date after undo = %v, want %v", restoredST.NextDate, originalNextDate)
		}
	})
}

func TestAutoPostCommand_UndoMultipleScheduledTransactions(t *testing.T) {
	t.Run("undoes auto-post across multiple scheduled transactions", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount1 := types.MustNewMoney("-500.00")
		amount2 := types.MustNewMoney("-100.00")

		st1 := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), amount1)
		st1.SetAutoPost(true)
		st1.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st1); err != nil {
			t.Fatalf("Create(st1) error = %v", err)
		}

		st2 := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), amount2)
		st2.SetAutoPost(true)
		st2.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st2); err != nil {
			t.Fatalf("Create(st2) error = %v", err)
		}

		originalNextDate1 := st1.NextDate
		originalNextDate2 := st2.NextDate

		// Run auto-post
		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}
		if summary.PostedCount != 2 {
			t.Fatalf("Expected 2 posted, got %d", summary.PostedCount)
		}

		// Undo
		cmd := undo.NewAutoPostCommand(env.txnSvc, env.scheduledSvc, summary)
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() error = %v", err)
		}

		// Both schedules should be restored
		restored1, _ := env.scheduledSvc.GetByID(st1.ID)
		if restored1.NextDate != originalNextDate1 {
			t.Errorf("st1 next_date after undo = %v, want %v", restored1.NextDate, originalNextDate1)
		}
		restored2, _ := env.scheduledSvc.GetByID(st2.ID)
		if restored2.NextDate != originalNextDate2 {
			t.Errorf("st2 next_date after undo = %v, want %v", restored2.NextDate, originalNextDate2)
		}

		// All transactions should be deleted
		for _, result := range summary.Results {
			for _, txn := range result.Transactions {
				if _, err := env.txnSvc.GetByID(txn.ID); err == nil {
					t.Errorf("Transaction %s should not exist after undo", txn.ID.String())
				}
			}
		}
	})
}

func TestAutoPostCommand_WithManagerPush(t *testing.T) {
	t.Run("pushed auto-post command can be undone via manager", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-250.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), amount)
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Run auto-post
		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}
		if summary.PostedCount != 1 {
			t.Fatalf("Expected 1 posted, got %d", summary.PostedCount)
		}

		txnID := summary.Results[0].Transactions[0].ID

		// Push to manager (not Execute, since auto-post already ran)
		mgr := undo.NewManager()
		cmd := undo.NewAutoPostCommand(env.txnSvc, env.scheduledSvc, summary)
		mgr.Push(cmd)

		if !mgr.CanUndo() {
			t.Fatal("Should be able to undo after Push")
		}
		if mgr.UndoDescription() != "Auto-post 1 scheduled transaction(s)" {
			t.Errorf("UndoDescription() = %q", mgr.UndoDescription())
		}

		// Undo via manager
		desc, err := mgr.Undo()
		if err != nil {
			t.Fatalf("Manager.Undo() error = %v", err)
		}
		if desc != "Auto-post 1 scheduled transaction(s)" {
			t.Errorf("undo desc = %q", desc)
		}

		// Transaction should be gone
		_, err = env.txnSvc.GetByID(txnID)
		if err == nil {
			t.Error("Transaction should not exist after undo")
		}

		// Redo should be available (Execute is no-op, but it goes back on undo stack)
		if !mgr.CanRedo() {
			t.Error("Should be able to redo after undo")
		}
	})
}

func TestAutoPostCommand_SkippedResultsNotUndone(t *testing.T) {
	t.Run("skipped results do not affect undo", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		// Create a variable-amount scheduled transaction (will be skipped)
		st := scheduled.NewTransaction(acct.ID, scheduled.FrequencyMonthly, types.Today())
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Run auto-post - should skip (no amount, no estimate)
		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}

		// Create undo command (should handle gracefully even with no transactions)
		cmd := undo.NewAutoPostCommand(env.txnSvc, env.scheduledSvc, summary)
		if err := cmd.Undo(); err != nil {
			t.Fatalf("Undo() should succeed even with only skipped results, got error = %v", err)
		}
	})
}

func TestAutoPostCommand_BeforeScheduleCaptured(t *testing.T) {
	t.Run("AutoPost captures BeforeSchedule in results", func(t *testing.T) {
		env := createScheduledTestEnv(t)
		acct := createScheduledTestAccount(t, env.accountRepo, "Checking")

		amount := types.MustNewMoney("-300.00")
		st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), amount)
		st.SetAutoPost(true)
		st.SetPostLeadDays(0)
		if err := env.scheduledSvc.Create(st); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		originalNextDate := st.NextDate

		summary, err := env.scheduledSvc.AutoPost()
		if err != nil {
			t.Fatalf("AutoPost() error = %v", err)
		}

		if len(summary.Results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(summary.Results))
		}

		result := summary.Results[0]
		if result.BeforeSchedule == nil {
			t.Fatal("BeforeSchedule should not be nil")
		}
		if result.BeforeSchedule.NextDate != originalNextDate {
			t.Errorf("BeforeSchedule.NextDate = %v, want %v", result.BeforeSchedule.NextDate, originalNextDate)
		}
		if result.BeforeSchedule.ID != st.ID {
			t.Errorf("BeforeSchedule.ID = %v, want %v", result.BeforeSchedule.ID, st.ID)
		}
	})
}
