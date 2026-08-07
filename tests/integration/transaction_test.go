package integration

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// TestTransactionLifecycle tests the complete transaction lifecycle:
// create database -> create account -> create transaction -> list -> update -> delete -> cleanup
func TestTransactionLifecycle(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create an account first (required for transactions)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.NewDate(2024, 1, 1),
	)
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	var transactionID types.ID

	// Step 1: Create a transaction
	t.Run("Create transaction", func(t *testing.T) {
		txn := transaction.NewTransaction(
			acct.ID,
			types.NewDate(2024, 1, 15),
			types.MustNewMoney("-50.00"),
		)
		txn.SetMemo("Coffee shop")

		err := txnRepo.Create(txn)
		if err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
		transactionID = txn.ID
	})

	// Step 2: List transactions
	t.Run("List transactions", func(t *testing.T) {
		transactions, err := txnRepo.List()
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}

		if len(transactions) != 1 {
			t.Fatalf("Expected 1 transaction, got %d", len(transactions))
		}

		retrieved := transactions[0]
		if !retrieved.Amount.Equal(types.MustNewMoney("-50.00")) {
			t.Errorf("Expected amount '-50.00', got %q", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "Coffee shop" {
			t.Errorf("Expected memo 'Coffee shop', got %v", retrieved.Memo)
		}
		if retrieved.Status != transaction.StatusUncleared {
			t.Errorf("Expected status 'uncleared', got %q", retrieved.Status)
		}
	})

	// Step 3: Get by ID
	t.Run("Get transaction by ID", func(t *testing.T) {
		txn, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}
		if txn.ID != transactionID {
			t.Errorf("Expected ID %s, got %s", transactionID.String(), txn.ID.String())
		}
	})

	// Step 4: Update transaction
	t.Run("Update transaction", func(t *testing.T) {
		txn, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get transaction: %v", err)
		}

		txn.Clear() // Mark as cleared
		txn.SetMemo("Coffee shop - updated")

		err = txnRepo.Update(txn)
		if err != nil {
			t.Fatalf("Failed to update transaction: %v", err)
		}

		// Verify update
		updated, err := txnRepo.GetByID(transactionID)
		if err != nil {
			t.Fatalf("Failed to get updated transaction: %v", err)
		}
		if updated.Status != transaction.StatusCleared {
			t.Errorf("Expected status 'cleared', got %q", updated.Status)
		}
		if !updated.Memo.Valid || updated.Memo.String != "Coffee shop - updated" {
			t.Errorf("Expected memo 'Coffee shop - updated', got %v", updated.Memo)
		}
	})

	// Step 5: Delete transaction
	t.Run("Delete transaction", func(t *testing.T) {
		err := txnRepo.Delete(transactionID)
		if err != nil {
			t.Fatalf("Failed to delete transaction: %v", err)
		}

		// Verify deletion
		transactions, err := txnRepo.List()
		if err != nil {
			t.Fatalf("Failed to list transactions: %v", err)
		}
		if len(transactions) != 0 {
			t.Errorf("Expected 0 transactions after deletion, got %d", len(transactions))
		}
	})
}

// TestTransactionListByAccount tests listing transactions by account.
func TestTransactionListByAccount(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create two accounts
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today())

	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("Failed to create checking account: %v", err)
	}
	if err := accountRepo.Create(savings); err != nil {
		t.Fatalf("Failed to create savings account: %v", err)
	}

	// Create transactions for each account
	txn1 := transaction.NewTransaction(checking.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-100.00"))
	txn2 := transaction.NewTransaction(checking.ID, types.NewDate(2024, 1, 15), types.MustNewMoney("-50.00"))
	txn3 := transaction.NewTransaction(savings.ID, types.NewDate(2024, 1, 12), types.MustNewMoney("500.00"))

	for _, txn := range []*transaction.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	// List transactions by checking account
	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("Failed to list checking transactions: %v", err)
	}
	if len(checkingTxns) != 2 {
		t.Errorf("Expected 2 checking transactions, got %d", len(checkingTxns))
	}

	// List transactions by savings account
	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("Failed to list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Errorf("Expected 1 savings transaction, got %d", len(savingsTxns))
	}

	// Verify ordering (most recent first)
	if checkingTxns[0].Date.Before(checkingTxns[1].Date) {
		t.Error("Transactions should be ordered by date descending")
	}
}

// TestTransactionNotFound tests error handling for non-existent transactions.
func TestTransactionNotFound(t *testing.T) {
	database := dbtest.New(t)

	txnRepo := transaction.NewRepository(database)

	// Try to get non-existent transaction
	_, err := txnRepo.GetByID(types.NewID())
	if err == nil {
		t.Error("Expected error for non-existent transaction")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}

	// Try to delete non-existent transaction
	err = txnRepo.Delete(types.NewID())
	if err == nil {
		t.Error("Expected error for deleting non-existent transaction")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

// TestTransactionRequiresValidAccount tests that transactions require a valid account.
func TestTransactionRequiresValidAccount(t *testing.T) {
	database := dbtest.New(t)

	txnRepo := transaction.NewRepository(database)

	// Try to create transaction with non-existent account
	txn := transaction.NewTransaction(
		types.NewID(), // Random account ID that doesn't exist
		types.Today(),
		types.MustNewMoney("-50.00"),
	)

	err := txnRepo.Create(txn)
	if err == nil {
		t.Error("Expected error when creating transaction with non-existent account")
	}
	if _, ok := err.(*dberrors.NotFoundError); !ok {
		t.Errorf("Expected NotFoundError, got %T: %v", err, err)
	}
}

// TestTransactionCountByAccount tests the CountByAccount method.
func TestTransactionCountByAccount(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Initially should be zero
	count, err := txnRepo.CountByAccount(acct.ID)
	if err != nil {
		t.Fatalf("Failed to count transactions: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 transactions, got %d", count)
	}

	// Create some transactions
	for range 3 {
		txn := transaction.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-10.00"))
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	// Should now be 3
	count, err = txnRepo.CountByAccount(acct.ID)
	if err != nil {
		t.Fatalf("Failed to count transactions: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 transactions, got %d", count)
	}
}

// TestTransactionWithAllFields tests creating a transaction with all optional fields.
func TestTransactionWithAllFields(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	payeeRepo := payee.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create category
	cat := category.NewCategory("Food", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	// Create payee
	py := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	// Create transaction with all fields
	txn := transaction.NewTransactionFull(
		acct.ID,
		types.NewDate(2024, 1, 15),
		types.MustNewMoney("-25.50"),
		py.ID,
		cat.ID,
		"Morning coffee",
	)
	txn.SetCheckNumber("1234")
	txn.Clear()

	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Retrieve and verify all fields
	retrieved, err := txnRepo.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("Failed to get transaction: %v", err)
	}

	if !retrieved.PayeeID.Valid || retrieved.PayeeID.ID != py.ID {
		t.Errorf("Expected payee ID %s, got %v", py.ID.String(), retrieved.PayeeID)
	}
	if !retrieved.CategoryID.Valid || retrieved.CategoryID.ID != cat.ID {
		t.Errorf("Expected category ID %s, got %v", cat.ID.String(), retrieved.CategoryID)
	}
	if !retrieved.Memo.Valid || retrieved.Memo.String != "Morning coffee" {
		t.Errorf("Expected memo 'Morning coffee', got %v", retrieved.Memo)
	}
	if !retrieved.CheckNumber.Valid || retrieved.CheckNumber.String != "1234" {
		t.Errorf("Expected check number '1234', got %v", retrieved.CheckNumber)
	}
	if retrieved.Status != transaction.StatusCleared {
		t.Errorf("Expected status 'cleared', got %q", retrieved.Status)
	}
}

// TestAccountWithTransactionCannotBeDeleted tests that accounts with transactions cannot be deleted.
func TestAccountWithTransactionCannotBeDeleted(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create transaction
	txn := transaction.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-10.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("Failed to create transaction: %v", err)
	}

	// Try to delete account
	err := accountRepo.Delete(acct.ID)
	if err == nil {
		t.Error("Expected error when deleting account with transactions")
	}
	if _, ok := err.(*dberrors.HasDependentsError); !ok {
		t.Errorf("Expected HasDependentsError, got %T: %v", err, err)
	}
}

// TestTransactionSearchByPayee tests searching transactions by payee name.
func TestTransactionSearchByPayee(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	payeeRepo := payee.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create payees
	coffeeShop := payee.NewPayee("Coffee Shop")
	grocery := payee.NewPayee("Grocery Store")
	gasStation := payee.NewPayee("Gas Station")

	for _, py := range []*payee.Payee{coffeeShop, grocery, gasStation} {
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}
	}

	// Create transactions
	txn1 := transaction.NewTransactionWithPayee(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-5.00"), coffeeShop.ID)
	txn2 := transaction.NewTransactionWithPayee(acct.ID, types.NewDate(2024, 1, 11), types.MustNewMoney("-50.00"), grocery.ID)
	txn3 := transaction.NewTransactionWithPayee(acct.ID, types.NewDate(2024, 1, 12), types.MustNewMoney("-30.00"), gasStation.ID)

	for _, txn := range []*transaction.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	t.Run("Search by exact payee name", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{PayeeName: "Coffee Shop"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Search by partial payee name", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{PayeeName: "shop"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result (Coffee Shop), got %d", len(results))
		}
	})

	t.Run("Search is case-insensitive", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{PayeeName: "COFFEE"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Search with no matches", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{PayeeName: "Restaurant"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})

	t.Run("Search matches multiple payees", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{PayeeName: "store"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result (Grocery Store), got %d", len(results))
		}
	})
}

// TestTransactionSearchByMemo tests searching transactions by memo.
func TestTransactionSearchByMemo(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create transactions with different memos
	txn1 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-5.00"))
	txn1.SetMemo("Morning coffee")

	txn2 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 11), types.MustNewMoney("-50.00"))
	txn2.SetMemo("Weekly groceries")

	txn3 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 12), types.MustNewMoney("-3.50"))
	txn3.SetMemo("Afternoon coffee break")

	for _, txn := range []*transaction.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	t.Run("Search by memo keyword", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{Memo: "coffee"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results (both coffee transactions), got %d", len(results))
		}
	})

	t.Run("Search is case-insensitive", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{Memo: "WEEKLY"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Search with no matches", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{Memo: "dinner"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})
}

// TestTransactionSearchByCategory tests searching transactions by category name.
func TestTransactionSearchByCategory(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	// Create account
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Create categories
	food := category.NewCategory("Food & Dining", category.TypeExpense)
	transport := category.NewCategory("Transportation", category.TypeExpense)
	utilities := category.NewCategory("Utilities", category.TypeExpense)

	for _, cat := range []*category.Category{food, transport, utilities} {
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
	}

	// Create transactions
	txn1 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-25.00"))
	txn1.SetCategory(food.ID)

	txn2 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 11), types.MustNewMoney("-40.00"))
	txn2.SetCategory(transport.ID)

	txn3 := transaction.NewTransaction(acct.ID, types.NewDate(2024, 1, 12), types.MustNewMoney("-100.00"))
	txn3.SetCategory(utilities.ID)

	for _, txn := range []*transaction.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	t.Run("Search by category name", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{CategoryName: "Food"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Search is case-insensitive", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{CategoryName: "transportation"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Search by partial category name", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{CategoryName: "util"})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})
}

// TestTransactionSearchCombinedCriteria tests searching with multiple criteria.
func TestTransactionSearchCombinedCriteria(t *testing.T) {
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	// Create accounts
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today())

	for _, acct := range []*account.Account{checking, savings} {
		if err := accountRepo.Create(acct); err != nil {
			t.Fatalf("Failed to create account: %v", err)
		}
	}

	// Create payees
	coffeeShop := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(coffeeShop); err != nil {
		t.Fatalf("Failed to create payee: %v", err)
	}

	// Create category
	food := category.NewCategory("Food", category.TypeExpense)
	if err := categoryRepo.Create(food); err != nil {
		t.Fatalf("Failed to create category: %v", err)
	}

	// Create transactions with various combinations
	txn1 := transaction.NewTransactionWithPayee(checking.ID, types.NewDate(2024, 1, 10), types.MustNewMoney("-5.00"), coffeeShop.ID)
	txn1.SetCategory(food.ID)
	txn1.SetMemo("Morning latte")

	txn2 := transaction.NewTransactionWithPayee(checking.ID, types.NewDate(2024, 2, 15), types.MustNewMoney("-5.50"), coffeeShop.ID)
	txn2.SetCategory(food.ID)
	txn2.SetMemo("Afternoon coffee")

	txn3 := transaction.NewTransactionWithPayee(savings.ID, types.NewDate(2024, 1, 20), types.MustNewMoney("-4.00"), coffeeShop.ID)
	txn3.SetCategory(food.ID)
	txn3.SetMemo("Quick espresso")

	for _, txn := range []*transaction.Transaction{txn1, txn2, txn3} {
		if err := txnRepo.Create(txn); err != nil {
			t.Fatalf("Failed to create transaction: %v", err)
		}
	}

	t.Run("Search by payee and date range", func(t *testing.T) {
		startDate := types.NewDate(2024, 1, 1)
		endDate := types.NewDate(2024, 1, 31)
		results, err := txnRepo.Search(transaction.SearchCriteria{
			PayeeName: "Coffee",
			StartDate: &startDate,
			EndDate:   &endDate,
		})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results (January only), got %d", len(results))
		}
	})

	t.Run("Search by account and memo", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{
			AccountID: &checking.ID,
			Memo:      "coffee",
		})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result (afternoon coffee in checking), got %d", len(results))
		}
	})

	t.Run("Search by category, payee, and account", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{
			CategoryName: "Food",
			PayeeName:    "Coffee",
			AccountID:    &savings.ID,
		})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result (savings account only), got %d", len(results))
		}
	})

	t.Run("Search with no matching criteria", func(t *testing.T) {
		startDate := types.NewDate(2024, 3, 1)
		endDate := types.NewDate(2024, 3, 31)
		results, err := txnRepo.Search(transaction.SearchCriteria{
			PayeeName: "Coffee",
			StartDate: &startDate,
			EndDate:   &endDate,
		})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})

	t.Run("Empty criteria returns all transactions", func(t *testing.T) {
		results, err := txnRepo.Search(transaction.SearchCriteria{})
		if err != nil {
			t.Fatalf("Failed to search: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results (all transactions), got %d", len(results))
		}
	})
}
