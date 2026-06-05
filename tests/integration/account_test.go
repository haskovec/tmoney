package integration

import (
	"os"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// TestAccountLifecycle tests the complete account lifecycle:
// create database -> run migrations -> create account -> list accounts -> update -> delete -> cleanup
func TestAccountLifecycle(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	defer database.Close()

	// Verify schema version is correct
	version, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}
	if version != db.CurrentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", db.CurrentSchemaVersion, version)
	}

	repo := account.NewRepository(database)

	// Step 2: Create a test account
	t.Run("Create account", func(t *testing.T) {
		account := account.NewAccount(
			"Chase Checking",
			account.TypeChecking,
			"USD",
			types.MustNewMoney("1000.00"),
			types.NewDate(2024, 1, 15),
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
		if retrieved.Type != account.TypeChecking {
			t.Errorf("Expected type 'checking', got %q", retrieved.Type)
		}
		if retrieved.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", retrieved.Currency)
		}
		if !retrieved.OpeningBalance.Equal(types.MustNewMoney("1000.00")) {
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
	database := dbtest.New(t)

	repo := account.NewRepository(database)

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
		account := account.NewAccount(
			tc.name,
			account.TypeInvestment,
			"USD",
			types.ZeroMoney,
			types.Today(),
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
	database := dbtest.New(t)

	repo := account.NewRepository(database)

	// Create multiple accounts
	accounts := []*account.Account{
		account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today()),
		account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("5000"), types.Today()),
		account.NewAccount("Credit Card", account.TypeCreditCard, "USD", types.MustNewMoney("0"), types.Today()),
	}

	// Set credit limit on credit card
	accounts[2].SetCreditLimit(types.MustNewMoney("10000"))

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
	var creditCard *account.Account
	for _, acc := range listed {
		if acc.Type == account.TypeCreditCard {
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
	if !creditCard.CreditLimit.Money.Equal(types.MustNewMoney("10000")) {
		t.Errorf("Expected credit limit 10000, got %s", creditCard.CreditLimit.Money.String())
	}
}

// TestAccountWithTransactionsCannotBeDeleted tests that an account with transactions cannot be deleted.
func TestAccountActiveFilter(t *testing.T) {
	database := dbtest.New(t)

	repo := account.NewRepository(database)

	// Create an active and closed account
	activeAcc := account.NewAccount("Active Account", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	closedAcc := account.NewAccount("Closed Account", account.TypeSavings, "USD", types.ZeroMoney, types.Today())
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
	database := dbtest.New(t)

	repo := account.NewRepository(database)

	// Try to get non-existent account by ID
	_, err := repo.GetByID(types.NewID())
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to get non-existent account by name
	_, err = repo.GetByName("Does Not Exist")
	if err == nil {
		t.Error("Expected error for non-existent account")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to delete non-existent account
	err = repo.Delete(types.NewID())
	if err == nil {
		t.Error("Expected error for deleting non-existent account")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

// TestAccountTrackLots tests that the track_lots field is persisted and retrieved correctly.
// SM-037: existing accounts get track_lots=false; new investment account can set track_lots=true.
// SM-038: Create, read, and update round-trip for track_lots field.
func TestAccountTrackLots(t *testing.T) {
	database := dbtest.New(t)

	// Verify schema version includes the track_lots migration
	version, err := database.SchemaVersion()
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}
	if version < 7 {
		t.Fatalf("Expected schema version >= 7, got %d", version)
	}

	repo := account.NewRepository(database)

	// Test 1: Non-investment account defaults to track_lots=false
	t.Run("non-investment account defaults to track_lots false", func(t *testing.T) {
		checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
		if err := repo.Create(checking); err != nil {
			t.Fatalf("Failed to create checking account: %v", err)
		}

		retrieved, err := repo.GetByID(checking.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve checking account: %v", err)
		}
		if retrieved.TrackLots {
			t.Error("Expected track_lots=false for checking account")
		}
	})

	// Test 2: Investment account with track_lots=false (default)
	t.Run("investment account defaults to track_lots false", func(t *testing.T) {
		investment := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
		if err := repo.Create(investment); err != nil {
			t.Fatalf("Failed to create investment account: %v", err)
		}

		retrieved, err := repo.GetByID(investment.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve investment account: %v", err)
		}
		if retrieved.TrackLots {
			t.Error("Expected track_lots=false for default investment account")
		}
	})

	// Test 3: Investment account with track_lots=true
	t.Run("investment account with track_lots true", func(t *testing.T) {
		lotTracked := account.NewAccount("IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
		lotTracked.TrackLots = true
		if err := repo.Create(lotTracked); err != nil {
			t.Fatalf("Failed to create lot-tracked investment account: %v", err)
		}

		// Verify via GetByID
		retrieved, err := repo.GetByID(lotTracked.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve lot-tracked account: %v", err)
		}
		if !retrieved.TrackLots {
			t.Error("Expected track_lots=true for lot-tracked investment account")
		}

		// Verify via GetByName
		byName, err := repo.GetByName("IRA")
		if err != nil {
			t.Fatalf("Failed to retrieve by name: %v", err)
		}
		if !byName.TrackLots {
			t.Error("Expected track_lots=true when retrieved by name")
		}
	})

	// Test 4: List returns track_lots correctly
	t.Run("list returns track_lots correctly", func(t *testing.T) {
		accounts, err := repo.List(false)
		if err != nil {
			t.Fatalf("Failed to list accounts: %v", err)
		}

		lotTrackedCount := 0
		for _, acc := range accounts {
			if acc.TrackLots {
				lotTrackedCount++
				if acc.Type != account.TypeInvestment {
					t.Errorf("Non-investment account %q has track_lots=true", acc.Name)
				}
			}
		}
		if lotTrackedCount != 1 {
			t.Errorf("Expected 1 lot-tracked account, got %d", lotTrackedCount)
		}
	})

	// Test 5: Update track_lots from false to true
	t.Run("update track_lots from false to true", func(t *testing.T) {
		account, err := repo.GetByName("Brokerage")
		if err != nil {
			t.Fatalf("Failed to get account: %v", err)
		}
		if account.TrackLots {
			t.Fatal("Precondition failed: expected track_lots=false")
		}

		account.TrackLots = true
		if err := repo.Update(account); err != nil {
			t.Fatalf("Failed to update account: %v", err)
		}

		updated, err := repo.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated account: %v", err)
		}
		if !updated.TrackLots {
			t.Error("Expected track_lots=true after update")
		}
	})

	// Test 6: Update track_lots from true to false
	t.Run("update track_lots from true to false", func(t *testing.T) {
		account, err := repo.GetByName("Brokerage")
		if err != nil {
			t.Fatalf("Failed to get account: %v", err)
		}
		if !account.TrackLots {
			t.Fatal("Precondition failed: expected track_lots=true")
		}

		account.TrackLots = false
		if err := repo.Update(account); err != nil {
			t.Fatalf("Failed to update account: %v", err)
		}

		updated, err := repo.GetByID(account.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve updated account: %v", err)
		}
		if updated.TrackLots {
			t.Error("Expected track_lots=false after update")
		}
	})
}

// TestAccountTrackLotsColumnExists verifies the track_lots column exists in the accounts table schema.
func TestAccountTrackLotsColumnExists(t *testing.T) {
	database := dbtest.New(t)

	var colName string
	err := database.Conn().QueryRow(
		`SELECT column_name FROM information_schema.columns WHERE table_name = 'accounts' AND column_name = 'track_lots'`,
	).Scan(&colName)
	if err != nil {
		t.Fatalf("track_lots column not found in accounts table: %v", err)
	}
	if colName != "track_lots" {
		t.Errorf("Expected column name 'track_lots', got %q", colName)
	}
}

func TestCheckSchema(t *testing.T) {
	database := dbtest.New(t)

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
