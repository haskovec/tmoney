package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
	"github.com/haskovec/tmoney/internal/service"
)

// createTestService creates a test database and service for testing.
func createTestService(t *testing.T) (*service.AccountService, *db.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "tmoney-service-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create database: %v", err)
	}

	repo := repository.NewAccountRepository(database)
	svc := service.NewAccountService(repo, database)

	cleanup := func() {
		database.Close()
		os.RemoveAll(tempDir)
	}

	return svc, database, cleanup
}

func TestAccountServiceCreate(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("creates valid account", func(t *testing.T) {
		account := models.NewAccount(
			"Test Checking",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("1000.00"),
			models.NewDate(2024, 1, 15),
		)

		err := svc.Create(account)
		if err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Verify account was created
		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if retrieved.Name != "Test Checking" {
			t.Errorf("Expected name 'Test Checking', got %q", retrieved.Name)
		}
	})

	t.Run("rejects invalid account", func(t *testing.T) {
		account := models.NewAccount(
			"", // Invalid: empty name
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("1000.00"),
			models.NewDate(2024, 1, 15),
		)

		err := svc.Create(account)
		if err == nil {
			t.Error("Expected validation error for empty name")
		}
		if _, ok := err.(*service.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		account1 := models.NewAccount(
			"Duplicate Account",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		err := svc.Create(account1)
		if err != nil {
			t.Fatalf("Failed to create first account: %v", err)
		}

		account2 := models.NewAccount(
			"Duplicate Account",
			models.AccountTypeSavings,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		err = svc.Create(account2)
		if err == nil {
			t.Error("Expected error for duplicate account name")
		}
		if _, ok := err.(*repository.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceUpdate(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("updates valid account", func(t *testing.T) {
		account := models.NewAccount(
			"Original Name",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		account.Name = "Updated Name"
		account.SetNotes("Some notes")
		if err := svc.Update(account); err != nil {
			t.Fatalf("Failed to update account: %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if retrieved.Name != "Updated Name" {
			t.Errorf("Expected name 'Updated Name', got %q", retrieved.Name)
		}
		if !retrieved.Notes.Valid || retrieved.Notes.String != "Some notes" {
			t.Errorf("Expected notes 'Some notes', got %v", retrieved.Notes)
		}
	})

	t.Run("rejects invalid update", func(t *testing.T) {
		account := models.NewAccount(
			"Valid Account",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		account.Name = "" // Invalid
		err := svc.Update(account)
		if err == nil {
			t.Error("Expected validation error for empty name")
		}
		if _, ok := err.(*service.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceGetBalance(t *testing.T) {
	svc, database, cleanup := createTestService(t)
	defer cleanup()

	t.Run("returns opening balance when no transactions", func(t *testing.T) {
		account := models.NewAccount(
			"Checking",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("500.00"),
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		balance, err := svc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		if !balance.CurrentBalance.Equal(models.MustNewMoney("500.00")) {
			t.Errorf("Expected current balance 500.00, got %s", balance.CurrentBalance.String())
		}
		if !balance.ClearedBalance.Equal(models.MustNewMoney("500.00")) {
			t.Errorf("Expected cleared balance 500.00, got %s", balance.ClearedBalance.String())
		}
	})

	t.Run("includes transactions in balance", func(t *testing.T) {
		account := models.NewAccount(
			"Checking With Transactions",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("1000.00"),
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Insert transactions directly for testing
		_, err := database.Conn().Exec(`
			INSERT INTO transactions (id, account_id, date, amount, status)
			VALUES
				(gen_random_uuid(), ?, CURRENT_DATE, -100.00, 'cleared'),
				(gen_random_uuid(), ?, CURRENT_DATE, -50.00, 'pending')
		`, account.ID.String(), account.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert transactions: %v", err)
		}

		balance, err := svc.GetBalance(account.ID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		// Current balance = 1000 - 100 - 50 = 850
		if !balance.CurrentBalance.Equal(models.MustNewMoney("850.00")) {
			t.Errorf("Expected current balance 850.00, got %s", balance.CurrentBalance.String())
		}

		// Cleared balance = 1000 - 100 = 900 (pending transaction excluded)
		if !balance.ClearedBalance.Equal(models.MustNewMoney("900.00")) {
			t.Errorf("Expected cleared balance 900.00, got %s", balance.ClearedBalance.String())
		}
	})
}

func TestAccountServiceGetAllBalances(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("returns balances for all accounts", func(t *testing.T) {
		accounts := []*models.Account{
			models.NewAccount("Account 1", models.AccountTypeChecking, "USD", models.MustNewMoney("100"), models.Today()),
			models.NewAccount("Account 2", models.AccountTypeSavings, "USD", models.MustNewMoney("200"), models.Today()),
			models.NewAccount("Account 3", models.AccountTypeCash, "USD", models.MustNewMoney("300"), models.Today()),
		}

		for _, acc := range accounts {
			if err := svc.Create(acc); err != nil {
				t.Fatalf("Failed to create account: %v", err)
			}
		}

		balances, err := svc.GetAllBalances()
		if err != nil {
			t.Fatalf("Failed to get all balances: %v", err)
		}

		if len(balances) != 3 {
			t.Errorf("Expected 3 balances, got %d", len(balances))
		}

		// Verify each account has correct balance
		for _, acc := range accounts {
			balance, ok := balances[acc.ID]
			if !ok {
				t.Errorf("Missing balance for account %s", acc.Name)
				continue
			}
			if !balance.CurrentBalance.Equal(acc.OpeningBalance) {
				t.Errorf("Account %s: expected balance %s, got %s",
					acc.Name, acc.OpeningBalance.String(), balance.CurrentBalance.String())
			}
		}
	})
}

func TestAccountServiceClose(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("closes account with zero balance", func(t *testing.T) {
		account := models.NewAccount(
			"Account To Close",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(account.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if retrieved.Active {
			t.Error("Expected account to be inactive")
		}
	})

	t.Run("rejects closing account with non-zero balance", func(t *testing.T) {
		account := models.NewAccount(
			"Account With Balance",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("100.00"),
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		err := svc.Close(account.ID)
		if err == nil {
			t.Error("Expected error when closing account with balance")
		}
		if _, ok := err.(*service.AccountHasBalanceError); !ok {
			t.Errorf("Expected AccountHasBalanceError, got %T: %v", err, err)
		}

		// Verify account is still active
		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if !retrieved.Active {
			t.Error("Account should still be active after failed close")
		}
	})

	t.Run("rejects closing already closed account", func(t *testing.T) {
		account := models.NewAccount(
			"Already Closed",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(account.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		err := svc.Close(account.ID)
		if err == nil {
			t.Error("Expected error when closing already closed account")
		}
		if _, ok := err.(*service.AccountAlreadyClosedError); !ok {
			t.Errorf("Expected AccountAlreadyClosedError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceReopen(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("reopens closed account", func(t *testing.T) {
		account := models.NewAccount(
			"Account To Reopen",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(account.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		if err := svc.Reopen(account.ID); err != nil {
			t.Fatalf("Failed to reopen account: %v", err)
		}

		retrieved, err := svc.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if !retrieved.Active {
			t.Error("Expected account to be active after reopen")
		}
	})

	t.Run("rejects reopening active account", func(t *testing.T) {
		account := models.NewAccount(
			"Active Account",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		err := svc.Reopen(account.ID)
		if err == nil {
			t.Error("Expected error when reopening active account")
		}
		if _, ok := err.(*service.AccountNotClosedError); !ok {
			t.Errorf("Expected AccountNotClosedError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceList(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("lists all accounts", func(t *testing.T) {
		accounts := []*models.Account{
			models.NewAccount("Active 1", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today()),
			models.NewAccount("Active 2", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today()),
			models.NewAccount("Closed", models.AccountTypeCash, "USD", models.ZeroMoney, models.Today()),
		}

		for _, acc := range accounts {
			if err := svc.Create(acc); err != nil {
				t.Fatalf("Failed to create account: %v", err)
			}
		}

		// Close one account
		if err := svc.Close(accounts[2].ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		// List all
		all, err := svc.List(false)
		if err != nil {
			t.Fatalf("Failed to list accounts: %v", err)
		}
		if len(all) != 3 {
			t.Errorf("Expected 3 accounts, got %d", len(all))
		}

		// List active only
		active, err := svc.List(true)
		if err != nil {
			t.Fatalf("Failed to list active accounts: %v", err)
		}
		if len(active) != 2 {
			t.Errorf("Expected 2 active accounts, got %d", len(active))
		}
	})
}

func TestAccountServiceDelete(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("deletes account without transactions", func(t *testing.T) {
		account := models.NewAccount(
			"Account To Delete",
			models.AccountTypeChecking,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		if err := svc.Create(account); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Delete(account.ID); err != nil {
			t.Fatalf("Failed to delete account: %v", err)
		}

		_, err := svc.GetByID(account.ID)
		if err == nil {
			t.Error("Expected error when getting deleted account")
		}
	})
}
