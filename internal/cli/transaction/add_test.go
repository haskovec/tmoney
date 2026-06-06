package transaction_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestTransactionAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--account", "Checking", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransactionAdd_MissingAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestTransactionAdd_MissingAmount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestTransactionAdd_InvalidAmount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "invalid"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected 'invalid --amount', got: %v", err)
	}
}

func TestTransactionAdd_InvalidDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--date", "invalid-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected 'invalid --date', got: %v", err)
	}
}

func TestTransactionAdd_AccountNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Nonexistent", "--amount", "-50.00"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestTransactionAdd_CategoryNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-50.00", "--category", "Nonexistent"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(transaction add) with nonexistent category should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestTransactionAdd_Basic(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-50.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Transaction created successfully") {
		t.Errorf("expected creation confirmation, got: %s", out)
	}
	if !strings.Contains(out, "Checking") {
		t.Errorf("expected account name in output, got: %s", out)
	}
	if !strings.Contains(out, "-$50.00") {
		t.Errorf("expected amount in output, got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	txnRepo := transactiondom.NewRepository(database)
	transactions, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Amount.String() != "-50" {
		t.Errorf("amount = %s, want -50", transactions[0].Amount.String())
	}
}

func TestTransactionAdd_PayeeAutoCreate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-5.50", "--payee", "Coffee Shop"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Coffee Shop") {
		t.Errorf("expected payee in output, got: %s", out)
	}
	if !strings.Contains(out, "(new)") {
		t.Errorf("expected '(new)' marker for auto-created payee, got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	payeeRepo := payee.NewRepository(database)
	p, err := payeeRepo.GetByName("Coffee Shop")
	if err != nil {
		t.Errorf("expected payee to be created: %v", err)
	}
	if p.Name != "Coffee Shop" {
		t.Errorf("payee name = %q, want %q", p.Name, "Coffee Shop")
	}
}

func TestTransactionAdd_ExistingPayee(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	payeeRepo := payee.NewRepository(database)
	existing := payee.NewPayee("Coffee Shop")
	if err := payeeRepo.Create(existing); err != nil {
		t.Fatalf("setup: create payee: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-5.50", "--payee", "Coffee Shop"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Coffee Shop") {
		t.Errorf("expected payee in output, got: %s", out)
	}
	if strings.Contains(out, "(new)") {
		t.Errorf("did not expect '(new)' for existing payee, got: %s", out)
	}
}

func TestTransactionAdd_WithCategory(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Food", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-25.00", "--category", "Food"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "Food") {
		t.Errorf("expected category in output, got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	txnRepo := transactiondom.NewRepository(database)
	transactions, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(transactions))
	}
	if !transactions[0].CategoryID.Valid {
		t.Error("transaction should have category set")
	}
}

func TestTransactionAdd_WithDateAndMemo(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.NewDate(2000, time.January, 1))
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"transaction", "add", "--file", dbPath, "--account", "Checking", "--amount", "-15.00", "--date", "2024-01-15", "--memo", "Lunch with friend"}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("expected date in output, got: %s", out)
	}
	if !strings.Contains(out, "Lunch with friend") {
		t.Errorf("expected memo in output, got: %s", out)
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()
	txnRepo := transactiondom.NewRepository(database)
	transactions, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(transactions))
	}
	if transactions[0].Date.String() != "2024-01-15" {
		t.Errorf("date = %s, want 2024-01-15", transactions[0].Date.String())
	}
	if !transactions[0].Memo.Valid || transactions[0].Memo.String != "Lunch with friend" {
		t.Errorf("memo = %v, want 'Lunch with friend'", transactions[0].Memo)
	}
}

func TestTransactionAdd_FullExample(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.NewDate(2000, time.January, 1))
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Dining", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transaction", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--amount", "-45.50",
		"--payee", "Olive Garden",
		"--category", "Dining",
		"--date", "2024-06-15",
		"--memo", "Birthday dinner",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"Transaction created successfully", "Checking", "-$45.50", "Olive Garden", "Dining", "2024-06-15", "Birthday dinner"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransactionCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transaction --help` to list `add`; got:\n%s", stdout.String())
	}
}

func TestTransactionAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"transaction", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(transaction add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transaction add --help` to describe the command; got:\n%s", stdout.String())
	}
}
