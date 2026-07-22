package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// splitTestEnv seeds a Checking + Savings account and Food/Household
// categories, then closes the DB so the CLI can open it fresh.
func splitTestEnv(t *testing.T) (dbPath string, checkingID types.ID, savingsID types.ID) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}

	catRepo := category.NewRepository(database)
	for _, name := range []string{"Food", "Household"} {
		if err := catRepo.Create(category.NewCategory(name, category.TypeExpense)); err != nil {
			t.Fatalf("setup: create category %s: %v", name, err)
		}
	}

	database.Close()
	return dbPath, checking.ID, savings.ID
}

// listSplits reopens the DB and returns the splits for a transaction.
func listSplits(t *testing.T, dbPath string, txnID types.ID) []*transactiondom.Split {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	splits, err := transactiondom.NewSplitRepository(database).ListByTransaction(txnID)
	if err != nil {
		t.Fatalf("list splits: %v", err)
	}
	return splits
}

// soleTransaction reopens the DB and returns the single transaction on an
// account (fails if there is not exactly one).
func soleTransaction(t *testing.T, dbPath string, acctID types.ID) *transactiondom.Transaction {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	txns, err := transactiondom.NewRepository(database).ListByAccount(acctID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction on account, got %d", len(txns))
	}
	return txns[0]
}

func TestTransactionAdd_Split_HappyPath(t *testing.T) {
	dbPath, checkingID, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-50.00",
		"--split", "Food=-40.00:dinner",
		"--split", "Household=-10.00:soap",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("split add: %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Split transaction created successfully") {
		t.Errorf("expected split confirmation, got: %s", out)
	}
	if !strings.Contains(out, "2 line(s)") {
		t.Errorf("expected line count in output, got: %s", out)
	}

	parent := soleTransaction(t, dbPath, checkingID)
	if parent.CategoryID.Valid {
		t.Error("split parent must not carry a category")
	}
	if parent.Amount.String() != "-50" {
		t.Errorf("parent amount = %s, want -50", parent.Amount.String())
	}

	splits := listSplits(t, dbPath, parent.ID)
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	byMemo := map[string]*transactiondom.Split{}
	for _, s := range splits {
		byMemo[s.Memo.String] = s
	}
	if s := byMemo["dinner"]; s == nil || s.Amount.String() != "-40" {
		t.Errorf("dinner split wrong: %+v", s)
	}
	if s := byMemo["soap"]; s == nil || s.Amount.String() != "-10" {
		t.Errorf("soap split wrong: %+v", s)
	}
}

func TestTransactionAdd_Split_DerivedTotal(t *testing.T) {
	dbPath, checkingID, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking",
		"--split", "Food=-30.00",
		"--split", "Household=-20.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("split add (derived): %v\nstderr=%s", err, stderr)
	}

	parent := soleTransaction(t, dbPath, checkingID)
	if parent.Amount.String() != "-50" {
		t.Errorf("derived parent amount = %s, want -50", parent.Amount.String())
	}
	if splits := listSplits(t, dbPath, parent.ID); len(splits) != 2 {
		t.Errorf("expected 2 splits, got %d", len(splits))
	}
}

func TestTransactionAdd_Split_SumMismatch(t *testing.T) {
	dbPath, checkingID, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-50.00",
		"--split", "Food=-30.00",
		"--split", "Household=-15.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected sum-mismatch error")
	}
	if !strings.Contains(err.Error(), "sum") {
		t.Errorf("expected sum error, got: %v", err)
	}

	database, oerr := db.Open(dbPath)
	if oerr != nil {
		t.Fatalf("post: db.Open: %v", oerr)
	}
	defer database.Close()
	txns, _ := transactiondom.NewRepository(database).ListByAccount(checkingID)
	if len(txns) != 0 {
		t.Errorf("expected nothing persisted on mismatch, got %d transactions", len(txns))
	}
}

func TestTransactionAdd_Split_TransferLine(t *testing.T) {
	dbPath, checkingID, savingsID := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-100.00",
		"--split", "Food=-60.00",
		"--split", "Transfer:Savings=-40.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("split add (transfer): %v\nstderr=%s", err, stderr)
	}

	parent := soleTransaction(t, dbPath, checkingID)
	splits := listSplits(t, dbPath, parent.ID)
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	var transferSplit *transactiondom.Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			transferSplit = s
		}
	}
	if transferSplit == nil {
		t.Fatal("expected one transfer-line split")
	}
	if transferSplit.TransferAccountID.ID != savingsID {
		t.Errorf("transfer target = %s, want savings", transferSplit.TransferAccountID.ID)
	}
	if !transferSplit.TransferID.Valid {
		t.Error("service should have minted a transfer_id")
	}

	// Counterpart lives in the savings account with the negated amount.
	counterpart := soleTransaction(t, dbPath, savingsID)
	if !counterpart.IsTransfer() {
		t.Error("counterpart should be a transfer")
	}
	if counterpart.Amount.String() != "40" {
		t.Errorf("counterpart amount = %s, want 40", counterpart.Amount.String())
	}
}

func TestTransactionAdd_Split_SelfTransferRefused(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-40.00",
		"--split", "Transfer:Checking=-40.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected self-transfer to be refused")
	}
	if !strings.Contains(err.Error(), "own account") {
		t.Errorf("expected own-account error, got: %v", err)
	}
}

func TestTransactionAdd_Split_UnknownCategoryRefused(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-40.00",
		"--split", "Nonexistent=-40.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected unknown-category error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestTransactionAdd_Split_UnknownTransferAccountRefused(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-40.00",
		"--split", "Transfer:Nowhere=-40.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected unknown-transfer-account error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestTransactionAdd_Split_CategoryFlagRefused(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--amount", "-40.00",
		"--category", "Food",
		"--split", "Food=-40.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected --category + --split to be refused")
	}
	if !strings.Contains(err.Error(), "--category") {
		t.Errorf("expected --category error, got: %v", err)
	}
}

func TestTransactionAdd_Split_Malformed(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	cases := []struct {
		name string
		spec string
	}{
		{"no equals", "Food-40.00"},
		{"bad amount", "Food=abc"},
		{"zero amount", "Food=0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			err := cli.ExecuteWith([]string{
				"transaction", "add", "--file", dbPath,
				"--account", "Checking",
				"--split", tc.spec,
			}, stdout, stderr)
			if err == nil {
				t.Fatalf("expected error for malformed split %q", tc.spec)
			}
		})
	}
}

func TestTransactionAdd_Split_ListAndSearchRendering(t *testing.T) {
	dbPath, _, _ := splitTestEnv(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add", "--file", dbPath,
		"--account", "Checking", "--payee", "Costco", "--amount", "-60.00",
		"--split", "Food=-20.00",
		"--split", "Household=-20.00",
		"--split", "Food=-20.00:snacks",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("split add: %v\nstderr=%s", err, stderr)
	}

	listOut, listErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "list", "--file", dbPath, "--account", "Checking"}, listOut, listErr); err != nil {
		t.Fatalf("transaction list: %v\nstderr=%s", err, listErr)
	}
	if !strings.Contains(listOut.String(), "[3 splits]") {
		t.Errorf("list should mark split parent, got:\n%s", listOut.String())
	}

	searchOut, searchErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "search", "Costco", "--file", dbPath}, searchOut, searchErr); err != nil {
		t.Fatalf("transaction search: %v\nstderr=%s", err, searchErr)
	}
	if !strings.Contains(searchOut.String(), "[3 splits]") {
		t.Errorf("search should mark split parent, got:\n%s", searchOut.String())
	}
}
