package repository

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// createTestAccount creates a test account in the database and returns it.
func createTestAccount(t *testing.T, repo *AccountRepository, name string) *models.Account {
	t.Helper()
	account := models.NewAccount(
		name,
		models.AccountTypeChecking,
		"USD",
		models.MustNewMoney("1000.00"),
		models.NewDate(2024, 1, 1),
	)
	if err := repo.Create(account); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return account
}

// =============================================================================
// ReconciliationRepository CRUD Tests
// =============================================================================

func TestReconciliationRepository_Create(t *testing.T) {
	t.Run("creates valid session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("5234.56"),
		)

		err := repo.Create(session)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(session.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != account.ID {
			t.Errorf("Expected account ID %v, got %v", account.ID, retrieved.AccountID)
		}
		if retrieved.Status != models.ReconciliationStatusInProgress {
			t.Errorf("Expected status in_progress, got %q", retrieved.Status)
		}
		if !retrieved.StatementBalance.Equal(models.MustNewMoney("5234.56")) {
			t.Errorf("Expected statement balance 5234.56, got %s", retrieved.StatementBalance.String())
		}
	})

	t.Run("rejects non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewReconciliationRepository(database)

		session := models.NewReconciliationSession(
			models.NewID(),
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)

		err := repo.Create(session)
		if err == nil {
			t.Error("Create() expected error for non-existent account")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestReconciliationRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("5234.56"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(session.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ID != session.ID {
			t.Errorf("Expected ID %v, got %v", session.ID, retrieved.ID)
		}
	})

	t.Run("returns NotFoundError for non-existent session", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewReconciliationRepository(database)

		_, err := repo.GetByID(models.NewID())
		if err == nil {
			t.Error("GetByID() expected error for non-existent session")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestReconciliationRepository_GetActiveByAccountID(t *testing.T) {
	t.Run("returns active session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("5234.56"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		active, err := repo.GetActiveByAccountID(account.ID)
		if err != nil {
			t.Fatalf("GetActiveByAccountID() error = %v", err)
		}
		if active == nil {
			t.Fatal("GetActiveByAccountID() returned nil, expected session")
		}
		if active.ID != session.ID {
			t.Errorf("Expected ID %v, got %v", session.ID, active.ID)
		}
	})

	t.Run("returns nil when no active session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		active, err := repo.GetActiveByAccountID(account.ID)
		if err != nil {
			t.Fatalf("GetActiveByAccountID() error = %v", err)
		}
		if active != nil {
			t.Error("GetActiveByAccountID() expected nil for account with no sessions")
		}
	})

	t.Run("does not return completed sessions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("5234.56"),
		)
		session.Complete()
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		active, err := repo.GetActiveByAccountID(account.ID)
		if err != nil {
			t.Fatalf("GetActiveByAccountID() error = %v", err)
		}
		if active != nil {
			t.Error("GetActiveByAccountID() should not return completed sessions")
		}
	})
}

func TestReconciliationRepository_GetLastCompletedByAccountID(t *testing.T) {
	t.Run("returns last completed session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		// Create and complete first session
		session1 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		session1.Complete()
		if err := repo.Create(session1); err != nil {
			t.Fatalf("Create() session1 error = %v", err)
		}

		// Create and complete second session
		session2 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 2, 28),
			models.MustNewMoney("2000"),
		)
		session2.Complete()
		if err := repo.Create(session2); err != nil {
			t.Fatalf("Create() session2 error = %v", err)
		}

		last, err := repo.GetLastCompletedByAccountID(account.ID)
		if err != nil {
			t.Fatalf("GetLastCompletedByAccountID() error = %v", err)
		}
		if last == nil {
			t.Fatal("GetLastCompletedByAccountID() returned nil, expected session")
		}
		// Should return most recent (session2)
		if last.ID != session2.ID {
			t.Errorf("Expected last completed session ID %v, got %v", session2.ID, last.ID)
		}
	})

	t.Run("returns nil when no completed sessions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		// Create in-progress session (not completed)
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		last, err := repo.GetLastCompletedByAccountID(account.ID)
		if err != nil {
			t.Fatalf("GetLastCompletedByAccountID() error = %v", err)
		}
		if last != nil {
			t.Error("GetLastCompletedByAccountID() should return nil when no completed sessions exist")
		}
	})
}

func TestReconciliationRepository_ListByAccountID(t *testing.T) {
	t.Run("lists all sessions for account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		session1 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		session1.Complete()
		if err := repo.Create(session1); err != nil {
			t.Fatalf("Create() session1 error = %v", err)
		}

		session2 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 2, 28),
			models.MustNewMoney("2000"),
		)
		if err := repo.Create(session2); err != nil {
			t.Fatalf("Create() session2 error = %v", err)
		}

		sessions, err := repo.ListByAccountID(account.ID)
		if err != nil {
			t.Fatalf("ListByAccountID() error = %v", err)
		}
		if len(sessions) != 2 {
			t.Errorf("Expected 2 sessions, got %d", len(sessions))
		}
	})

	t.Run("returns empty list for account with no sessions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		sessions, err := repo.ListByAccountID(account.ID)
		if err != nil {
			t.Fatalf("ListByAccountID() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("Expected 0 sessions, got %d", len(sessions))
		}
	})

	t.Run("does not include sessions from other accounts", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account1 := createTestAccount(t, accountRepo, "Checking")
		account2 := createTestAccount(t, accountRepo, "Savings")

		session1 := models.NewReconciliationSession(
			account1.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		if err := repo.Create(session1); err != nil {
			t.Fatalf("Create() session1 error = %v", err)
		}

		session2 := models.NewReconciliationSession(
			account2.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("2000"),
		)
		if err := repo.Create(session2); err != nil {
			t.Fatalf("Create() session2 error = %v", err)
		}

		sessions, err := repo.ListByAccountID(account1.ID)
		if err != nil {
			t.Fatalf("ListByAccountID() error = %v", err)
		}
		if len(sessions) != 1 {
			t.Errorf("Expected 1 session for account1, got %d", len(sessions))
		}
		if sessions[0].AccountID != account1.ID {
			t.Errorf("Expected account ID %v, got %v", account1.ID, sessions[0].AccountID)
		}
	})
}

func TestReconciliationRepository_Update(t *testing.T) {
	t.Run("updates session status to completed", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("5234.56"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		session.Complete()
		err := repo.Update(session)
		if err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := repo.GetByID(session.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Status != models.ReconciliationStatusCompleted {
			t.Errorf("Expected status completed, got %q", retrieved.Status)
		}
		if !retrieved.CompletedAt.Valid {
			t.Error("CompletedAt should be set after completing")
		}
	})

	t.Run("returns NotFoundError for non-existent session", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewReconciliationRepository(database)

		session := models.NewReconciliationSession(
			models.NewID(),
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)

		err := repo.Update(session)
		if err == nil {
			t.Error("Update() expected error for non-existent session")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestReconciliationRepository_Delete(t *testing.T) {
	t.Run("deletes existing session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")
		session := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := repo.Delete(session.ID)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err = repo.GetByID(session.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent session", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewReconciliationRepository(database)

		err := repo.Delete(models.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent session")
		}
		if _, ok := err.(*NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestReconciliationRepository_DeleteByAccountID(t *testing.T) {
	t.Run("deletes all sessions for account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := NewAccountRepository(database)
		repo := NewReconciliationRepository(database)

		account := createTestAccount(t, accountRepo, "Checking")

		session1 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 1, 31),
			models.MustNewMoney("1000"),
		)
		session1.Complete()
		if err := repo.Create(session1); err != nil {
			t.Fatalf("Create() session1 error = %v", err)
		}

		session2 := models.NewReconciliationSession(
			account.ID,
			models.NewDate(2024, 2, 28),
			models.MustNewMoney("2000"),
		)
		if err := repo.Create(session2); err != nil {
			t.Fatalf("Create() session2 error = %v", err)
		}

		count, err := repo.DeleteByAccountID(account.ID)
		if err != nil {
			t.Fatalf("DeleteByAccountID() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 deleted, got %d", count)
		}

		sessions, err := repo.ListByAccountID(account.ID)
		if err != nil {
			t.Fatalf("ListByAccountID() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("Expected 0 sessions after delete, got %d", len(sessions))
		}
	})

	t.Run("returns 0 when no sessions to delete", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewReconciliationRepository(database)

		count, err := repo.DeleteByAccountID(models.NewID())
		if err != nil {
			t.Fatalf("DeleteByAccountID() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 deleted, got %d", count)
		}
	})
}
