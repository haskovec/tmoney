package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// editFixture creates a database with a checking account (opening balance
// 1000.00) and one -50.00 transaction with payee "Coffee Shop", category
// "Food", and memo "latte". It returns the db path, the account ID, and
// the transaction ID.
func editFixture(t *testing.T) (dbPath string, acctID, txnID types.ID) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	defer database.Close()

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.MustParseDate("2026-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create test payee: %v", err)
	}

	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Food", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}

	txnRepo := transactiondom.NewRepository(database)
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	txn.SetPayee(py.ID)
	txn.SetCategory(cat.ID)
	txn.SetMemo("latte")
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}

	return dbPath, acct.ID, txn.ID
}

// reload fetches the transaction back out of a fresh database handle.
func reload(t *testing.T, dbPath string, txnID types.ID) (*transactiondom.Transaction, *app.Services, func()) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	svc := app.NewServices(database)
	txn, err := svc.TransactionRepo.GetByID(txnID)
	if err != nil {
		database.Close()
		t.Fatalf("failed to reload transaction: %v", err)
	}
	return txn, svc, func() { database.Close() }
}

func runEdit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"transaction", "edit"}, args...), stdout, stderr)
	return stdout.String(), err
}

func TestTransactionEdit_MissingFile(t *testing.T) {
	_, err := runEdit(t, "--txn-id", "abc", "--amount", "-1.00")
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected --file error, got: %v", err)
	}
}

func TestTransactionEdit_NoEditableFlags(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "at least one editable flag") {
		t.Fatalf("expected at-least-one-flag error, got: %v", err)
	}
}

func TestTransactionEdit_InvalidID(t *testing.T) {
	dbPath, _, _ := editFixture(t)
	_, err := runEdit(t, "--txn-id", "not-a-uuid", "--amount", "-1.00", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "invalid --txn-id") {
		t.Fatalf("expected invalid --txn-id error, got: %v", err)
	}
}

func TestTransactionEdit_Amount_BalanceReflects(t *testing.T) {
	dbPath, acctID, txnID := editFixture(t)

	out, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-75.25", "--file", dbPath)
	if err != nil {
		t.Fatalf("transaction edit --amount: %v", err)
	}
	if !strings.Contains(out, "Transaction updated successfully") {
		t.Errorf("expected success message, got:\n%s", out)
	}

	txn, svc, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if txn.Amount.String() != "-75.25" {
		t.Errorf("amount = %s, want -75.25", txn.Amount.String())
	}
	bal, err := svc.Account.GetBalance(acctID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if !bal.CurrentBalance.Equal(types.MustNewMoney("924.75")) {
		t.Errorf("balance = %s, want 924.75", bal.CurrentBalance.String())
	}
}

func TestTransactionEdit_Date(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--date", "2026-01-15", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --date: %v", err)
	}

	txn, _, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if txn.Date.String() != "2026-01-15" {
		t.Errorf("date = %s, want 2026-01-15", txn.Date.String())
	}
}

func TestTransactionEdit_InvalidDate(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--date", "01/15/2026", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Fatalf("expected invalid --date error, got: %v", err)
	}
}

func TestTransactionEdit_PayeeAutoCreates(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	out, err := runEdit(t, "--txn-id", txnID.String(), "--payee", "New Bakery", "--file", dbPath)
	if err != nil {
		t.Fatalf("transaction edit --payee: %v", err)
	}
	if !strings.Contains(out, "New Bakery") {
		t.Errorf("expected new payee in output, got:\n%s", out)
	}

	txn, svc, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if !txn.PayeeID.Valid {
		t.Fatal("payee should be set")
	}
	py, err := svc.PayeeRepo.GetByID(txn.PayeeID.ID)
	if err != nil || py.Name != "New Bakery" {
		t.Errorf("payee = %v (err %v), want New Bakery", py, err)
	}
}

func TestTransactionEdit_ClearPayee(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--payee", "", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --payee \"\": %v", err)
	}

	txn, _, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if txn.PayeeID.Valid {
		t.Error("payee should be cleared")
	}
}

func TestTransactionEdit_Category(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Travel", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("failed to create category: %v", err)
	}
	database.Close()

	out, err := runEdit(t, "--txn-id", txnID.String(), "--category", "Travel", "--file", dbPath)
	if err != nil {
		t.Fatalf("transaction edit --category: %v", err)
	}
	if !strings.Contains(out, "Travel") {
		t.Errorf("expected new category in output, got:\n%s", out)
	}

	txn, _, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if !txn.CategoryID.Valid || txn.CategoryID.ID != cat.ID {
		t.Errorf("category = %v, want %s", txn.CategoryID, cat.ID)
	}
}

func TestTransactionEdit_UnknownCategory(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--category", "Nope", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected category-not-found error, got: %v", err)
	}
}

func TestTransactionEdit_ClearCategory(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--category", "", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --category \"\": %v", err)
	}

	txn, _, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if txn.CategoryID.Valid {
		t.Error("category should be cleared")
	}
}

func TestTransactionEdit_MemoSetAndClear(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--memo", "oat milk", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --memo: %v", err)
	}
	txn, _, closeDB := reload(t, dbPath, txnID)
	if !txn.Memo.Valid || txn.Memo.String != "oat milk" {
		t.Errorf("memo = %v, want oat milk", txn.Memo)
	}
	closeDB()

	// Explicit empty string clears; an unset flag would have left it alone.
	if _, err := runEdit(t, "--txn-id", txnID.String(), "--memo", "", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --memo \"\": %v", err)
	}
	txn, _, closeDB = reload(t, dbPath, txnID)
	defer closeDB()
	if txn.Memo.Valid {
		t.Errorf("memo should be cleared, got %q", txn.Memo.String)
	}
}

func TestTransactionEdit_MemoUnsetLeavesValue(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	// Edit a different field; the memo flag is not supplied.
	if _, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-60.00", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --amount: %v", err)
	}

	txn, _, closeDB := reload(t, dbPath, txnID)
	defer closeDB()
	if !txn.Memo.Valid || txn.Memo.String != "latte" {
		t.Errorf("memo = %v, want latte (unset flag must not clear)", txn.Memo)
	}
}

func TestTransactionEdit_StatusClearedAndBack(t *testing.T) {
	dbPath, _, txnID := editFixture(t)

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--status", "cleared", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --status cleared: %v", err)
	}
	txn, _, closeDB := reload(t, dbPath, txnID)
	if txn.Status != transactiondom.StatusCleared {
		t.Errorf("status = %s, want cleared", txn.Status)
	}
	closeDB()

	if _, err := runEdit(t, "--txn-id", txnID.String(), "--status", "uncleared", "--file", dbPath); err != nil {
		t.Fatalf("transaction edit --status uncleared: %v", err)
	}
	txn, _, closeDB = reload(t, dbPath, txnID)
	defer closeDB()
	if txn.Status != transactiondom.StatusUncleared {
		t.Errorf("status = %s, want uncleared", txn.Status)
	}
}

func TestTransactionEdit_StatusReconciledRejected(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--status", "reconciled", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "tmoney reconcile") {
		t.Fatalf("expected pointer to tmoney reconcile, got: %v", err)
	}
}

func TestTransactionEdit_StatusInvalid(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--status", "bogus", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "invalid --status") {
		t.Fatalf("expected invalid --status error, got: %v", err)
	}
}

func TestTransactionEdit_StatusVoidRejected(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--status", "void", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "transaction void") {
		t.Fatalf("expected pointer to transaction void, got: %v", err)
	}
}

func TestTransactionEdit_RefusesTransferLeg(t *testing.T) {
	dbPath, txnID := transferLegFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-1.00", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "transfer edit") {
		t.Fatalf("expected pointer to transfer edit, got: %v", err)
	}
}

func TestTransactionEdit_RefusesSplitParent(t *testing.T) {
	dbPath, txnID := splitParentFixture(t)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-1.00", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "split") {
		t.Fatalf("expected split-parent refusal, got: %v", err)
	}
}

func TestTransactionEdit_RefusesReconciled(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	markStatus(t, dbPath, txnID, transactiondom.StatusReconciled)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-1.00", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "reconciled") {
		t.Fatalf("expected reconciled refusal, got: %v", err)
	}
}

func TestTransactionEdit_RefusesVoid(t *testing.T) {
	dbPath, _, txnID := editFixture(t)
	markStatus(t, dbPath, txnID, transactiondom.StatusVoid)
	_, err := runEdit(t, "--txn-id", txnID.String(), "--amount", "-1.00", "--file", dbPath)
	if err == nil || !strings.Contains(err.Error(), "void") {
		t.Fatalf("expected void refusal, got: %v", err)
	}
}

func TestTransactionEdit_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "edit", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction edit --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "edit") {
		t.Errorf("expected help output to mention edit; got:\n%s", stdout.String())
	}
}

// transferLegFixture creates a database with a bank↔bank transfer and returns
// the db path plus one leg's transaction ID, for tests that assert the plain
// transaction verbs refuse a transfer leg.
func transferLegFixture(t *testing.T) (string, types.ID) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	defer database.Close()

	acctRepo := account.NewRepository(database)
	from := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	to := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	for _, a := range []*account.Account{from, to} {
		if err := acctRepo.Create(a); err != nil {
			t.Fatalf("failed to create account: %v", err)
		}
	}

	svc := app.NewServices(database)
	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: from.ID,
		ToAccountID:   to.ID,
		Date:          types.Today(),
		Amount:        types.MustNewMoney("100.00"),
	})
	if err != nil {
		t.Fatalf("failed to create transfer: %v", err)
	}
	return dbPath, res.From.RowID
}

// splitParentFixture creates a database with a checking account and a
// split transaction (two categorized lines), returning the db path and
// the parent transaction ID.
func splitParentFixture(t *testing.T) (string, types.ID) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	defer database.Close()

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	catRepo := category.NewRepository(database)
	food := category.NewCategory("Food", category.TypeExpense)
	fuel := category.NewCategory("Fuel", category.TypeExpense)
	for _, c := range []*category.Category{food, fuel} {
		if err := catRepo.Create(c); err != nil {
			t.Fatalf("failed to create category: %v", err)
		}
	}

	svc := app.NewServices(database)
	parent := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-100.00"))
	splits := []*transactiondom.Split{
		transactiondom.NewSplit(parent.ID, food.ID, types.MustNewMoney("-60.00")),
		transactiondom.NewSplit(parent.ID, fuel.ID, types.MustNewMoney("-40.00")),
	}
	if err := svc.Transaction.CreateWithSplits(parent, splits); err != nil {
		t.Fatalf("failed to create split transaction: %v", err)
	}
	return dbPath, parent.ID
}

// markStatus flips a fixture transaction's status directly through the
// repository's narrow status update.
func markStatus(t *testing.T, dbPath string, txnID types.ID, status transactiondom.Status) {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()
	txnRepo := transactiondom.NewRepository(database)
	if err := txnRepo.UpdateStatus(txnID, status); err != nil {
		t.Fatalf("failed to set status %s: %v", status, err)
	}
}
