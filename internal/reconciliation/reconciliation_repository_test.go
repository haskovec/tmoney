package reconciliation

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

// createRepoTestAccount creates a test account in the database and returns it.
func createRepoTestAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	acct := account.NewAccount(
		name,
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.NewDate(2024, 1, 1),
	)
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

// =============================================================================
// Repository CRUD Tests
// =============================================================================

func TestRepository_Create(t *testing.T) {
	t.Run("creates valid session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
		)

		err := repo.Create(session)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := repo.GetByID(session.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.AccountID != acct.ID {
			t.Errorf("Expected account ID %v, got %v", acct.ID, retrieved.AccountID)
		}
		if retrieved.Status != SessionStatusInProgress {
			t.Errorf("Expected status in_progress, got %q", retrieved.Status)
		}
		if !retrieved.StatementBalance.Equal(types.MustNewMoney("5234.56")) {
			t.Errorf("Expected statement balance 5234.56, got %s", retrieved.StatementBalance.String())
		}
	})

	t.Run("rejects non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		session := NewSession(
			types.NewID(),
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("1000"),
		)

		err := repo.Create(session)
		if err == nil {
			t.Error("Create() expected error for non-existent account")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetByID(t *testing.T) {
	t.Run("retrieves existing session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
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
		repo := NewRepository(database)

		_, err := repo.GetByID(types.NewID())
		if err == nil {
			t.Error("GetByID() expected error for non-existent session")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_GetActiveByAccountID(t *testing.T) {
	t.Run("returns active session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
		)
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		active, err := repo.GetActiveByAccountID(acct.ID)
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
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")

		active, err := repo.GetActiveByAccountID(acct.ID)
		if err != nil {
			t.Fatalf("GetActiveByAccountID() error = %v", err)
		}
		if active != nil {
			t.Error("GetActiveByAccountID() expected nil for account with no sessions")
		}
	})

	t.Run("does not return completed sessions", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
		)
		session.Complete()
		if err := repo.Create(session); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		active, err := repo.GetActiveByAccountID(acct.ID)
		if err != nil {
			t.Fatalf("GetActiveByAccountID() error = %v", err)
		}
		if active != nil {
			t.Error("GetActiveByAccountID() should not return completed sessions")
		}
	})
}

func TestRepository_Update(t *testing.T) {
	t.Run("updates session status to completed", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("5234.56"),
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
		if retrieved.Status != SessionStatusCompleted {
			t.Errorf("Expected status completed, got %q", retrieved.Status)
		}
		if !retrieved.CompletedAt.Valid {
			t.Error("CompletedAt should be set after completing")
		}
	})

	t.Run("returns NotFoundError for non-existent session", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		session := NewSession(
			types.NewID(),
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("1000"),
		)

		err := repo.Update(session)
		if err == nil {
			t.Error("Update() expected error for non-existent session")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_Delete(t *testing.T) {
	t.Run("deletes existing session", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")
		session := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("1000"),
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
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("returns NotFoundError for non-existent session", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		err := repo.Delete(types.NewID())
		if err == nil {
			t.Error("Delete() expected error for non-existent session")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Errorf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestRepository_DeleteByAccountID(t *testing.T) {
	t.Run("deletes all sessions for account", func(t *testing.T) {
		database := createTestDB(t)
		accountRepo := account.NewRepository(database)
		repo := NewRepository(database)

		acct := createRepoTestAccount(t, accountRepo, "Checking")

		session1 := NewSession(
			acct.ID,
			types.NewDate(2024, 1, 31),
			types.MustNewMoney("1000"),
		)
		session1.Complete()
		if err := repo.Create(session1); err != nil {
			t.Fatalf("Create() session1 error = %v", err)
		}

		session2 := NewSession(
			acct.ID,
			types.NewDate(2024, 2, 28),
			types.MustNewMoney("2000"),
		)
		if err := repo.Create(session2); err != nil {
			t.Fatalf("Create() session2 error = %v", err)
		}

		count, err := repo.DeleteByAccountID(acct.ID)
		if err != nil {
			t.Fatalf("DeleteByAccountID() error = %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 deleted, got %d", count)
		}

		sessions, err := repo.ListByAccountID(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccountID() error = %v", err)
		}
		if len(sessions) != 0 {
			t.Errorf("Expected 0 sessions after delete, got %d", len(sessions))
		}
	})

	t.Run("returns 0 when no sessions to delete", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)

		count, err := repo.DeleteByAccountID(types.NewID())
		if err != nil {
			t.Fatalf("DeleteByAccountID() error = %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 deleted, got %d", count)
		}
	})
}
