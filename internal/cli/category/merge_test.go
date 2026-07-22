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

func TestCategoryMerge_HappyPath(t *testing.T) {
	database, path := newCatFile(t)
	srcID := seedTopLevel(t, database, "Dining", categorydom.TypeExpense)
	tgtID := seedTopLevel(t, database, "Dining Out", categorydom.TypeExpense)
	// A child under the source that must be reassigned to the target.
	childID := seedChild(t, database, "Fast Food", srcID, categorydom.TypeExpense)

	// A transaction on the source that must be reassigned.
	acctRepo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("100.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-12.00"))
	txn.SetCategory(srcID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"category", "merge", "--file", path, "--from", "Dining", "--to", "Dining Out"}, stdout, stderr); err != nil {
		t.Fatalf("category merge: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Dining") || !strings.Contains(stdout.String(), "Dining Out") {
		t.Errorf("merge summary should name both categories, got:\n%s", stdout.String())
	}

	reDB, repo := reopenCat(t, path)
	defer reDB.Close()

	// Source is gone.
	if _, err := repo.GetByID(srcID); err == nil {
		t.Error("source category should be deleted after merge")
	}
	// Child now points at the target.
	child, err := repo.GetByID(childID)
	if err != nil {
		t.Fatalf("reload child: %v", err)
	}
	if !child.ParentID.Valid || child.ParentID.ID != tgtID {
		t.Errorf("child parent = %v, want target %s", child.ParentID, tgtID)
	}

	// Transaction now points at the target.
	txnRepo2 := transactiondom.NewRepository(reDB)
	got, err := txnRepo2.GetByID(txn.ID)
	if err != nil {
		t.Fatalf("reload transaction: %v", err)
	}
	if !got.CategoryID.Valid || got.CategoryID.ID != tgtID {
		t.Errorf("transaction category = %v, want target %s", got.CategoryID, tgtID)
	}
}

func TestCategoryMerge_TypeMismatch(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	seedTopLevel(t, database, "Salary", categorydom.TypeIncome)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "merge", "--file", path, "--from", "Groceries", "--to", "Salary"}, stdout, stderr)
	if err == nil {
		t.Fatal("merging across types should be refused")
	}
	if !strings.Contains(err.Error(), "merge") {
		t.Errorf("expected merge error, got: %v", err)
	}
}

func TestCategoryMerge_SameCategory(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "merge", "--file", path, "--from", "Groceries", "--to", "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("merging a category into itself should be refused")
	}
	if !strings.Contains(err.Error(), "itself") {
		t.Errorf("expected same-category error, got: %v", err)
	}
}

func TestCategoryMerge_SystemRefused(t *testing.T) {
	database, path := newCatFile(t)
	seedTopLevel(t, database, "Groceries", categorydom.TypeExpense)
	database.Close()
	// Value Adjustment is a system category ensured on open.

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "merge", "--file", path, "--from", "Value Adjustment", "--to", "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("merging a system category should be refused")
	}
	if !strings.Contains(err.Error(), "system category") {
		t.Errorf("expected system-category refusal, got: %v", err)
	}
}

func TestCategoryMerge_MissingFlags(t *testing.T) {
	database, path := newCatFile(t)
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"category", "merge", "--file", path, "--from", "Groceries"}, stdout, stderr)
	if err == nil {
		t.Fatal("merge without --to should error")
	}
}
