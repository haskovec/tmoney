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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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

func TestTransactionService_ListByAccount(t *testing.T) {
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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)
	return svc, accountRepo, categoryRepo, payeeRepo
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

// =============================================================================
// Void Transaction Tests
// =============================================================================

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

		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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

	// The edit guard reads the row's current status; it does not remember that the
	// row was ever reconciled. reconciliation.Service unlocks a row by writing the
	// prior status back through Repository.UpdateStatus (reconciliation_service.go:391),
	// so this walks the same two writes and asserts the guard follows the column.
	t.Run("allows editing once a reconciled transaction returns to cleared", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
		}
		locked, _ := svc.GetByID(txn.ID)
		newAmount, _ := types.NewMoney("-100.00")
		locked.Amount = newAmount
		if _, ok := svc.Update(locked).(*IsReconciledError); !ok {
			t.Fatal("Update() should be refused while the transaction is reconciled")
		}

		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusCleared); err != nil {
			t.Fatalf("UpdateStatus(cleared) error = %v", err)
		}
		retrieved, _ := svc.GetByID(txn.ID)
		retrieved.Amount = newAmount
		if err := svc.Update(retrieved); err != nil {
			t.Fatalf("Update() should succeed once cleared, got error: %v", err)
		}

		final, _ := svc.GetByID(txn.ID)
		if !final.Amount.Equal(newAmount) {
			t.Errorf("Expected amount %s after edit, got %s", newAmount.String(), final.Amount.String())
		}
	})
}

// =============================================================================
// Reconciled Transaction Locking Tests
// =============================================================================

func TestTransactionService_StatusOperations_ReconciledGuard(t *testing.T) {
	t.Run("rejects ClearTransaction on reconciled", func(t *testing.T) {
		svc, accountRepo := createTestTransactionService(t)
		acct := createTestAccount(t, accountRepo, "Checking")

		amount, _ := types.NewMoney("-50.00")
		txn := NewTransaction(acct.ID, types.Today(), amount)
		if err := svc.Create(txn); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
		payeeRepo := payee.NewRepository(database)
		accountRepo := account.NewRepository(database)
		categoryRepo := category.NewRepository(database)

		svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, nil, database)

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
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
		if err := svc.txnRepo.UpdateStatus(txn.ID, StatusReconciled); err != nil {
			t.Fatalf("UpdateStatus(reconciled) error = %v", err)
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
