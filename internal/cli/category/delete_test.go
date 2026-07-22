package category_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestCategoryDelete_ByName(t *testing.T) {
	database, path := newCatFile(t)
	id := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Groceries"}, stdout, stderr); err != nil {
		t.Fatalf("category delete: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	if _, err := repo.GetByID(id); err == nil {
		t.Error("category should be gone after delete")
	}
}

func TestCategoryDelete_ByID(t *testing.T) {
	database, path := newCatFile(t)
	id := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "delete", "--file", path, id.String()}, stdout, stderr); err != nil {
		t.Fatalf("category delete <id>: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	if _, err := repo.GetByID(id); err == nil {
		t.Error("category should be gone after delete by id")
	}
}

func TestCategoryDelete_ParentChildForm(t *testing.T) {
	database, path := newCatFile(t)
	food := seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	childID := seedChild(t, database, "Snacks", food, categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Food:Snacks"}, stdout, stderr); err != nil {
		t.Fatalf("category delete Parent:Child: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()
	if _, err := repo.GetByID(childID); err == nil {
		t.Error("child should be gone after Parent:Child delete")
	}
	// Parent survives.
	if _, err := repo.GetByID(food); err != nil {
		t.Errorf("parent should survive: %v", err)
	}
}

func TestCategoryDelete_Ambiguous(t *testing.T) {
	database, path := newCatFile(t)
	// Two children named "Snacks" under different parents, no top-level Snacks.
	food := seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	fun := seedTopLevel(t, database, "Fun", categorydom.TypeExpense)
	seedChild(t, database, "Snacks", food, categorydom.TypeExpense)
	seedChild(t, database, "Snacks", fun, categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Snacks"}, stdout, stderr)
	if err == nil {
		t.Fatal("ambiguous bare name should error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguity error, got: %v", err)
	}
}

func TestCategoryDelete_SystemRefused(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Value Adjustment"}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting a system category should be refused")
	}
	if !strings.Contains(err.Error(), "system category") {
		t.Errorf("expected system-category refusal, got: %v", err)
	}
}

func TestCategoryDelete_HasSubcategoriesRefused(t *testing.T) {
	database, path := newCatFile(t)
	food := seedTopLevel(t, database, "Food", categorydom.TypeExpense)
	seedChild(t, database, "Snacks", food, categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Food"}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting a category with subcategories should be refused")
	}
	if !strings.Contains(err.Error(), "subcategories") {
		t.Errorf("expected subcategories refusal, got: %v", err)
	}
	// Subcategories refusal must NOT carry the merge hint.
	if strings.Contains(err.Error(), "category merge") {
		t.Errorf("subcategories refusal should not suggest merge, got: %v", err)
	}
}

func TestCategoryDelete_HasTransactionsRefusedWithMergeHint(t *testing.T) {
	database, path := newCatFile(t)
	catID := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)

	acctRepo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-12.00"))
	txn.SetCategory(catID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting a category with transactions should be refused")
	}
	if !strings.Contains(err.Error(), "transactions") {
		t.Errorf("expected transactions refusal, got: %v", err)
	}
	if !strings.Contains(err.Error(), "category merge") {
		t.Errorf("transactions refusal should suggest `category merge`, got: %v", err)
	}
}

func TestCategoryDelete_HasSplitLinesRefusedWithMergeHint(t *testing.T) {
	database, path := newCatFile(t)
	catID := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	_, err := database.Conn().Exec(`
		INSERT INTO transaction_splits (id, transaction_id, category_id, amount)
		VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?)
	`, types.NewID().String(), types.NewID().String(), catID.String(), "-10.00")
	if err != nil {
		t.Fatalf("insert split: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	delErr := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Groceries"}, stdout, stderr)
	if delErr == nil {
		t.Fatal("deleting a category with split lines should be refused")
	}
	if !strings.Contains(delErr.Error(), "split lines") {
		t.Errorf("expected split-lines refusal, got: %v", delErr)
	}
	if !strings.Contains(delErr.Error(), "category merge") {
		t.Errorf("split-lines refusal should suggest `category merge`, got: %v", delErr)
	}
}

func TestCategoryDelete_HasScheduledRefusedWithMergeHint(t *testing.T) {
	database, path := newCatFile(t)
	catID := seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)

	acctRepo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	_, err := database.Conn().Exec(`
		INSERT INTO scheduled_transactions
			(id, account_id, category_id, amount, frequency, start_date, next_date)
		VALUES (CAST(? AS UUID), CAST(? AS UUID), CAST(? AS UUID), ?, 'monthly', ?, ?)
	`, types.NewID().String(), acct.ID.String(), catID.String(), "-25.00",
		types.Today().String(), types.Today().String())
	if err != nil {
		t.Fatalf("insert scheduled: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	delErr := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Groceries"}, stdout, stderr)
	if delErr == nil {
		t.Fatal("deleting a category with scheduled transactions should be refused")
	}
	if !strings.Contains(delErr.Error(), "scheduled transactions") {
		t.Errorf("expected scheduled refusal, got: %v", delErr)
	}
	if !strings.Contains(delErr.Error(), "category merge") {
		t.Errorf("scheduled refusal should suggest `category merge`, got: %v", delErr)
	}
}

func TestCategoryDelete_NotFound(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "delete", "--file", path, "Nope"}, stdout, stderr)
	if err == nil {
		t.Fatal("deleting a nonexistent category should error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}
