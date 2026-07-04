package transaction

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

func createTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
}

func createTestTransactionService(t *testing.T) (*Service, *account.Repository) {
	t.Helper()
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	return svc, accountRepo
}

func createTestAccount(t *testing.T, repo *account.Repository, name string) *account.Account {
	t.Helper()
	return createTestAccountOfType(t, repo, name, account.TypeChecking)
}

func createTestAccountOfType(t *testing.T, repo *account.Repository, name string, accountType account.Type) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, accountType, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}
	return acct
}

func TestNewService(t *testing.T) {
	t.Run("creates service with repositories", func(t *testing.T) {
		svc, _ := createTestTransactionService(t)
		if svc == nil {
			t.Error("NewService should not return nil")
		}
	})
}

func TestTransactionService_Create(t *testing.T) {
	t.Run("creates valid transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		err := svc.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it was created
		retrieved, err := svc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if !retrieved.Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), retrieved.Amount.String())
		}
	})

	t.Run("validates transaction before creating", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		// Zero amount is invalid
		txn := NewTransaction(acct.ID, types.Today(), types.ZeroMoney)
		err := svc.Create(txn)
		if err == nil {
			t.Error("Create() expected error for zero amount")
		}
		if _, ok := err.(*types.ServiceValidationError); !ok {
			t.Errorf("Expected ServiceValidationError, got %T", err)
		}
	})

	t.Run("auto-populates category from payee default", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		// Create account, category, and payee with default category
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Groceries", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		py := payee.NewPayeeWithCategory("Kroger", cat.ID)
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		// Create transaction with payee but no category
		amount, _ := types.NewMoney("-100.00")
		txn := NewTransactionWithPayee(acct.ID, types.Today(), amount, py.ID)

		err := svc.Create(txn)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify category was auto-populated
		retrieved, _ := svc.GetByID(txn.ID)
		if !retrieved.HasCategory() {
			t.Error("Expected category to be auto-populated from payee")
		}
		if retrieved.CategoryID.ID != cat.ID {
			t.Errorf("Expected category %s, got %s", cat.ID.String(), retrieved.CategoryID.ID.String())
		}
	})
}

func TestTransactionService_Update(t *testing.T) {
	t.Run("updates transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Update the amount
		newAmount, _ := types.NewMoney("-75.00")
		txn.Amount = newAmount
		if err := svc.Update(txn); err != nil {
			t.Fatalf("Update() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if !retrieved.Amount.Equal(newAmount) {
			t.Errorf("Expected amount %s, got %s", newAmount.String(), retrieved.Amount.String())
		}
	})

	t.Run("validates transaction before updating", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Invalid update: zero amount
		txn.Amount = types.ZeroMoney
		err := svc.Update(txn)
		if err == nil {
			t.Error("Update() expected error for zero amount")
		}
	})
}

func TestTransactionService_Delete(t *testing.T) {
	t.Run("deletes transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.Delete(txn.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		_, err := svc.GetByID(txn.ID)
		if err == nil {
			t.Error("GetByID() expected error after delete")
		}
	})

	t.Run("deletes transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		split := NewSplit(txn.ID, cat.ID, splitAmount)

		if err := svc.CreateWithSplits(txn, []*Split{split}); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Delete should succeed and remove splits too
		if err := svc.Delete(txn.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify splits are gone
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 0 {
			t.Errorf("Expected 0 splits after delete, got %d", len(splits))
		}
	})
}

func TestTransactionService_List(t *testing.T) {
	t.Run("lists all transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount1, _ := types.NewMoney("-50.00")
		amount2, _ := types.NewMoney("-75.00")

		if err := svc.Create(NewTransaction(acct.ID, types.Today(), amount1)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(acct.ID, types.Today(), amount2)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions, got %d", len(txns))
		}
	})

	t.Run("lists transactions by account", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account1 := createTestAccount(t, accountRepo, "Checking")
		account2 := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("-50.00")

		if err := svc.Create(NewTransaction(account1.ID, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(account2.ID, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.ListByAccount(account1.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction for account1, got %d", len(txns))
		}
	})
}

func TestTransactionService_CreateWithSplits(t *testing.T) {
	t.Run("creates transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		split1Amount, _ := types.NewMoney("-70.00")
		split2Amount, _ := types.NewMoney("-30.00")
		splits := []*Split{
			NewSplit(txn.ID, cat1.ID, split1Amount),
			NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Verify splits were created
		retrievedSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(retrievedSplits) != 2 {
			t.Errorf("Expected 2 splits, got %d", len(retrievedSplits))
		}
	})

	t.Run("rejects splits that don't sum to transaction amount", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		// Splits don't sum to -100
		splitAmount, _ := types.NewMoney("-50.00")
		splits := []*Split{
			NewSplit(txn.ID, cat.ID, splitAmount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("CreateWithSplits() expected error for mismatched split totals")
		}
	})

	t.Run("rejects transaction with both category and splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		txn.SetCategory(cat.ID) // Has category

		splitAmount, _ := types.NewMoney("-100.00")
		splits := []*Split{
			NewSplit(txn.ID, cat.ID, splitAmount),
		}

		err := svc.CreateWithSplits(txn, splits)
		if err == nil {
			t.Error("CreateWithSplits() expected error when transaction has category and splits")
		}
		if _, ok := err.(*HasSplitsError); !ok {
			t.Errorf("Expected TransactionHasSplitsError, got %T: %v", err, err)
		}
	})
}

// TestTransactionService_MixedSignSplit_Allowed covers MS-005: split lines may
// have signs independent of the parent amount as long as their signed sum
// equals the parent. Paycheck-shaped transactions need this: gross income is
// positive, withholdings are negative, and the parent's net is the deposit.
func TestTransactionService_MixedSignSplit_Allowed(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	acct := createTestAccount(t, accountRepo, "Checking")

	income := category.NewCategory("Salary", category.TypeIncome)
	tax := category.NewCategory("Tax", category.TypeExpense)
	if err := categoryRepo.Create(income); err != nil {
		t.Fatalf("Failed to create income category: %v", err)
	}
	if err := categoryRepo.Create(tax); err != nil {
		t.Fatalf("Failed to create tax category: %v", err)
	}

	// Parent net: +100. Lines: +200 (gross) and -100 (tax). Signed sum = +100.
	parentAmount, _ := types.NewMoney("100.00")
	txn := NewTransaction(acct.ID, types.Today(), parentAmount)

	grossAmount, _ := types.NewMoney("200.00")
	taxAmount, _ := types.NewMoney("-100.00")
	splits := []*Split{
		NewSplit(txn.ID, income.ID, grossAmount),
		NewSplit(txn.ID, tax.ID, taxAmount),
	}

	if err := svc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("CreateWithSplits() with mixed signs error = %v", err)
	}

	retrieved, err := svc.GetSplits(txn.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 splits, got %d", len(retrieved))
	}

	total := SplitCollection(retrieved).Total()
	if !total.Equal(parentAmount) {
		t.Errorf("Expected split total %s, got %s", parentAmount.String(), total.String())
	}
}

// TestTransactionService_LegacySameSignSplit_StillWorks covers MS-005: the
// relaxed signed-sum check must keep legacy same-sign splits valid.
func TestTransactionService_LegacySameSignSplit_StillWorks(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	acct := createTestAccount(t, accountRepo, "Checking")

	food := category.NewCategory("Food", category.TypeExpense)
	household := category.NewCategory("Household", category.TypeExpense)
	if err := categoryRepo.Create(food); err != nil {
		t.Fatalf("Failed to create food category: %v", err)
	}
	if err := categoryRepo.Create(household); err != nil {
		t.Fatalf("Failed to create household category: %v", err)
	}

	parentAmount, _ := types.NewMoney("-100.00")
	txn := NewTransaction(acct.ID, types.Today(), parentAmount)

	foodAmount, _ := types.NewMoney("-70.00")
	householdAmount, _ := types.NewMoney("-30.00")
	splits := []*Split{
		NewSplit(txn.ID, food.ID, foodAmount),
		NewSplit(txn.ID, household.ID, householdAmount),
	}

	if err := svc.CreateWithSplits(txn, splits); err != nil {
		t.Fatalf("CreateWithSplits() with same-sign splits error = %v", err)
	}

	retrieved, err := svc.GetSplits(txn.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(retrieved) != 2 {
		t.Fatalf("Expected 2 splits, got %d", len(retrieved))
	}
}

// TestTransactionService_TransferLine_CreatesPair covers MS-006: when a
// transaction is created with a transfer-typed split-line, the service mints
// a fresh transfer_id, stores it on the split-item, and creates a paired
// single-line transaction in the target account whose transfer_id matches
// and whose amount inverts the line's signed amount.
func TestTransactionService_TransferLine_CreatesPair(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	// Parent net +700: salary +1000, transfer -300 to Savings.
	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))

	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}

	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(reloaded) != 2 {
		t.Fatalf("Expected 2 splits, got %d", len(reloaded))
	}

	var xfer *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found in reloaded splits")
	}
	if !xfer.TransferID.Valid || xfer.TransferID.ID.IsNil() {
		t.Errorf("transfer-line should have a transfer_id minted, got %v", xfer.TransferID)
	}
	if xfer.TransferAccountID.ID != savings.ID {
		t.Errorf("transfer-line.TransferAccountID = %v, want %v",
			xfer.TransferAccountID.ID, savings.ID)
	}

	pairedList, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(pairedList) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings, got %d", len(pairedList))
	}
	paired := pairedList[0]
	if !paired.Amount.Equal(types.MustNewMoney("300.00")) {
		t.Errorf("paired.Amount = %s, want 300.00", paired.Amount.String())
	}
	if !paired.TransferID.Valid || paired.TransferID.ID != xfer.TransferID.ID {
		t.Errorf("paired.TransferID = %v, want %v", paired.TransferID, xfer.TransferID)
	}
	if !paired.TransferAccountID.Valid || paired.TransferAccountID.ID != checking.ID {
		t.Errorf("paired.TransferAccountID = %v, want %v (parent's account)",
			paired.TransferAccountID, checking.ID)
	}
	if paired.AccountID != savings.ID {
		t.Errorf("paired.AccountID = %v, want %v (savings)",
			paired.AccountID, savings.ID)
	}
	if !paired.Date.Equal(parent.Date) {
		t.Errorf("paired.Date = %v, want parent date %v", paired.Date, parent.Date)
	}
	// The parent itself must not be marked as a transfer; only the split-item
	// carries the transfer linkage on the parent's side.
	parentReload, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID(parent) error = %v", err)
	}
	if parentReload.IsTransfer() {
		t.Errorf("parent transaction should not be a transfer; transfer_id should be NULL on parent row")
	}
}

// TestTransactionService_SelfTransfer_Rejected covers MS-006: a transfer-line
// whose target account equals the parent transaction's account must be
// rejected by validation.
func TestTransactionService_SelfTransfer_Rejected(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	checking := createTestAccount(t, accountRepo, "Checking")

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-100.00"),
		TransferAccountID: types.NullableID{ID: checking.ID, Valid: true},
	}

	err := svc.CreateWithSplits(parent, []*Split{transferLine})
	if err == nil {
		t.Fatal("CreateWithSplits() expected error for self-transfer, got nil")
	}
	if _, ok := err.(*types.ServiceValidationError); !ok {
		t.Errorf("Expected ServiceValidationError, got %T: %v", err, err)
	}

	// The parent must not have been persisted; the listing for the account is
	// empty.
	list, err := svc.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("Expected no transactions after rejected self-transfer, got %d", len(list))
	}
}

// TestTransactionService_EditTransferLineAmount_UpdatesPair covers MS-007:
// editing a transfer-line's amount cascades to the paired single-line
// counter-transaction in the target account so both sides stay equal-and-
// opposite. The transfer_id linking the pair must be preserved.
func TestTransactionService_EditTransferLineAmount_UpdatesPair(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Reload to capture the minted transfer_id and the paired counterpart.
	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found in reloaded splits")
	}
	originalTransferID := xfer.TransferID.ID

	pairedBefore, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(pairedBefore) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings before edit, got %d", len(pairedBefore))
	}
	pairedID := pairedBefore[0].ID

	// Edit the transfer-line amount to -400. UpdateSplit doesn't enforce
	// parent rebalance — that is the caller's (UI's) responsibility — so the
	// cascade is what we're verifying.
	xfer.Amount = types.MustNewMoney("-400.00")
	if err := svc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit() error = %v", err)
	}

	// The split's amount is persisted; the transfer_id is unchanged.
	gotSplit, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after update: %v", err)
	}
	var afterXfer *Split
	for _, s := range gotSplit {
		if s.TransferAccountID.Valid {
			afterXfer = s
		}
	}
	if afterXfer == nil {
		t.Fatalf("transfer-line missing after update")
	}
	if !afterXfer.Amount.Equal(types.MustNewMoney("-400.00")) {
		t.Errorf("split amount after update = %s, want -400.00", afterXfer.Amount.String())
	}
	if !afterXfer.TransferID.Valid || afterXfer.TransferID.ID != originalTransferID {
		t.Errorf("transfer_id changed on amount edit: got %v want %v",
			afterXfer.TransferID, originalTransferID)
	}

	// The paired counterpart's amount mirrors the new amount (negated). Its
	// ID, account, and transfer_id remain unchanged.
	pairedAfter, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) after update: %v", err)
	}
	if len(pairedAfter) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings after edit, got %d", len(pairedAfter))
	}
	paired := pairedAfter[0]
	if paired.ID != pairedID {
		t.Errorf("paired transaction was replaced (id %v -> %v); amount edits should mutate in place",
			pairedID, paired.ID)
	}
	if !paired.Amount.Equal(types.MustNewMoney("400.00")) {
		t.Errorf("paired.Amount = %s, want 400.00", paired.Amount.String())
	}
	if !paired.TransferID.Valid || paired.TransferID.ID != originalTransferID {
		t.Errorf("paired.TransferID = %v, want %v", paired.TransferID, originalTransferID)
	}
}

// TestTransactionService_EditTransferLineTarget_MovesPair covers MS-007:
// changing the target account on a transfer-line deletes the old paired
// counter-transaction and creates a new one in the new target with a fresh
// transfer_id (so the old linkage is replaced rather than mutated in place).
func TestTransactionService_EditTransferLineTarget_MovesPair(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	hsa := createTestAccount(t, accountRepo, "HSA")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found in reloaded splits")
	}
	oldTransferID := xfer.TransferID.ID

	// Move the target from Savings to HSA, keeping the amount.
	xfer.TransferAccountID = types.NullableID{ID: hsa.ID, Valid: true}
	if err := svc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit() error = %v", err)
	}

	// The old target (Savings) has no paired transaction anymore.
	savingsTxns, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) after update: %v", err)
	}
	if len(savingsTxns) != 0 {
		t.Errorf("expected 0 paired transactions in old target Savings, got %d", len(savingsTxns))
	}

	// The new target (HSA) has the new paired transaction.
	hsaTxns, err := svc.ListByAccount(hsa.ID)
	if err != nil {
		t.Fatalf("ListByAccount(hsa) after update: %v", err)
	}
	if len(hsaTxns) != 1 {
		t.Fatalf("expected 1 paired transaction in new target HSA, got %d", len(hsaTxns))
	}
	newPaired := hsaTxns[0]
	if !newPaired.Amount.Equal(types.MustNewMoney("300.00")) {
		t.Errorf("new paired.Amount = %s, want 300.00", newPaired.Amount.String())
	}
	if !newPaired.TransferID.Valid || newPaired.TransferID.ID == oldTransferID {
		t.Errorf("new paired.TransferID = %v, want fresh id (not %v)",
			newPaired.TransferID, oldTransferID)
	}
	if !newPaired.TransferAccountID.Valid || newPaired.TransferAccountID.ID != checking.ID {
		t.Errorf("new paired.TransferAccountID = %v, want %v (parent's account)",
			newPaired.TransferAccountID, checking.ID)
	}

	// The split-item now references the new transfer_id and the new account.
	gotSplit, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after target move: %v", err)
	}
	var afterXfer *Split
	for _, s := range gotSplit {
		if s.TransferAccountID.Valid {
			afterXfer = s
		}
	}
	if afterXfer == nil {
		t.Fatalf("transfer-line missing after target move")
	}
	if afterXfer.TransferAccountID.ID != hsa.ID {
		t.Errorf("split.TransferAccountID = %v, want %v", afterXfer.TransferAccountID.ID, hsa.ID)
	}
	if !afterXfer.TransferID.Valid || afterXfer.TransferID.ID == oldTransferID {
		t.Errorf("split.TransferID = %v, want fresh id (not %v)",
			afterXfer.TransferID, oldTransferID)
	}
	if afterXfer.TransferID.ID != newPaired.TransferID.ID {
		t.Errorf("split.TransferID (%v) does not match new paired.TransferID (%v)",
			afterXfer.TransferID.ID, newPaired.TransferID.ID)
	}
}

// TestTransactionService_DeleteTransferLine_DeletesPair covers MS-008:
// deleting a single transfer-line split cascades to the paired
// counter-transaction in the target account. The parent transaction
// retains its remaining split-items even though the totals may now be
// out of balance (validation is the caller's responsibility on a later
// save).
func TestTransactionService_DeleteTransferLine_DeletesPair(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer, salaryReloaded *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		} else {
			salaryReloaded = s
		}
	}
	if xfer == nil || salaryReloaded == nil {
		t.Fatalf("expected one transfer-line and one salary line, got %d splits", len(reloaded))
	}

	// Sanity: the paired transaction exists in the savings register before
	// the delete.
	pairedBefore, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(pairedBefore) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings before delete, got %d", len(pairedBefore))
	}

	// Delete the transfer-line. The paired counter-transaction in Savings
	// must vanish; the salary line on the parent must remain.
	if err := svc.DeleteSplit(xfer.ID); err != nil {
		t.Fatalf("DeleteSplit() error = %v", err)
	}

	savingsAfter, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) after delete: %v", err)
	}
	if len(savingsAfter) != 0 {
		t.Errorf("expected 0 paired transactions in Savings after delete, got %d", len(savingsAfter))
	}

	remaining, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining split on parent, got %d", len(remaining))
	}
	if remaining[0].ID != salaryReloaded.ID {
		t.Errorf("remaining split id = %v, want salary line %v",
			remaining[0].ID, salaryReloaded.ID)
	}
}

// TestTransactionService_DeleteParent_DeletesAllPairs covers MS-008:
// deleting the parent transaction of a multi-line split with transfer-
// typed lines also deletes every paired counter-transaction in the
// target accounts.
func TestTransactionService_DeleteParent_DeletesAllPairs(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	hsa := createTestAccount(t, accountRepo, "HSA")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	// Parent net +500: salary +1000, transfer -300 to Savings, transfer -200 to HSA.
	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("500.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	toSavings := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	toHSA := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: hsa.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, toSavings, toHSA}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Sanity: both paired counter-transactions exist before delete.
	if list, err := svc.ListByAccount(savings.ID); err != nil {
		t.Fatalf("ListByAccount(savings): %v", err)
	} else if len(list) != 1 {
		t.Fatalf("expected 1 paired txn in Savings before parent delete, got %d", len(list))
	}
	if list, err := svc.ListByAccount(hsa.ID); err != nil {
		t.Fatalf("ListByAccount(hsa): %v", err)
	} else if len(list) != 1 {
		t.Fatalf("expected 1 paired txn in HSA before parent delete, got %d", len(list))
	}

	if err := svc.Delete(parent.ID); err != nil {
		t.Fatalf("Delete(parent) error = %v", err)
	}

	// All paired counter-transactions in target accounts are gone.
	if list, err := svc.ListByAccount(savings.ID); err != nil {
		t.Fatalf("ListByAccount(savings) after parent delete: %v", err)
	} else if len(list) != 0 {
		t.Errorf("expected 0 paired txns in Savings after parent delete, got %d", len(list))
	}
	if list, err := svc.ListByAccount(hsa.ID); err != nil {
		t.Fatalf("ListByAccount(hsa) after parent delete: %v", err)
	} else if len(list) != 0 {
		t.Errorf("expected 0 paired txns in HSA after parent delete, got %d", len(list))
	}

	// The parent itself, and its splits, are gone.
	if _, err := svc.GetByID(parent.ID); err == nil {
		t.Errorf("expected parent transaction to be deleted, but GetByID returned nil error")
	}
	if remaining, err := svc.GetSplits(parent.ID); err != nil {
		t.Fatalf("GetSplits() after parent delete: %v", err)
	} else if len(remaining) != 0 {
		t.Errorf("expected 0 splits on deleted parent, got %d", len(remaining))
	}
}

// TestTransactionService_DeletePairedSide_RemovesParentLine covers MS-009:
// deleting the paired single-line counter-transaction from the target
// account's register reverse-cascades to remove the corresponding split-item
// on the parent multi-line transaction. The parent's other split-items
// remain intact (the totals may now be out of balance, which is the caller's
// responsibility to resolve on a later save).
func TestTransactionService_DeletePairedSide_RemovesParentLine(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Reload to discover the salary line's id (CreateWithSplits keeps the
	// caller's struct ids; we just want them by reference).
	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer, salaryReloaded *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		} else {
			salaryReloaded = s
		}
	}
	if xfer == nil || salaryReloaded == nil {
		t.Fatalf("expected one transfer-line and one salary line, got %d splits", len(reloaded))
	}

	// Grab the paired counter-transaction from the Savings register.
	pairedList, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(pairedList) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings, got %d", len(pairedList))
	}
	paired := pairedList[0]

	// Delete the paired side from the target account. The parent's transfer-
	// line split-item must vanish; the salary line remains.
	if err := svc.Delete(paired.ID); err != nil {
		t.Fatalf("Delete(paired) error = %v", err)
	}

	// Paired transaction is gone.
	if savingsAfter, err := svc.ListByAccount(savings.ID); err != nil {
		t.Fatalf("ListByAccount(savings) after delete: %v", err)
	} else if len(savingsAfter) != 0 {
		t.Errorf("expected 0 paired transactions in Savings after delete, got %d", len(savingsAfter))
	}

	// Parent transaction still exists.
	if _, err := svc.GetByID(parent.ID); err != nil {
		t.Fatalf("parent transaction unexpectedly deleted: %v", err)
	}

	// Parent has exactly one remaining split: the salary line.
	remaining, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining split on parent (transfer-line removed), got %d", len(remaining))
	}
	if remaining[0].ID != salaryReloaded.ID {
		t.Errorf("remaining split id = %v, want salary line %v",
			remaining[0].ID, salaryReloaded.ID)
	}
	if remaining[0].TransferAccountID.Valid {
		t.Errorf("remaining split should be the categorized salary line, but TransferAccountID is set")
	}
}

// TestTransactionService_EditPairedSideAmount_UpdatesParentLine covers MS-009:
// editing the paired side's amount reverse-cascades to the parent's transfer-
// line split-item, which mirrors the new (negated) amount. The transfer_id
// linking the pair is preserved.
func TestTransactionService_EditPairedSideAmount_UpdatesParentLine(t *testing.T) {
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-300.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	reloaded, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range reloaded {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found in reloaded splits")
	}
	originalTransferID := xfer.TransferID.ID

	pairedList, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(pairedList) != 1 {
		t.Fatalf("expected 1 paired transaction in Savings, got %d", len(pairedList))
	}
	paired := pairedList[0]

	// Edit the paired side's amount from +300 to +400. The reverse cascade
	// must mirror -400 onto the parent's transfer-line split-item.
	paired.Amount = types.MustNewMoney("400.00")
	if err := svc.Update(paired); err != nil {
		t.Fatalf("Update(paired) error = %v", err)
	}

	// Paired side persisted with the new amount and unchanged transfer_id.
	pairedAfter, err := svc.GetByID(paired.ID)
	if err != nil {
		t.Fatalf("GetByID(paired) after update: %v", err)
	}
	if !pairedAfter.Amount.Equal(types.MustNewMoney("400.00")) {
		t.Errorf("paired.Amount after update = %s, want 400.00", pairedAfter.Amount.String())
	}
	if !pairedAfter.TransferID.Valid || pairedAfter.TransferID.ID != originalTransferID {
		t.Errorf("paired.TransferID changed on amount edit: got %v want %v",
			pairedAfter.TransferID, originalTransferID)
	}

	// Parent's transfer-line split mirrors the new amount as -400; the salary
	// line is untouched.
	after, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after update: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("expected 2 splits on parent after edit, got %d", len(after))
	}
	var afterXfer, afterSalary *Split
	for _, s := range after {
		if s.TransferAccountID.Valid {
			afterXfer = s
		} else {
			afterSalary = s
		}
	}
	if afterXfer == nil || afterSalary == nil {
		t.Fatalf("expected one transfer-line and one salary line, got %d splits", len(after))
	}
	if !afterXfer.Amount.Equal(types.MustNewMoney("-400.00")) {
		t.Errorf("parent transfer-line amount after paired-side edit = %s, want -400.00",
			afterXfer.Amount.String())
	}
	if !afterXfer.TransferID.Valid || afterXfer.TransferID.ID != originalTransferID {
		t.Errorf("parent transfer-line transfer_id changed: got %v want %v",
			afterXfer.TransferID, originalTransferID)
	}
	if !afterSalary.Amount.Equal(types.MustNewMoney("1000.00")) {
		t.Errorf("salary line should be untouched, got amount %s want 1000.00",
			afterSalary.Amount.String())
	}
}

func TestTransactionService_ValidateSplitTotals(t *testing.T) {
	t.Run("returns true when splits match", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		splits := []*Split{
			NewSplit(txn.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		valid, err := svc.ValidateSplitTotals(txn.ID)
		if err != nil {
			t.Fatalf("ValidateSplitTotals() error = %v", err)
		}
		if !valid {
			t.Error("ValidateSplitTotals() should return true when splits match")
		}
	})
}

func TestTransactionService_CreateTransfer(t *testing.T) {
	t.Run("creates transfer between accounts", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if pair.FromTransaction == nil || pair.ToTransaction == nil {
			t.Fatal("CreateTransfer() should return both transactions")
		}

		// Verify from transaction
		if !pair.FromTransaction.Amount.IsNegative() {
			t.Error("From transaction should have negative amount")
		}
		if pair.FromTransaction.AccountID != checking.ID {
			t.Error("From transaction should be in checking account")
		}

		// Verify to transaction
		if !pair.ToTransaction.Amount.IsPositive() {
			t.Error("To transaction should have positive amount")
		}
		if pair.ToTransaction.AccountID != savings.ID {
			t.Error("To transaction should be in savings account")
		}

		// Verify transfer link
		if !pair.FromTransaction.IsTransfer() {
			t.Error("From transaction should be marked as transfer")
		}
		if !pair.ToTransaction.IsTransfer() {
			t.Error("To transaction should be marked as transfer")
		}
	})

	t.Run("rejects negative transfer amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("-500.00")
		_, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err == nil {
			t.Error("CreateTransfer() expected error for negative amount")
		}
		if _, ok := err.(*InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T: %v", err, err)
		}
	})

	t.Run("rejects transfer to same account", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("500.00")
		_, err := svc.CreateTransfer(checking.ID, checking.ID, types.Today(), amount, "", types.NullableID{})
		if err == nil {
			t.Error("CreateTransfer() expected error for same account")
		}
	})
}

func TestTransactionService_DeleteTransfer(t *testing.T) {
	t.Run("deletes both sides of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Delete the transfer
		if err := svc.DeleteTransfer(pair.FromTransaction.TransferID.ID); err != nil {
			t.Fatalf("DeleteTransfer() error = %v", err)
		}

		// Both sides should be gone
		_, err = svc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("From transaction should be deleted")
		}
		_, err = svc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("To transaction should be deleted")
		}
	})

	t.Run("delete via Delete also deletes both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Delete via regular Delete (should detect transfer and delete both)
		if err := svc.Delete(pair.FromTransaction.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Both sides should be gone
		_, err = svc.GetByID(pair.FromTransaction.ID)
		if err == nil {
			t.Error("From transaction should be deleted")
		}
		_, err = svc.GetByID(pair.ToTransaction.ID)
		if err == nil {
			t.Error("To transaction should be deleted")
		}
	})
}

func TestTransactionService_UpdateTransfer(t *testing.T) {
	t.Run("updates both sides of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Update the transfer
		newAmount, _ := types.NewMoney("750.00")
		newDate, _ := types.ParseDate("2024-06-15")
		err = svc.UpdateTransfer(pair.FromTransaction.TransferID.ID, newDate, newAmount, "Savings transfer", StatusCleared, types.NullableID{})
		if err != nil {
			t.Fatalf("UpdateTransfer() error = %v", err)
		}

		// Verify both sides updated
		updatedPair, _ := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if !updatedPair.FromTransaction.Amount.Neg().Equal(newAmount) {
			t.Errorf("From amount should be -%s, got %s", newAmount.String(), updatedPair.FromTransaction.Amount.String())
		}
		if !updatedPair.ToTransaction.Amount.Equal(newAmount) {
			t.Errorf("To amount should be %s, got %s", newAmount.String(), updatedPair.ToTransaction.Amount.String())
		}
		if updatedPair.FromTransaction.Status != StatusCleared {
			t.Error("From status should be cleared")
		}
		if updatedPair.ToTransaction.Status != StatusCleared {
			t.Error("To status should be cleared")
		}
	})
}

func TestTransactionService_GetTransferCounterpart(t *testing.T) {
	t.Run("returns other side of transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Get counterpart from from-side
		other, err := svc.GetTransferCounterpart(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetTransferCounterpart() error = %v", err)
		}
		if other.ID != pair.ToTransaction.ID {
			t.Error("GetTransferCounterpart() should return to-transaction")
		}

		// Get counterpart from to-side
		other, err = svc.GetTransferCounterpart(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetTransferCounterpart() error = %v", err)
		}
		if other.ID != pair.FromTransaction.ID {
			t.Error("GetTransferCounterpart() should return from-transaction")
		}
	})
}

func TestTransactionService_IsTransfer(t *testing.T) {
	t.Run("returns true for transfer transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		isTransfer, err := svc.IsTransfer(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if !isTransfer {
			t.Error("IsTransfer() should return true for transfer")
		}
	})

	t.Run("returns false for regular transactions", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		isTransfer, err := svc.IsTransfer(txn.ID)
		if err != nil {
			t.Fatalf("IsTransfer() error = %v", err)
		}
		if isTransfer {
			t.Error("IsTransfer() should return false for regular transaction")
		}
	})
}

func TestTransactionService_StatusOperations(t *testing.T) {
	t.Run("ClearTransaction sets status to cleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusCleared {
			t.Errorf("Expected status cleared, got %s", retrieved.Status)
		}
	})

	t.Run("ReconcileTransaction sets status to reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusReconciled {
			t.Errorf("Expected status reconciled, got %s", retrieved.Status)
		}
	})

	t.Run("MarkTransactionUncleared sets status to uncleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Clear first
		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		// Then mark uncleared
		if err := svc.MarkTransactionUncleared(txn.ID); err != nil {
			t.Fatalf("MarkTransactionUncleared() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusUncleared {
			t.Errorf("Expected status uncleared, got %s", retrieved.Status)
		}
	})
}

func TestTransactionService_Duplicate(t *testing.T) {
	t.Run("duplicates transaction with today's date", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		oldDate, _ := types.ParseDate("2024-01-15")
		amount, _ := types.NewMoney("-50.00")
		original := NewTransaction(acct.ID, oldDate, amount)
		original.SetMemo("Original memo")
		if err := svc.Create(original); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Clear the original
		if err := svc.ClearTransaction(original.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		duplicate, err := svc.Duplicate(original.ID)
		if err != nil {
			t.Fatalf("Duplicate() error = %v", err)
		}

		if duplicate.ID == original.ID {
			t.Error("Duplicate should have new ID")
		}
		if !duplicate.Amount.Equal(original.Amount) {
			t.Error("Duplicate should have same amount")
		}
		if duplicate.Date == oldDate {
			t.Error("Duplicate should have today's date, not original date")
		}
		if duplicate.Status != StatusUncleared {
			t.Error("Duplicate should have uncleared status")
		}
	})

	t.Run("duplicates transaction with splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		original := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		splits := []*Split{
			NewSplit(original.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(original, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		duplicate, err := svc.Duplicate(original.ID)
		if err != nil {
			t.Fatalf("Duplicate() error = %v", err)
		}

		// Verify splits were duplicated
		dupSplits, err := svc.GetSplits(duplicate.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(dupSplits) != 1 {
			t.Errorf("Expected 1 split in duplicate, got %d", len(dupSplits))
		}
	})

	t.Run("rejects duplicating transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		_, err = svc.Duplicate(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Duplicate() expected error for transfer")
		}
		if _, ok := err.(*CannotDuplicateTransferError); !ok {
			t.Errorf("Expected CannotDuplicateTransferError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_GetBalanceImpact(t *testing.T) {
	t.Run("returns single impact for regular transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		impacts, err := svc.GetBalanceImpact(txn.ID)
		if err != nil {
			t.Fatalf("GetBalanceImpact() error = %v", err)
		}
		if len(impacts) != 1 {
			t.Errorf("Expected 1 impact, got %d", len(impacts))
		}
		if !impacts[0].Amount.Equal(amount) {
			t.Errorf("Expected amount %s, got %s", amount.String(), impacts[0].Amount.String())
		}
	})

	t.Run("returns two impacts for transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		impacts, err := svc.GetBalanceImpact(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetBalanceImpact() error = %v", err)
		}
		if len(impacts) != 2 {
			t.Errorf("Expected 2 impacts for transfer, got %d", len(impacts))
		}

		// One should be from (negative), one should be to (positive)
		hasFrom := false
		hasTo := false
		for _, impact := range impacts {
			if impact.IsTransferFrom {
				hasFrom = true
			}
			if impact.IsTransferTo {
				hasTo = true
			}
		}
		if !hasFrom || !hasTo {
			t.Error("Transfer should have both from and to impacts")
		}
	})
}

func TestTransactionService_AddSplit_TransferError(t *testing.T) {
	t.Run("rejects adding split to transfer", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		cat := category.NewCategory("Transfer", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Try to add a split to the transfer
		splitAmount, _ := types.NewMoney("-500.00")
		split := NewSplit(pair.FromTransaction.ID, cat.ID, splitAmount)

		err = svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected error for transfer")
		}
		if _, ok := err.(*TransferCannotHaveSplitsError); !ok {
			t.Errorf("Expected TransferCannotHaveSplitsError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Search Tests
// =============================================================================

// createTestTransactionServiceWithCategories creates a test service with access
// to category and payee repos for search tests.
func createTestTransactionServiceWithCategories(t *testing.T) (
	*Service,
	*account.Repository,
	*category.Repository,
	*payee.Repository,
) {
	t.Helper()
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	transferRepo := NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	return svc, accountRepo, categoryRepo, payeeRepo
}

func TestTransactionService_ListByDateRange(t *testing.T) {
	t.Run("returns transactions within date range", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		date1, _ := types.ParseDate("2024-01-15")
		date2, _ := types.ParseDate("2024-02-15")
		date3, _ := types.ParseDate("2024-03-15")

		if err := svc.Create(NewTransaction(acct.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(acct.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(acct.ID, date3, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := types.ParseDate("2024-01-01")
		endDate, _ := types.ParseDate("2024-02-28")

		txns, err := svc.ListByDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("ListByDateRange() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions in range, got %d", len(txns))
		}
	})

	t.Run("returns empty for no matches", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		date1, _ := types.ParseDate("2024-06-15")
		if err := svc.Create(NewTransaction(acct.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := types.ParseDate("2024-01-01")
		endDate, _ := types.ParseDate("2024-02-28")

		txns, err := svc.ListByDateRange(startDate, endDate)
		if err != nil {
			t.Fatalf("ListByDateRange() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions, got %d", len(txns))
		}
	})
}

func TestTransactionService_ListByAccountAndDateRange(t *testing.T) {
	t.Run("filters by both account and date range", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		account1 := createTestAccount(t, accountRepo, "Checking")
		account2 := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("-50.00")
		date1, _ := types.ParseDate("2024-01-15")
		date2, _ := types.ParseDate("2024-02-15")
		date3, _ := types.ParseDate("2024-03-15")

		// Account 1: Jan, Feb, Mar
		if err := svc.Create(NewTransaction(account1.ID, date1, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(account1.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(account1.ID, date3, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Account 2: Feb
		if err := svc.Create(NewTransaction(account2.ID, date2, amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		startDate, _ := types.ParseDate("2024-01-01")
		endDate, _ := types.ParseDate("2024-02-28")

		txns, err := svc.ListByAccountAndDateRange(account1.ID, startDate, endDate)
		if err != nil {
			t.Fatalf("ListByAccountAndDateRange() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions for account1 in range, got %d", len(txns))
		}

		txns2, err := svc.ListByAccountAndDateRange(account2.ID, startDate, endDate)
		if err != nil {
			t.Fatalf("ListByAccountAndDateRange() error = %v", err)
		}
		if len(txns2) != 1 {
			t.Errorf("Expected 1 transaction for account2 in range, got %d", len(txns2))
		}
	})
}

func TestTransactionService_SearchByPayee(t *testing.T) {
	t.Run("finds transactions by payee name", func(t *testing.T) {
		svc, accountRepo, _, payeeRepo := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		payee1 := payee.NewPayee("Kroger Grocery")
		if err := payeeRepo.Create(payee1); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}
		payee2 := payee.NewPayee("Target")
		if err := payeeRepo.Create(payee2); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := types.NewMoney("-50.00")

		txn1 := NewTransactionWithPayee(acct.ID, types.Today(), amount, payee1.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		txn2 := NewTransactionWithPayee(acct.ID, types.Today(), amount, payee2.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Search for "Kroger" (partial, case-insensitive)
		txns, err := svc.SearchByPayee("kroger")
		if err != nil {
			t.Fatalf("SearchByPayee() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'kroger', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching payee", func(t *testing.T) {
		svc, accountRepo, _, payeeRepo := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		py := payee.NewPayee("Kroger")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransactionWithPayee(acct.ID, types.Today(), amount, py.ID)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByPayee("walmart")
		if err != nil {
			t.Fatalf("SearchByPayee() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'walmart', got %d", len(txns))
		}
	})
}

func TestTransactionService_SearchByMemo(t *testing.T) {
	t.Run("finds transactions by memo", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")

		txn1 := NewTransaction(acct.ID, types.Today(), amount)
		txn1.SetMemo("Grocery shopping at store")
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := NewTransaction(acct.ID, types.Today(), amount)
		txn2.SetMemo("Gas station fill up")
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByMemo("grocery")
		if err != nil {
			t.Fatalf("SearchByMemo() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'grocery', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching memo", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		txn.SetMemo("Grocery shopping")
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByMemo("electric")
		if err != nil {
			t.Fatalf("SearchByMemo() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'electric', got %d", len(txns))
		}
	})
}

func TestTransactionService_SearchByCategory(t *testing.T) {
	t.Run("finds transactions by category name", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Groceries", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		cat2 := category.NewCategory("Entertainment", category.TypeExpense)
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-50.00")

		txn1 := NewTransaction(acct.ID, types.Today(), amount)
		txn1.SetCategory(cat1.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := NewTransaction(acct.ID, types.Today(), amount)
		txn2.SetCategory(cat2.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByCategory("groceries")
		if err != nil {
			t.Fatalf("SearchByCategory() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching 'groceries', got %d", len(txns))
		}
	})

	t.Run("returns empty for no matching category", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Groceries", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		txn.SetCategory(cat.ID)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.SearchByCategory("utilities")
		if err != nil {
			t.Fatalf("SearchByCategory() error = %v", err)
		}
		if len(txns) != 0 {
			t.Errorf("Expected 0 transactions for 'utilities', got %d", len(txns))
		}
	})
}

func TestTransactionService_Search(t *testing.T) {
	t.Run("searches with combined criteria", func(t *testing.T) {
		svc, accountRepo, categoryRepo, payeeRepo := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Groceries", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		py := payee.NewPayee("Kroger")
		if err := payeeRepo.Create(py); err != nil {
			t.Fatalf("Failed to create payee: %v", err)
		}

		amount, _ := types.NewMoney("-50.00")
		date1, _ := types.ParseDate("2024-01-15")
		date2, _ := types.ParseDate("2024-03-15")

		// Txn 1: Kroger, Groceries, Jan
		txn1 := NewTransactionWithPayee(acct.ID, date1, amount, py.ID)
		txn1.SetCategory(cat.ID)
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Txn 2: Kroger, Groceries, Mar
		txn2 := NewTransactionWithPayee(acct.ID, date2, amount, py.ID)
		txn2.SetCategory(cat.ID)
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Search with date range that only includes Jan
		startDate, _ := types.ParseDate("2024-01-01")
		endDate, _ := types.ParseDate("2024-02-01")
		criteria := SearchCriteria{
			PayeeName: "kroger",
			StartDate: &startDate,
			EndDate:   &endDate,
		}

		txns, err := svc.Search(criteria)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching combined criteria, got %d", len(txns))
		}
	})

	t.Run("searches with memo criteria", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")

		txn1 := NewTransaction(acct.ID, types.Today(), amount)
		txn1.SetMemo("Weekly groceries")
		if err := svc.Create(txn1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txn2 := NewTransaction(acct.ID, types.Today(), amount)
		txn2.SetMemo("Gas station")
		if err := svc.Create(txn2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		criteria := SearchCriteria{
			Memo: "groceries",
		}

		txns, err := svc.Search(criteria)
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 1 {
			t.Errorf("Expected 1 transaction matching memo, got %d", len(txns))
		}
	})

	t.Run("empty criteria returns all", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		if err := svc.Create(NewTransaction(acct.ID, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.Create(NewTransaction(acct.ID, types.Today(), amount)); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		txns, err := svc.Search(SearchCriteria{})
		if err != nil {
			t.Fatalf("Search() error = %v", err)
		}
		if len(txns) != 2 {
			t.Errorf("Expected 2 transactions with empty criteria, got %d", len(txns))
		}
	})
}

// =============================================================================
// Split Update Tests
// =============================================================================

func TestTransactionService_AddSplit(t *testing.T) {
	t.Run("adds split to existing transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Add a split that matches the total
		splitAmount, _ := types.NewMoney("-100.00")
		split := NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err != nil {
			t.Fatalf("AddSplit() error = %v", err)
		}

		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 1 {
			t.Errorf("Expected 1 split, got %d", len(splits))
		}
	})

	t.Run("returns mismatch error when split total does not match", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Add a split with partial amount (doesn't match total)
		splitAmount, _ := types.NewMoney("-30.00")
		split := NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected mismatch error")
		}
		if _, ok := err.(*SplitTotalMismatchError); !ok {
			t.Errorf("Expected SplitTotalMismatchError, got %T: %v", err, err)
		}

		// Split should still be created despite mismatch
		splits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(splits) != 1 {
			t.Errorf("Expected 1 split (still created), got %d", len(splits))
		}
	})
}

func TestTransactionService_UpdateSplit(t *testing.T) {
	t.Run("updates split amount", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		split1Amount, _ := types.NewMoney("-60.00")
		split2Amount, _ := types.NewMoney("-40.00")
		splits := []*Split{
			NewSplit(txn.ID, cat1.ID, split1Amount),
			NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Update first split
		retrievedSplits, _ := svc.GetSplits(txn.ID)
		newAmount, _ := types.NewMoney("-80.00")
		retrievedSplits[0].Amount = newAmount

		err := svc.UpdateSplit(retrievedSplits[0])
		if err != nil {
			t.Fatalf("UpdateSplit() error = %v", err)
		}

		// Verify the update
		updatedSplits, _ := svc.GetSplits(txn.ID)
		found := false
		for _, s := range updatedSplits {
			if s.ID == retrievedSplits[0].ID {
				if !s.Amount.Equal(newAmount) {
					t.Errorf("Expected updated amount %s, got %s", newAmount.String(), s.Amount.String())
				}
				found = true
			}
		}
		if !found {
			t.Error("Updated split not found")
		}
	})

	t.Run("validates split before updating", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		splits := []*Split{
			NewSplit(txn.ID, cat.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		// Set invalid zero amount
		retrievedSplits[0].Amount = types.ZeroMoney

		err := svc.UpdateSplit(retrievedSplits[0])
		if err == nil {
			t.Error("UpdateSplit() expected error for zero amount")
		}
	})
}

func TestTransactionService_DeleteSplit(t *testing.T) {
	t.Run("deletes a split", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		split1Amount, _ := types.NewMoney("-60.00")
		split2Amount, _ := types.NewMoney("-40.00")
		splits := []*Split{
			NewSplit(txn.ID, cat1.ID, split1Amount),
			NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) != 2 {
			t.Fatalf("Expected 2 splits, got %d", len(retrievedSplits))
		}

		// Delete the first split
		if err := svc.DeleteSplit(retrievedSplits[0].ID); err != nil {
			t.Fatalf("DeleteSplit() error = %v", err)
		}

		remaining, _ := svc.GetSplits(txn.ID)
		if len(remaining) != 1 {
			t.Errorf("Expected 1 remaining split, got %d", len(remaining))
		}
	})
}

func TestTransactionService_ReplaceSplits(t *testing.T) {
	t.Run("replaces all splits", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		cat3 := category.NewCategory("Entertainment", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat3); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		origSplits := []*Split{
			NewSplit(txn.ID, cat1.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Replace with two new splits
		newSplit1, _ := types.NewMoney("-70.00")
		newSplit2, _ := types.NewMoney("-30.00")
		newSplits := []*Split{
			NewSplit(txn.ID, cat2.ID, newSplit1),
			NewSplit(txn.ID, cat3.ID, newSplit2),
		}

		if err := svc.ReplaceSplits(txn.ID, newSplits); err != nil {
			t.Fatalf("ReplaceSplits() error = %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) != 2 {
			t.Errorf("Expected 2 splits after replace, got %d", len(retrievedSplits))
		}
	})

	t.Run("rejects replacement splits that do not sum to transaction amount", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		splitAmount, _ := types.NewMoney("-100.00")
		origSplits := []*Split{
			NewSplit(txn.ID, cat1.ID, splitAmount),
		}

		if err := svc.CreateWithSplits(txn, origSplits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Try to replace with splits that don't sum correctly
		badAmount, _ := types.NewMoney("-50.00")
		badSplits := []*Split{
			NewSplit(txn.ID, cat2.ID, badAmount),
		}

		err := svc.ReplaceSplits(txn.ID, badSplits)
		if err == nil {
			t.Error("ReplaceSplits() expected error for mismatched totals")
		}
	})
}

// =============================================================================
// Transfer Update Tests
// =============================================================================

func TestTransactionService_UpdateTransferAmount(t *testing.T) {
	t.Run("updates amount on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		newAmount, _ := types.NewMoney("750.00")
		if err := svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, newAmount); err != nil {
			t.Fatalf("UpdateTransferAmount() error = %v", err)
		}

		// Verify both sides updated
		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if !updatedPair.FromTransaction.Amount.Neg().Equal(newAmount) {
			t.Errorf("From amount should be -%s, got %s", newAmount.String(), updatedPair.FromTransaction.Amount.String())
		}
		if !updatedPair.ToTransaction.Amount.Equal(newAmount) {
			t.Errorf("To amount should be %s, got %s", newAmount.String(), updatedPair.ToTransaction.Amount.String())
		}
	})

	t.Run("rejects negative amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		negAmount, _ := types.NewMoney("-100.00")
		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, negAmount)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for negative amount")
		}
		if _, ok := err.(*InvalidTransferAmountError); !ok {
			t.Errorf("Expected InvalidTransferAmountError, got %T: %v", err, err)
		}
	})

	t.Run("rejects zero amount", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, types.ZeroMoney)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for zero amount")
		}
	})
}

func TestTransactionService_UpdateTransferDate(t *testing.T) {
	t.Run("updates date on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		newDate, _ := types.ParseDate("2024-06-15")
		if err := svc.UpdateTransferDate(pair.FromTransaction.TransferID.ID, newDate); err != nil {
			t.Fatalf("UpdateTransferDate() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Date != newDate {
			t.Errorf("From date should be %s, got %s", newDate, updatedPair.FromTransaction.Date)
		}
		if updatedPair.ToTransaction.Date != newDate {
			t.Errorf("To date should be %s, got %s", newDate, updatedPair.ToTransaction.Date)
		}
	})
}

func TestTransactionService_UpdateTransferStatus(t *testing.T) {
	t.Run("updates status on both sides", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusCleared); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Status != StatusCleared {
			t.Errorf("From status should be cleared, got %s", updatedPair.FromTransaction.Status)
		}
		if updatedPair.ToTransaction.Status != StatusCleared {
			t.Errorf("To status should be cleared, got %s", updatedPair.ToTransaction.Status)
		}
	})

	t.Run("updates to reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if err != nil {
			t.Fatalf("GetTransferPair() error = %v", err)
		}

		if updatedPair.FromTransaction.Status != StatusReconciled {
			t.Errorf("From status should be reconciled, got %s", updatedPair.FromTransaction.Status)
		}
		if updatedPair.ToTransaction.Status != StatusReconciled {
			t.Errorf("To status should be reconciled, got %s", updatedPair.ToTransaction.Status)
		}
	})
}

// =============================================================================
// Void Transaction Tests
// =============================================================================

func TestTransactionService_VoidTransaction(t *testing.T) {
	t.Run("voids a regular transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		txn.SetMemo("Original memo")
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		retrieved, err := svc.GetByID(txn.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.Status != StatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
		if !retrieved.Amount.IsZero() {
			t.Errorf("Expected amount 0, got %s", retrieved.Amount.String())
		}
		if !retrieved.Memo.Valid || retrieved.Memo.String != "**VOID**" {
			t.Errorf("Expected memo '**VOID**', got %q", retrieved.Memo.String)
		}
	})

	t.Run("voids a cleared transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-75.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
	})

	t.Run("rejects voiding a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Voiding again should fail
		err := svc.VoidTransaction(txn.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for already void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects voiding a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.VoidTransaction(txn.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("voids a split transaction and removes splits", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)

		split1Amount, _ := types.NewMoney("-60.00")
		split2Amount, _ := types.NewMoney("-40.00")
		splits := []*Split{
			NewSplit(txn.ID, cat1.ID, split1Amount),
			NewSplit(txn.ID, cat2.ID, split2Amount),
		}

		if err := svc.CreateWithSplits(txn, splits); err != nil {
			t.Fatalf("CreateWithSplits() error = %v", err)
		}

		// Void the split transaction
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Verify transaction is voided
		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusVoid {
			t.Errorf("Expected status void, got %s", retrieved.Status)
		}
		if !retrieved.Amount.IsZero() {
			t.Errorf("Expected amount 0, got %s", retrieved.Amount.String())
		}

		// Verify splits are removed
		remainingSplits, err := svc.GetSplits(txn.ID)
		if err != nil {
			t.Fatalf("GetSplits() error = %v", err)
		}
		if len(remainingSplits) != 0 {
			t.Errorf("Expected 0 splits after void, got %d", len(remainingSplits))
		}
	})

	t.Run("voids a transfer (both sides)", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Void via the from-side
		if err := svc.VoidTransaction(pair.FromTransaction.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Both sides should be void
		fromTxn, err := svc.GetByID(pair.FromTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if fromTxn.Status != StatusVoid {
			t.Errorf("From transaction should be void, got %s", fromTxn.Status)
		}
		if !fromTxn.Amount.IsZero() {
			t.Errorf("From amount should be 0, got %s", fromTxn.Amount.String())
		}
		if !fromTxn.Memo.Valid || fromTxn.Memo.String != "**VOID**" {
			t.Errorf("From memo should be '**VOID**', got %q", fromTxn.Memo.String)
		}

		toTxn, err := svc.GetByID(pair.ToTransaction.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if toTxn.Status != StatusVoid {
			t.Errorf("To transaction should be void, got %s", toTxn.Status)
		}
		if !toTxn.Amount.IsZero() {
			t.Errorf("To amount should be 0, got %s", toTxn.Amount.String())
		}
		if !toTxn.Memo.Valid || toTxn.Memo.String != "**VOID**" {
			t.Errorf("To memo should be '**VOID**', got %q", toTxn.Memo.String)
		}
	})

	t.Run("voids transfer via to-side", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("300.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Void via the to-side
		if err := svc.VoidTransaction(pair.ToTransaction.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Both sides should be void
		fromTxn, _ := svc.GetByID(pair.FromTransaction.ID)
		if fromTxn.Status != StatusVoid {
			t.Errorf("From transaction should be void, got %s", fromTxn.Status)
		}

		toTxn, _ := svc.GetByID(pair.ToTransaction.ID)
		if toTxn.Status != StatusVoid {
			t.Errorf("To transaction should be void, got %s", toTxn.Status)
		}
	})

	t.Run("rejects voiding transfer when one side is reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// Reconcile both sides (transfer status updates both)
		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		// Voiding should fail
		err = svc.VoidTransaction(pair.FromTransaction.ID)
		if err == nil {
			t.Error("VoidTransaction() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_Update_VoidGuard(t *testing.T) {
	t.Run("rejects editing a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		// Try to update the void transaction
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := types.NewMoney("-100.00")
		retrieved.Amount = newAmount
		retrieved.Status = StatusUncleared

		err := svc.Update(retrieved)
		if err == nil {
			t.Error("Update() expected error for void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects editing a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Try to update the reconciled transaction
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := types.NewMoney("-100.00")
		retrieved.Amount = newAmount

		err := svc.Update(retrieved)
		if err == nil {
			t.Error("Update() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// Reconciled Transaction Locking Tests
// =============================================================================

func TestTransactionService_Delete_ReconciledGuard(t *testing.T) {
	t.Run("rejects deleting a reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.Delete(txn.ID)
		if err == nil {
			t.Error("Delete() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects deleting a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.Delete(txn.ID)
		if err == nil {
			t.Error("Delete() expected error for void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects deleting a reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		err = svc.Delete(pair.FromTransaction.ID)
		if err == nil {
			t.Error("Delete() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_UnReconcileTransaction(t *testing.T) {
	t.Run("un-reconciles a reconciled transaction to cleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		if err := svc.UnReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("UnReconcileTransaction() error = %v", err)
		}

		retrieved, _ := svc.GetByID(txn.ID)
		if retrieved.Status != StatusCleared {
			t.Errorf("Expected status cleared after un-reconcile, got %s", retrieved.Status)
		}
	})

	t.Run("rejects un-reconciling a non-reconciled transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for uncleared transaction")
		}
		if _, ok := err.(*NotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects un-reconciling a cleared transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.ClearTransaction(txn.ID); err != nil {
			t.Fatalf("ClearTransaction() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for cleared transaction")
		}
		if _, ok := err.(*NotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects un-reconciling a void transaction", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.UnReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("UnReconcileTransaction() expected error for void transaction")
		}
		if _, ok := err.(*NotReconciledError); !ok {
			t.Errorf("Expected TransactionNotReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("allows editing after un-reconcile", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Reconcile then un-reconcile
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}
		if err := svc.UnReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("UnReconcileTransaction() error = %v", err)
		}

		// Should now be editable
		retrieved, _ := svc.GetByID(txn.ID)
		newAmount, _ := types.NewMoney("-100.00")
		retrieved.Amount = newAmount
		if err := svc.Update(retrieved); err != nil {
			t.Fatalf("Update() should succeed after un-reconcile, got error: %v", err)
		}

		final, _ := svc.GetByID(txn.ID)
		if !final.Amount.Equal(newAmount) {
			t.Errorf("Expected amount %s after edit, got %s", newAmount.String(), final.Amount.String())
		}
	})
}

func TestTransactionService_StatusOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects ClearTransaction on reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.ClearTransaction(txn.ID)
		if err == nil {
			t.Error("ClearTransaction() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects MarkTransactionUncleared on reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		err := svc.MarkTransactionUncleared(txn.ID)
		if err == nil {
			t.Error("MarkTransactionUncleared() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ClearTransaction on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.ClearTransaction(txn.ID)
		if err == nil {
			t.Error("ClearTransaction() expected error for void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects MarkTransactionUncleared on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.MarkTransactionUncleared(txn.ID)
		if err == nil {
			t.Error("MarkTransactionUncleared() expected error for void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ReconcileTransaction on void", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.VoidTransaction(txn.ID); err != nil {
			t.Fatalf("VoidTransaction() error = %v", err)
		}

		err := svc.ReconcileTransaction(txn.ID)
		if err == nil {
			t.Error("ReconcileTransaction() expected error for void transaction")
		}
		if _, ok := err.(*IsVoidError); !ok {
			t.Errorf("Expected TransactionIsVoidError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_SplitOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects AddSplit on reconciled transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		splitAmount, _ := types.NewMoney("-100.00")
		split := NewSplit(txn.ID, cat.ID, splitAmount)

		err := svc.AddSplit(split)
		if err == nil {
			t.Error("AddSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateSplit on reconciled transaction", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it first (no splits yet)
		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Insert a split directly via SQL to create a reconciled transaction with splits
		splitID := types.NewID()
		if _, err := database.Conn().Exec(
			`INSERT INTO transaction_splits (id, transaction_id, category_id, amount, created_at)
			 VALUES (?, ?, ?, -100.0000, CURRENT_TIMESTAMP)`,
			splitID.String(), txn.ID.String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("Failed to insert split: %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) == 0 {
			t.Fatal("Expected splits to exist")
		}
		newAmount, _ := types.NewMoney("-80.00")
		retrievedSplits[0].Amount = newAmount

		err := svc.UpdateSplit(retrievedSplits[0])
		if err == nil {
			t.Error("UpdateSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects DeleteSplit on reconciled transaction", func(t *testing.T) {
		database := createTestDB(t)
		txnRepo := NewRepository(database)
		splitRepo := NewSplitRepository(database)
		transferRepo := NewTransferRepository(database, txnRepo)
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)

		acct := createTestAccount(t, accountRepo, "Checking")

		cat := category.NewCategory("Food", category.TypeExpense)
		if err := categoryRepo.Create(cat); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it first (no splits yet)
		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		// Insert a split directly via SQL
		splitID := types.NewID()
		if _, err := database.Conn().Exec(
			`INSERT INTO transaction_splits (id, transaction_id, category_id, amount, created_at)
			 VALUES (?, ?, ?, -100.0000, CURRENT_TIMESTAMP)`,
			splitID.String(), txn.ID.String(), cat.ID.String(),
		); err != nil {
			t.Fatalf("Failed to insert split: %v", err)
		}

		retrievedSplits, _ := svc.GetSplits(txn.ID)
		if len(retrievedSplits) == 0 {
			t.Fatal("Expected splits to exist")
		}

		err := svc.DeleteSplit(retrievedSplits[0].ID)
		if err == nil {
			t.Error("DeleteSplit() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects ReplaceSplits on reconciled transaction", func(t *testing.T) {
		svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		cat1 := category.NewCategory("Food", category.TypeExpense)
		cat2 := category.NewCategory("Household", category.TypeExpense)
		if err := categoryRepo.Create(cat1); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}
		if err := categoryRepo.Create(cat2); err != nil {
			t.Fatalf("Failed to create category: %v", err)
		}

		// Create transaction and reconcile it (no splits needed for ReplaceSplits guard test)
		amount, _ := types.NewMoney("-100.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.ReconcileTransaction(txn.ID); err != nil {
			t.Fatalf("ReconcileTransaction() error = %v", err)
		}

		splitAmount, _ := types.NewMoney("-100.00")
		newSplits := []*Split{
			NewSplit(txn.ID, cat2.ID, splitAmount),
		}

		err := svc.ReplaceSplits(txn.ID, newSplits)
		if err == nil {
			t.Error("ReplaceSplits() expected error for reconciled transaction")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})
}

func TestTransactionService_TransferOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects UpdateTransfer on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newAmount, _ := types.NewMoney("750.00")
		newDate, _ := types.ParseDate("2024-06-15")
		err = svc.UpdateTransfer(pair.FromTransaction.TransferID.ID, newDate, newAmount, "Updated", StatusCleared, types.NullableID{})
		if err == nil {
			t.Error("UpdateTransfer() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateTransferAmount on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newAmount, _ := types.NewMoney("750.00")
		err = svc.UpdateTransferAmount(pair.FromTransaction.TransferID.ID, newAmount)
		if err == nil {
			t.Error("UpdateTransferAmount() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects UpdateTransferDate on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		newDate, _ := types.ParseDate("2024-06-15")
		err = svc.UpdateTransferDate(pair.FromTransaction.TransferID.ID, newDate)
		if err == nil {
			t.Error("UpdateTransferDate() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("rejects DeleteTransfer on reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		err = svc.DeleteTransfer(pair.FromTransaction.TransferID.ID)
		if err == nil {
			t.Error("DeleteTransfer() expected error for reconciled transfer")
		}
		if _, ok := err.(*IsReconciledError); !ok {
			t.Errorf("Expected TransactionIsReconciledError, got %T: %v", err, err)
		}
	})

	t.Run("allows UpdateTransferStatus on non-reconciled transfer", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		checking := createTestAccount(t, accountRepo, "Checking")
		savings := createTestAccount(t, accountRepo, "Savings")

		amount, _ := types.NewMoney("500.00")
		pair, err := svc.CreateTransfer(checking.ID, savings.ID, types.Today(), amount, "", types.NullableID{})
		if err != nil {
			t.Fatalf("CreateTransfer() error = %v", err)
		}

		// UpdateTransferStatus should still work (it's how reconciliation happens)
		if err := svc.UpdateTransferStatus(pair.FromTransaction.TransferID.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateTransferStatus() error = %v", err)
		}

		updatedPair, _ := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
		if updatedPair.FromTransaction.Status != StatusReconciled {
			t.Errorf("Expected reconciled status, got %s", updatedPair.FromTransaction.Status)
		}
	})
}

func TestCreateTransfer_RejectsInvestmentSource(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)
	checking := createTestAccountOfType(t, accountRepo, "Checking", account.TypeChecking)

	amount, _ := types.NewMoney("500.00")
	_, err := svc.CreateTransfer(ira.ID, checking.ID, types.Today(), amount, "", types.NullableID{})
	if err == nil {
		t.Fatal("CreateTransfer() expected error when source is an investment account")
	}
	if _, ok := err.(*NotRegularAccountError); !ok {
		t.Errorf("Expected *NotRegularAccountError, got %T: %v", err, err)
	}
}

func TestCreateTransfer_RejectsInvestmentDest(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	checking := createTestAccountOfType(t, accountRepo, "Checking", account.TypeChecking)
	hsa := createTestAccountOfType(t, accountRepo, "HSA", account.TypeHSA)

	amount, _ := types.NewMoney("500.00")
	_, err := svc.CreateTransfer(checking.ID, hsa.ID, types.Today(), amount, "", types.NullableID{})
	if err == nil {
		t.Fatal("CreateTransfer() expected error when destination is an investment account")
	}
	if _, ok := err.(*NotRegularAccountError); !ok {
		t.Errorf("Expected *NotRegularAccountError, got %T: %v", err, err)
	}
}

func TestUpdateTransfer_RejectsInvestmentAccounts(t *testing.T) {
	// To exercise the UpdateTransfer guard, we plant a transfer pair whose
	// "from" account is an investment account via the repository (bypassing
	// the now-hardened CreateTransfer), then attempt UpdateTransfer.
	svc, accountRepo := createTestTransactionService(t)
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)
	checking := createTestAccountOfType(t, accountRepo, "Checking", account.TypeChecking)

	amount, _ := types.NewMoney("500.00")
	pair := NewTransferPair(ira.ID, checking.ID, types.Today(), amount)
	if err := svc.transferRepo.Create(pair); err != nil {
		t.Fatalf("seed transfer pair via repo: %v", err)
	}

	newAmount, _ := types.NewMoney("750.00")
	err := svc.UpdateTransfer(pair.FromTransaction.TransferID.ID, types.Today(), newAmount, "memo", StatusUncleared, types.NullableID{})
	if err == nil {
		t.Fatal("UpdateTransfer() expected error when a leg is an investment account")
	}
	if _, ok := err.(*NotRegularAccountError); !ok {
		t.Errorf("Expected *NotRegularAccountError, got %T: %v", err, err)
	}
}
