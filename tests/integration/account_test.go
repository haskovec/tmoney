package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/models"
	"github.com/haskovec/tmoney/internal/repository"
)

// TestAccountLifecycle tests the complete account lifecycle:
// create database -> run migrations -> create account -> list accounts -> update -> delete -> cleanup
func TestAccountLifecycle(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "tmoney-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Cleanup

	dbPath := filepath.Join(tempDir, "test.tdb")

	// Step 1: Create a new database (this also runs migrations)
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	// Verify schema version is correct
	version, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}
	if version != db.CurrentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", db.CurrentSchemaVersion, version)
	}

	repo := repository.NewAccountRepository(database)

	// Step 2: Create a test account
	t.Run("Create account", func(t *testing.T) {
		account := models.NewAccount(
			"Chase Checking",
			models.AccountTypeChecking,
			"USD",
			models.MustNewMoney("1000.00"),
			models.NewDate(2024, 1, 15),
		)
		account.SetInstitution("Chase Bank")
		account.SetAccountNumber("1234")

		err = repo.Create(account)
		if err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}
	})

	// Step 3: List all accounts
	t.Run("List accounts", func(t *testing.T) {
		accounts, err := repo.List(false)
		if err != nil {
			t.Fatalf("Failed to list accounts: %v", err)
		}

		if len(accounts) != 1 {
			t.Fatalf("Expected 1 account, got %d", len(accounts))
		}

		// Verify account data
		retrieved := accounts[0]
		if retrieved.Name != "Chase Checking" {
			t.Errorf("Expected name 'Chase Checking', got %q", retrieved.Name)
		}
		if retrieved.Type != models.AccountTypeChecking {
			t.Errorf("Expected type 'checking', got %q", retrieved.Type)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", retrieved.Currency)
		}
		if !retrieved.OpeningBalance.Equal(models.MustNewMoney("1000.00")) {
			t.Errorf("Expected opening balance '1000.00', got %q", retrieved.OpeningBalance.String())
		}
		if !retrieved.Active {
			t.Error("Expected account to be active")
		}
		if !retrieved.Institution.Valid || retrieved.Institution.String != "Chase Bank" {
			t.Errorf("Expected institution 'Chase Bank', got %v", retrieved.Institution)
		}
		if !retrieved.AccountNumber.Valid || retrieved.AccountNumber.String != "1234" {
			t.Errorf("Expected account number '1234', got %v", retrieved.AccountNumber)
		}
	})

	// Step 4: Retrieve by ID and by name
	t.Run("Get account by ID and name", func(t *testing.T) {
		// Get by name
		account, err := repo.GetByName("Chase Checking")
		if err != nil {
			t.Fatalf("Failed to get account by name: %v", err)
		}
		if account.Name != "Chase Checking" {
			t.Errorf("Expected name 'Chase Checking', got %q", account.Name)
		}

		// Get by ID
		accountByID, err := repo.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to get account by ID: %v", err)
		}
		if accountByID.Name != account.Name {
			t.Errorf("Expected same account, got different names: %q vs %q", account.Name, accountByID.Name)
		}
	})

	// Step 5: Update the account
	t.Run("Update account", func(t *testing.T) {
		account, err := repo.GetByName("Chase Checking")
		if err != nil {
			t.Fatalf("Failed to get account: %v", err)
		}

		// Update the account
		account.Name = "Chase Primary Checking"
		account.SetNotes("Main checking account")

		err = repo.Update(account)
		if err != nil {
			t.Fatalf("Failed to update account: %v", err)
		}

		// Verify the update
		updated, err := repo.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to get updated account: %v", err)
		}
		if updated.Name != "Chase Primary Checking" {
			t.Errorf("Expected name 'Chase Primary Checking', got %q", updated.Name)
		}
		if !updated.Notes.Valid || updated.Notes.String != "Main checking account" {
			t.Errorf("Expected notes 'Main checking account', got %v", updated.Notes)
		}
	})

	// Step 6: Delete the account
	t.Run("Delete account", func(t *testing.T) {
		account, err := repo.GetByName("Chase Primary Checking")
		if err != nil {
			t.Fatalf("Failed to get account: %v", err)
		}

		err = repo.Delete(account.ID)
		if err != nil {
			t.Fatalf("Failed to delete account: %v", err)
		}

		// Verify deletion
		accounts, err := repo.List(false)
		if err != nil {
			t.Fatalf("Failed to list accounts: %v", err)
		}
		if len(accounts) != 0 {
			t.Errorf("Expected 0 accounts after deletion, got %d", len(accounts))
		}
	})

	// Verify temp directory still has the database file (cleaned up by defer)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file should exist before cleanup")
	}
}

// TestAlphanumericAccountNumber tests that account numbers with letters are stored and retrieved correctly.
func TestAlphanumericAccountNumber(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewAccountRepository(database)

	tests := []struct {
		name          string
		accountNumber string
	}{
		{"Brokerage", "Z12-345ABC"},
		{"IRA", "9X8Y7Z"},
		{"Trading", "ACCT-2024-001"},
		{"Foreign", "GB29NWBK60161331926819"},
	}

	for _, tc := range tests {
		account := models.NewAccount(
			tc.name,
			models.AccountTypeInvestment,
			"USD",
			models.ZeroMoney,
			models.Today(),
		)
		account.SetAccountNumber(tc.accountNumber)

		if err := repo.Create(account); err != nil {
			t.Fatalf("Failed to create account %q: %v", tc.name, err)
		}

		retrieved, err := repo.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account %q: %v", tc.name, err)
		}

		if !retrieved.AccountNumber.Valid {
			t.Errorf("Account %q: account number should be valid", tc.name)
		}
		if retrieved.AccountNumber.String != tc.accountNumber {
			t.Errorf("Account %q: expected account number %q, got %q",
				tc.name, tc.accountNumber, retrieved.AccountNumber.String)
		}
	}
}

// TestMultipleAccounts tests creating and listing multiple accounts.
func TestMultipleAccounts(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	// Create database
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewAccountRepository(database)

	// Create multiple accounts
	accounts := []*models.Account{
		models.NewAccount("Checking", models.AccountTypeChecking, "USD", models.MustNewMoney("1000"), models.Today()),
		models.NewAccount("Savings", models.AccountTypeSavings, "USD", models.MustNewMoney("5000"), models.Today()),
		models.NewAccount("Credit Card", models.AccountTypeCreditCard, "USD", models.MustNewMoney("0"), models.Today()),
	}

	// Set credit limit on credit card
	accounts[2].SetCreditLimit(models.MustNewMoney("10000"))

	for _, acc := range accounts {
		if err := repo.Create(acc); err != nil {
			t.Fatalf("Failed to create account %q: %v", acc.Name, err)
		}
	}

	// List all accounts
	listed, err := repo.List(false)
	if err != nil {
		t.Fatalf("Failed to list accounts: %v", err)
	}

	if len(listed) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(listed))
	}

	// Verify credit limit on credit card
	var creditCard *models.Account
	for _, acc := range listed {
		if acc.Type == models.AccountTypeCreditCard {
			creditCard = acc
			break
		}
	}
	if creditCard == nil {
		t.Fatal("Credit card account not found")
	}
	if !creditCard.CreditLimit.Valid {
		t.Error("Credit limit should be set")
	}
	if !creditCard.CreditLimit.Money.Equal(models.MustNewMoney("10000")) {
		t.Errorf("Expected credit limit 10000, got %s", creditCard.CreditLimit.Money.String())
	}
}

// TestAccountWithTransactionsCannotBeDeleted tests that an account with transactions cannot be deleted.
func TestAccountActiveFilter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewAccountRepository(database)

	// Create an active and closed account
	activeAcc := models.NewAccount("Active Account", models.AccountTypeChecking, "USD", models.ZeroMoney, models.Today())
	closedAcc := models.NewAccount("Closed Account", models.AccountTypeSavings, "USD", models.ZeroMoney, models.Today())
	closedAcc.Close()

	if err := repo.Create(activeAcc); err != nil {
		t.Fatalf("Failed to create active account: %v", err)
	}
	if err := repo.Create(closedAcc); err != nil {
		t.Fatalf("Failed to create closed account: %v", err)
	}

	// List all accounts
	allAccounts, err := repo.List(false)
	if err != nil {
		t.Fatalf("Failed to list all accounts: %v", err)
	}
	if len(allAccounts) != 2 {
		t.Errorf("Expected 2 accounts, got %d", len(allAccounts))
	}

	// List active accounts only
	activeAccounts, err := repo.List(true)
	if err != nil {
		t.Fatalf("Failed to list active accounts: %v", err)
	}
	if len(activeAccounts) != 1 {
		t.Errorf("Expected 1 active account, got %d", len(activeAccounts))
	}
	if activeAccounts[0].Name != "Active Account" {
		t.Errorf("Expected 'Active Account', got %q", activeAccounts[0].Name)
	}
}

// TestAccountNotFound tests error handling for non-existent accounts.
func TestAccountNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	repo := repository.NewAccountRepository(database)

	// Try to get non-existent account by ID
	_, err = repo.GetByID(models.NewID())
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to get non-existent account by name
	_, err = repo.GetByName("Does Not Exist")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to delete non-existent account
	err = repo.Delete(models.NewID())
	if err == nil {
		t.Error("Expected error for deleting non-existent account")
	}
	if _, ok := err.(*repository.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

func TestCheckSchema(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tmoney-schema-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer database.Close()

	// Check if UNIQUE constraint exists
	rows, err := database.Conn().Query(`SELECT constraint_type FROM information_schema.table_constraints WHERE table_name = 'accounts'`)
	if err != nil {
		t.Fatalf("Failed to query constraints: %v", err)
	}
	defer rows.Close()

	t.Log("Constraints on accounts table:")
	for rows.Next() {
		var constraintType string
		rows.Scan(&constraintType)
		t.Logf("  %s", constraintType)
	}
}
