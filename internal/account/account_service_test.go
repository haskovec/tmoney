package account

import (
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

func TestNewService(t *testing.T) {
	t.Run("creates service with repository and db", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		if svc == nil {
			t.Fatal("NewService should not return nil")
		}
		if svc.repo != repo {
			t.Error("NewService should store repository")
		}
		if svc.db != database {
			t.Error("NewService should store database")
		}
	})
}

func TestService_Create(t *testing.T) {
	t.Run("creates valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount(
			"Test Checking",
			TypeChecking,
			"USD",
			types.MustNewMoney("1000.00"),
			types.NewDate(2024, 1, 15),
		)

		err := svc.Create(account)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Test Checking" {
			t.Errorf("Expected name 'Test Checking', got %q", retrieved.Name)
		}
		if retrieved.Type != TypeChecking {
			t.Errorf("Expected type checking, got %q", retrieved.Type)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", retrieved.Currency)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("", TypeChecking, "USD", types.ZeroMoney, types.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects invalid currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Test", TypeChecking, "INVALID", types.ZeroMoney, types.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for invalid currency")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects invalid account type", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Test", Type("invalid"), "USD", types.ZeroMoney, types.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for invalid account type")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("creates account with optional fields", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Full Account", TypeChecking, "USD", types.ZeroMoney, types.Today())
		account.SetInstitution("First Bank")
		account.SetAccountNumber("12345")
		account.SetNotes("Main checking")

		err := svc.Create(account)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Institution.Valid || retrieved.Institution.String != "First Bank" {
			t.Errorf("Expected institution 'First Bank', got %v", retrieved.Institution)
		}
		if !retrieved.AccountNumber.Valid || retrieved.AccountNumber.String != "12345" {
			t.Errorf("Expected account number '12345', got %v", retrieved.AccountNumber)
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Main checking" {
			t.Errorf("Expected notes 'Main checking', got %v", retrieved.Notes)
		}
	})
}

func TestService_GetByName(t *testing.T) {
	t.Run("retrieves account by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("My Savings", TypeSavings, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := svc.GetByName("My Savings")
		if err != nil {
			t.Fatalf("GetByName() error = %v", err)
		}
		if retrieved.ID != account.ID {
			t.Errorf("Expected ID %s, got %s", account.ID.String(), retrieved.ID.String())
		}
	})

	t.Run("returns error for non-existent name", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		_, err := svc.GetByName("Does Not Exist")
		if err == nil {
			t.Error("GetByName() expected error for non-existent name")
		}
	})
}

func TestService_Update(t *testing.T) {
	t.Run("updates valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Original", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		account.Name = "Updated"
		account.SetNotes("Added notes")
		if err := svc.Update(account); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "Updated" {
			t.Errorf("Expected name 'Updated', got %q", retrieved.Name)
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Added notes" {
			t.Errorf("Expected notes 'Added notes', got %v", retrieved.Notes)
		}
	})

	t.Run("rejects invalid update", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Valid", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		account.Name = "" // Invalid
		err := svc.Update(account)
		if err == nil {
			t.Error("Update() expected error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("renames account that has transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Old Name", TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		_, err := database.Conn().Exec(`
			INSERT INTO transactions (id, account_id, date, amount, status)
			VALUES
				(gen_random_uuid(), ?, CURRENT_DATE, -25.00, 'cleared'),
				(gen_random_uuid(), ?, CURRENT_DATE, -10.00, 'uncleared')
		`, account.ID.String(), account.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert transactions: %v", err)
		}

		account.Name = "New Name"
		if err := svc.Update(account); err != nil {
			t.Fatalf("Update() failed renaming account with transactions: %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Name != "New Name" {
			t.Errorf("Expected name 'New Name', got %q", retrieved.Name)
		}

		var txnCount int
		err = database.Conn().QueryRow(
			`SELECT COUNT(*) FROM transactions WHERE CAST(account_id AS VARCHAR) = ?`,
			account.ID.String(),
		).Scan(&txnCount)
		if err != nil {
			t.Fatalf("Failed to count transactions: %v", err)
		}
		if txnCount != 2 {
			t.Errorf("Expected 2 transactions on renamed account, got %d", txnCount)
		}
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("deletes account without transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("To Delete", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(account.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(account.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})
}

func TestService_List(t *testing.T) {
	t.Run("returns all accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		acct1 := NewAccount("Active 1", TypeChecking, "USD", types.ZeroMoney, types.Today())
		acct2 := NewAccount("Active 2", TypeSavings, "USD", types.ZeroMoney, types.Today())
		for _, a := range []*Account{acct1, acct2} {
			if err := svc.Create(a); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		all, err := svc.List(false)
		if err != nil {
			t.Fatalf("List(false) error = %v", err)
		}
		if len(all) != 2 {
			t.Errorf("Expected 2 accounts, got %d", len(all))
		}
	})

	t.Run("filters active only", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		active := NewAccount("Active", TypeChecking, "USD", types.ZeroMoney, types.Today())
		toClose := NewAccount("To Close", TypeSavings, "USD", types.ZeroMoney, types.Today())
		for _, a := range []*Account{active, toClose} {
			if err := svc.Create(a); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		// Close one account
		if err := svc.Close(toClose.ID, types.Today()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		// All accounts
		all, err := svc.List(false)
		if err != nil {
			t.Fatalf("List(false) error = %v", err)
		}
		if len(all) != 2 {
			t.Errorf("Expected 2 total accounts, got %d", len(all))
		}

		// Active only
		activeList, err := svc.List(true)
		if err != nil {
			t.Fatalf("List(true) error = %v", err)
		}
		if len(activeList) != 1 {
			t.Errorf("Expected 1 active account, got %d", len(activeList))
		}
		if activeList[0].Name != "Active" {
			t.Errorf("Expected 'Active', got %q", activeList[0].Name)
		}
	})

	t.Run("returns empty list for no accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		all, err := svc.List(false)
		if err != nil {
			t.Fatalf("List(false) error = %v", err)
		}
		if len(all) != 0 {
			t.Errorf("Expected 0 accounts, got %d", len(all))
		}
	})
}

func TestService_GetBalance(t *testing.T) {
	t.Run("returns opening balance with no transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Checking", TypeChecking, "USD", types.MustNewMoney("500.00"), types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		balance, err := svc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}

		if balance.AccountID != account.ID {
			t.Errorf("Expected account ID %s, got %s", account.ID.String(), balance.AccountID.String())
		}
		if !balance.CurrentBalance.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected current balance 500.00, got %s", balance.CurrentBalance.String())
		}
		if !balance.ClearedBalance.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected cleared balance 500.00, got %s", balance.ClearedBalance.String())
		}
	})

	t.Run("includes transactions in balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Checking", TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Insert transactions directly
		_, err := database.Conn().Exec(`
			INSERT INTO transactions (id, account_id, date, amount, status)
			VALUES
				(gen_random_uuid(), ?, CURRENT_DATE, -100.00, 'cleared'),
				(gen_random_uuid(), ?, CURRENT_DATE, -50.00, 'uncleared')
		`, account.ID.String(), account.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert transactions: %v", err)
		}

		balance, err := svc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}

		// Current balance = 1000 - 100 - 50 = 850
		if !balance.CurrentBalance.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Expected current balance 850.00, got %s", balance.CurrentBalance.String())
		}

		// Cleared balance = 1000 - 100 = 900
		if !balance.ClearedBalance.Equal(types.MustNewMoney("900.00")) {
			t.Errorf("Expected cleared balance 900.00, got %s", balance.ClearedBalance.String())
		}
	})
}

func TestService_GetAllBalances(t *testing.T) {
	t.Run("returns balances for all accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		accounts := []*Account{
			NewAccount("Acct 1", TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today()),
			NewAccount("Acct 2", TypeSavings, "USD", types.MustNewMoney("200.00"), types.Today()),
			NewAccount("Acct 3", TypeCash, "USD", types.MustNewMoney("300.00"), types.Today()),
		}

		for _, a := range accounts {
			if err := svc.Create(a); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		balances, err := svc.GetAllBalances()
		if err != nil {
			t.Fatalf("GetAllBalances() error = %v", err)
		}

		if len(balances) != 3 {
			t.Errorf("Expected 3 balances, got %d", len(balances))
		}

		for _, a := range accounts {
			b, ok := balances[a.ID]
			if !ok {
				t.Errorf("Missing balance for account %s", a.Name)
				continue
			}
			if !b.CurrentBalance.Equal(a.OpeningBalance) {
				t.Errorf("Account %s: expected balance %s, got %s",
					a.Name, a.OpeningBalance.String(), b.CurrentBalance.String())
			}
		}
	})

	t.Run("returns empty map for no accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		balances, err := svc.GetAllBalances()
		if err != nil {
			t.Fatalf("GetAllBalances() error = %v", err)
		}

		if len(balances) != 0 {
			t.Errorf("Expected 0 balances, got %d", len(balances))
		}
	})
}

func TestService_Close(t *testing.T) {
	t.Run("closes account with zero balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("To Close", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		closeDate := types.Today()
		if err := svc.Close(account.ID, closeDate); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Active {
			t.Error("Account should be inactive after close")
		}
		if !retrieved.ClosedDate.Valid || !retrieved.ClosedDate.Date.Equal(closeDate) {
			t.Errorf("expected close date %s persisted, got valid=%v %s",
				closeDate, retrieved.ClosedDate.Valid, retrieved.ClosedDate.Date)
		}
	})

	t.Run("rejects close date before opening date", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Future Open", TypeChecking, "USD", types.ZeroMoney, types.MustParseDate("2024-01-15"))
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Close(account.ID, types.MustParseDate("2024-01-14"))
		if err == nil {
			t.Fatal("Close() expected error for close date before opening date")
		}
		if _, ok := err.(*InvalidCloseDateError); !ok {
			t.Errorf("Expected InvalidCloseDateError, got %T: %v", err, err)
		}
	})

	t.Run("rejects future close date", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Future Close", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Close(account.ID, types.Today().AddDays(1))
		if err == nil {
			t.Fatal("Close() expected error for future close date")
		}
		if _, ok := err.(*InvalidCloseDateError); !ok {
			t.Errorf("Expected InvalidCloseDateError, got %T: %v", err, err)
		}
	})

	t.Run("rejects closing account with non-zero balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Has Balance", TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Close(account.ID, types.Today())
		if err == nil {
			t.Error("Close() expected error for account with balance")
		}
		if _, ok := err.(*HasBalanceError); !ok {
			t.Errorf("Expected HasBalanceError, got %T: %v", err, err)
		}

		// Verify still active
		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Active {
			t.Error("Account should still be active after failed close")
		}
	})

	t.Run("rejects closing already closed account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Already Closed", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Close(account.ID, types.Today()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		err := svc.Close(account.ID, types.Today())
		if err == nil {
			t.Error("Close() expected error for already closed account")
		}
		if _, ok := err.(*AlreadyClosedError); !ok {
			t.Errorf("Expected AlreadyClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects closing non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.Close(types.NewID(), types.Today())
		if err == nil {
			t.Error("Close() expected error for non-existent account")
		}
	})
}

func TestService_Reopen(t *testing.T) {
	t.Run("reopens closed account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("To Reopen", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Close(account.ID, types.Today()); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		if err := svc.Reopen(account.ID); err != nil {
			t.Fatalf("Reopen() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Active {
			t.Error("Account should be active after reopen")
		}
		if retrieved.ClosedDate.Valid {
			t.Error("Reopen should clear the persisted close date")
		}
	})

	t.Run("rejects reopening active account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Already Active", TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Reopen(account.ID)
		if err == nil {
			t.Error("Reopen() expected error for active account")
		}
		if _, ok := err.(*NotClosedError); !ok {
			t.Errorf("Expected NotClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects reopening non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		err := svc.Reopen(types.NewID())
		if err == nil {
			t.Error("Reopen() expected error for non-existent account")
		}
	})
}

func TestService_validateAccount(t *testing.T) {
	t.Run("returns nil for valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("Valid", TypeChecking, "USD", types.ZeroMoney, types.Today())
		err := svc.validateAccount(account)
		if err != nil {
			t.Errorf("validateAccount() expected nil, got %v", err)
		}
	})

	t.Run("returns ServiceValidationError for invalid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := NewRepository(database)
		svc := NewService(repo, database)

		account := NewAccount("", TypeChecking, "USD", types.ZeroMoney, types.Today())
		err := svc.validateAccount(account)
		if err == nil {
			t.Error("validateAccount() expected error for empty name")
		}
		svcErr, ok := err.(*types.ServiceValidationError)
		if !ok {
			t.Fatalf("Expected ServiceValidationError, got %T", err)
		}
		if !svcErr.Errors.HasErrors() {
			t.Error("ServiceValidationError should contain validation errors")
		}
	})
}
