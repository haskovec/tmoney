package integration

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// createTestService creates a test database and service for testing.
func createTestService(t *testing.T) (*account.Service, *db.DB, func()) {
	t.Helper()

	database := dbtest.New(t)

	repo := account.NewRepository(database)
	svc := account.NewService(repo, database)

	cleanup := func() {}

	return svc, database, cleanup
}

func TestAccountServiceCreate(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("creates valid account", func(t *testing.T) {
		acct := account.NewAccount(
			"Test Checking",
			account.TypeChecking,
			"USD",
			types.MustNewMoney("1000.00"),
			types.NewDate(2024, 1, 15),
		)

		err := svc.Create(acct)
		if err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Verify account was created
		retrieved, err := svc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if retrieved.Name != "Test Checking" {
			t.Errorf("Expected name 'Test Checking', got %q", retrieved.Name)
		}
	})

	t.Run("rejects invalid account", func(t *testing.T) {
		acct := account.NewAccount(
			"", // Invalid: empty name
			account.TypeChecking,
			"USD",
			types.MustNewMoney("1000.00"),
			types.NewDate(2024, 1, 15),
		)

		err := svc.Create(acct)
		if err == nil {
			t.Error("Expected validation error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})

	t.Run("rejects duplicate name", func(t *testing.T) {
		acct1 := account.NewAccount(
			"Duplicate Account",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		err := svc.Create(acct1)
		if err != nil {
			t.Fatalf("Failed to create first account: %v", err)
		}

		acct2 := account.NewAccount(
			"Duplicate Account",
			account.TypeSavings,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		err = svc.Create(acct2)
		if err == nil {
			t.Error("Expected error for duplicate account name")
		}
		if _, ok := err.(*dberrors.DuplicateError); !ok {
			t.Errorf("Expected DuplicateError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceUpdate(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("updates valid account", func(t *testing.T) {
		acct := account.NewAccount(
			"Original Name",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		acct.Name = "Updated Name"
		acct.SetNotes("Some notes")
		if err := svc.Update(acct); err != nil {
			t.Fatalf("Failed to update account: %v", err)
		}

		retrieved, err := svc.GetByID(acct.ID)
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
		acct := account.NewAccount(
			"Valid Account",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		acct.Name = "" // Invalid
		err := svc.Update(acct)
		if err == nil {
			t.Error("Expected validation error for empty name")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceGetBalance(t *testing.T) {
	svc, database, cleanup := createTestService(t)
	defer cleanup()

	t.Run("returns opening balance when no transactions", func(t *testing.T) {
		acct := account.NewAccount(
			"Checking",
			account.TypeChecking,
			"USD",
			types.MustNewMoney("500.00"),
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		balance, err := svc.GetBalance(acct.ID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		if !balance.CurrentBalance.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected current balance 500.00, got %s", balance.CurrentBalance.String())
		}
		if !balance.ClearedBalance.Equal(types.MustNewMoney("500.00")) {
			t.Errorf("Expected cleared balance 500.00, got %s", balance.ClearedBalance.String())
		}
	})

	t.Run("includes transactions in balance", func(t *testing.T) {
		acct := account.NewAccount(
			"Checking With Transactions",
			account.TypeChecking,
			"USD",
			types.MustNewMoney("1000.00"),
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		// Insert transactions directly for testing
		_, err := database.Conn().Exec(`
			INSERT INTO transactions (id, account_id, date, amount, status)
			VALUES
				(gen_random_uuid(), ?, CURRENT_DATE, -100.00, 'cleared'),
				(gen_random_uuid(), ?, CURRENT_DATE, -50.00, 'uncleared')
		`, acct.ID.String(), acct.ID.String())
		if err != nil {
			t.Fatalf("Failed to insert transactions: %v", err)
		}

		balance, err := svc.GetBalance(acct.ID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		// Current balance = 1000 - 100 - 50 = 850
		if !balance.CurrentBalance.Equal(types.MustNewMoney("850.00")) {
			t.Errorf("Expected current balance 850.00, got %s", balance.CurrentBalance.String())
		}

		// Cleared balance = 1000 - 100 = 900 (pending transaction excluded)
		if !balance.ClearedBalance.Equal(types.MustNewMoney("900.00")) {
			t.Errorf("Expected cleared balance 900.00, got %s", balance.ClearedBalance.String())
		}
	})
}

func TestAccountServiceGetAllBalances(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("returns balances for all accounts", func(t *testing.T) {
		accounts := []*account.Account{
			account.NewAccount("Account 1", account.TypeChecking, "USD", types.MustNewMoney("100"), types.Today()),
			account.NewAccount("Account 2", account.TypeSavings, "USD", types.MustNewMoney("200"), types.Today()),
			account.NewAccount("Account 3", account.TypeCash, "USD", types.MustNewMoney("300"), types.Today()),
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
		acct := account.NewAccount(
			"Account To Close",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(acct.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		retrieved, err := svc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if retrieved.Active {
			t.Error("Expected account to be inactive")
		}
	})

	t.Run("rejects closing account with non-zero balance", func(t *testing.T) {
		acct := account.NewAccount(
			"Account With Balance",
			account.TypeChecking,
			"USD",
			types.MustNewMoney("100.00"),
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		err := svc.Close(acct.ID)
		if err == nil {
			t.Error("Expected error when closing account with balance")
		}
		if _, ok := err.(*account.HasBalanceError); !ok {
			t.Errorf("Expected AccountHasBalanceError, got %T: %v", err, err)
		}

		// Verify account is still active
		retrieved, err := svc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if !retrieved.Active {
			t.Error("Account should still be active after failed close")
		}
	})

	t.Run("rejects closing already closed account", func(t *testing.T) {
		acct := account.NewAccount(
			"Already Closed",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(acct.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		err := svc.Close(acct.ID)
		if err == nil {
			t.Error("Expected error when closing already closed account")
		}
		if _, ok := err.(*account.AlreadyClosedError); !ok {
			t.Errorf("Expected AccountAlreadyClosedError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceReopen(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("reopens closed account", func(t *testing.T) {
		acct := account.NewAccount(
			"Account To Reopen",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Close(acct.ID); err != nil {
			t.Fatalf("Failed to close account: %v", err)
		}

		if err := svc.Reopen(acct.ID); err != nil {
			t.Fatalf("Failed to reopen account: %v", err)
		}

		retrieved, err := svc.GetByID(acct.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve account: %v", err)
		}
		if !retrieved.Active {
			t.Error("Expected account to be active after reopen")
		}
	})

	t.Run("rejects reopening active account", func(t *testing.T) {
		acct := account.NewAccount(
			"Active Account",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		err := svc.Reopen(acct.ID)
		if err == nil {
			t.Error("Expected error when reopening active account")
		}
		if _, ok := err.(*account.NotClosedError); !ok {
			t.Errorf("Expected AccountNotClosedError, got %T: %v", err, err)
		}
	})
}

func TestAccountServiceList(t *testing.T) {
	svc, _, cleanup := createTestService(t)
	defer cleanup()

	t.Run("lists all accounts", func(t *testing.T) {
		accounts := []*account.Account{
			account.NewAccount("Active 1", account.TypeChecking, "USD", types.ZeroMoney, types.Today()),
			account.NewAccount("Active 2", account.TypeSavings, "USD", types.ZeroMoney, types.Today()),
			account.NewAccount("Closed", account.TypeCash, "USD", types.ZeroMoney, types.Today()),
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
		acct := account.NewAccount(
			"Account To Delete",
			account.TypeChecking,
			"USD",
			types.ZeroMoney,
			types.Today(),
		)
		if err := svc.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}

		if err := svc.Delete(acct.ID); err != nil {
			t.Fatalf("Failed to delete account: %v", err)
		}

		_, err := svc.GetByID(acct.ID)
		if err == nil {
			t.Error("Expected error when getting deleted account")
		}
	})
}
