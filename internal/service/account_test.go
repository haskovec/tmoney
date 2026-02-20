package service

import (
	"testing"

	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

func TestNewAccountService(t *testing.T) {
	t.Run("creates service with repository and db", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		if svc == nil {
			t.Fatal("NewAccountService should not return nil")
		}
		if svc.repo != repo {
			t.Error("NewAccountService should store repository")
		}
		if svc.db != database {
			t.Error("NewAccountService should store database")
		}
	})
}

func TestAccountService_Create(t *testing.T) {
	t.Run("creates valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount(
			"Test Checking",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("1000.00"),
			models.NewDate(2024, 1, 15),
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
		if retrieved.Type != models.AccountTypeChecking {
			t.Errorf("Expected type checking, got %q", retrieved.Type)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", retrieved.Currency)
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for empty name")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects invalid currency", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Test", models.AccountTypeChecking, "INVALID", models.ZeroMoney, models.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for invalid currency")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("rejects invalid account type", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Test", models.AccountType("invalid"), "USD", models.ZeroMoney, models.Today())
		err := svc.Create(account)
		if err == nil {
			t.Error("Create() expected error for invalid account type")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("creates account with optional fields", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Full Account", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
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

func TestAccountService_GetByName(t *testing.T) {
	t.Run("retrieves account by name", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("My Savings", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today())
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		_, err := svc.GetByName("Does Not Exist")
		if err == nil {
			t.Error("GetByName() expected error for non-existent name")
		}
	})
}

func TestAccountService_Update(t *testing.T) {
	t.Run("updates valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Original", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Valid", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		account.Name = "" // Invalid
		err := svc.Update(account)
		if err == nil {
			t.Error("Update() expected error for empty name")
		}
		if _, ok := err.(*ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})
}

func TestAccountService_Delete(t *testing.T) {
	t.Run("deletes account without transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("To Delete", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
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

func TestAccountService_List(t *testing.T) {
	t.Run("returns all accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		acct1 := models.NewAccount("Active 1", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		acct2 := models.NewAccount("Active 2", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today())
		for _, a := range []*models.Account{acct1, acct2} {
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		active := models.NewAccount("Active", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		toClose := models.NewAccount("To Close", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today())
		for _, a := range []*models.Account{active, toClose} {
			if err := svc.Create(a); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
		}

		// Close one account
		if err := svc.Close(toClose.ID); err != nil {
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		all, err := svc.List(false)
		if err != nil {
			t.Fatalf("List(false) error = %v", err)
		}
		if len(all) != 0 {
			t.Errorf("Expected 0 accounts, got %d", len(all))
		}
	})
}

func TestAccountService_GetBalance(t *testing.T) {
	t.Run("returns opening balance with no transactions", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("500.00"), models.Today())
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
		if !balance.CurrentBalance.Equal(models.MustNewMoney("500.00")) {
			t.Errorf("Expected current balance 500.00, got %s", balance.CurrentBalance.String())
		}
		if !balance.ClearedBalance.Equal(models.MustNewMoney("500.00")) {
			t.Errorf("Expected cleared balance 500.00, got %s", balance.ClearedBalance.String())
		}
	})

	t.Run("includes transactions in balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000.00"), models.Today())
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
		if !balance.CurrentBalance.Equal(models.MustNewMoney("850.00")) {
			t.Errorf("Expected current balance 850.00, got %s", balance.CurrentBalance.String())
		}

		// Cleared balance = 1000 - 100 = 900
		if !balance.ClearedBalance.Equal(models.MustNewMoney("900.00")) {
			t.Errorf("Expected cleared balance 900.00, got %s", balance.ClearedBalance.String())
		}
	})
}

func TestAccountService_GetAllBalances(t *testing.T) {
	t.Run("returns balances for all accounts", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		accounts := []*models.Account{
			models.NewAccount("Acct 1", models.AccountTypeChecking, "USD", models.MustNewMoney("100.00"), models.Today()),
			models.NewAccount("Acct 2", models.AccountTypeSavings, "USD", models.MustNewMoney("200.00"), models.Today()),
			models.NewAccount("Acct 3", models.AccountTypeCash, "USD", models.MustNewMoney("300.00"), models.Today()),
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		balances, err := svc.GetAllBalances()
		if err != nil {
			t.Fatalf("GetAllBalances() error = %v", err)
		}

		if len(balances) != 0 {
			t.Errorf("Expected 0 balances, got %d", len(balances))
		}
	})
}

func TestAccountService_Close(t *testing.T) {
	t.Run("closes account with zero balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("To Close", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Close(account.ID); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.Active {
			t.Error("Account should be inactive after close")
		}
	})

	t.Run("rejects closing account with non-zero balance", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Has Balance", models.AccountTypeChecking, "USD", models.MustNewMoney("100.00"), models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Close(account.ID)
		if err == nil {
			t.Error("Close() expected error for account with balance")
		}
		if _, ok := err.(*AccountHasBalanceError); !ok {
			t.Errorf("Expected AccountHasBalanceError, got %T: %v", err, err)
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
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Already Closed", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Close(account.ID); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		err := svc.Close(account.ID)
		if err == nil {
			t.Error("Close() expected error for already closed account")
		}
		if _, ok := err.(*AccountAlreadyClosedError); !ok {
			t.Errorf("Expected AccountAlreadyClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects closing non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		err := svc.Close(models.NewID())
		if err == nil {
			t.Error("Close() expected error for non-existent account")
		}
	})
}

func TestAccountService_Reopen(t *testing.T) {
	t.Run("reopens closed account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("To Reopen", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Close(account.ID); err != nil {
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
	})

	t.Run("rejects reopening active account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Already Active", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		if err := svc.Create(account); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.Reopen(account.ID)
		if err == nil {
			t.Error("Reopen() expected error for active account")
		}
		if _, ok := err.(*AccountNotClosedError); !ok {
			t.Errorf("Expected AccountNotClosedError, got %T: %v", err, err)
		}
	})

	t.Run("rejects reopening non-existent account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		err := svc.Reopen(models.NewID())
		if err == nil {
			t.Error("Reopen() expected error for non-existent account")
		}
	})
}

func TestAccountService_validateAccount(t *testing.T) {
	t.Run("returns nil for valid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("Valid", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		err := svc.validateAccount(account)
		if err != nil {
			t.Errorf("validateAccount() expected nil, got %v", err)
		}
	})

	t.Run("returns ServiceValidationError for invalid account", func(t *testing.T) {
		database := createTestDB(t)
		repo := repository.NewAccountRepository(database)
		svc := NewAccountService(repo, database)

		account := models.NewAccount("", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
		err := svc.validateAccount(account)
		if err == nil {
			t.Error("validateAccount() expected error for empty name")
		}
		svcErr, ok := err.(*ServiceValidationError)
		if !ok {
			t.Fatalf("Expected ServiceValidationError, got %T", err)
		}
		if !svcErr.Errors.HasErrors() {
			t.Error("ServiceValidationError should contain validation errors")
		}
	})
}
