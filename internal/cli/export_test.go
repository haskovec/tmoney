package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestExport_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"export", "out.csv"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(export) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestExport_MissingPositionalFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"export", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(export) without positional file should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestExport_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", filepath.Join(tmpDir, "out.csv"),
		"--file", dbPath,
		"--format", "ofx",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("export with OFX format should return error")
	}
	if !strings.Contains(err.Error(), "must be csv or qif") {
		t.Errorf("error should mention valid formats, got: %v", err)
	}
}

func TestExport_UndetectableFormat(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.xyz")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("export with undetectable format should return error")
	}
	if !strings.Contains(err.Error(), "cannot detect format") {
		t.Errorf("error should mention format detection failure, got: %v", err)
	}
}

func TestExport_CSV(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn1 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(txn1); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	txn2 := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-15"), types.MustNewMoney("-120.00"))
	if err := txnRepo.Create(txn2); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.csv")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(export csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "EXPORT COMPLETE") {
		t.Error("output should contain EXPORT COMPLETE header")
	}
	if !strings.Contains(output, "Transactions: 2") {
		t.Errorf("output should show 2 transactions, got: %s", output)
	}
	if !strings.Contains(output, "CSV") {
		t.Error("output should show CSV format")
	}

	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	csvStr := string(content)
	if !strings.Contains(csvStr, "Date,Account,Payee,Category,Amount") {
		t.Error("CSV should contain header row")
	}
	if !strings.Contains(csvStr, "2024-03-01") {
		t.Error("CSV should contain first transaction date")
	}
	if !strings.Contains(csvStr, "2024-03-15") {
		t.Error("CSV should contain second transaction date")
	}
}

func TestExport_QIF(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-06-01"), types.MustNewMoney("-75.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.qif")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(export qif) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "QIF") {
		t.Error("output should show QIF format")
	}
	content, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export file: %v", err)
	}
	qifStr := string(content)
	if !strings.Contains(qifStr, "!Type:") {
		t.Error("QIF should contain type header")
	}
	if !strings.Contains(qifStr, "T-75.00") {
		t.Error("QIF should contain transaction amount")
	}
}

func TestExport_WithAccountFilter(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("5000"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	if err := txnRepo.Create(transaction.NewTransaction(checking.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	if err := txnRepo.Create(transaction.NewTransaction(savings.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-25.00"))); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "checking.csv")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(export --account) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Accounts:     1") {
		t.Errorf("output should show 1 account, got: %s", output)
	}
	if !strings.Contains(output, "Transactions: 1") {
		t.Errorf("output should show 1 transaction, got: %s", output)
	}
}

func TestExport_WithDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	for _, pair := range []struct {
		date   string
		amount string
	}{
		{"2024-01-15", "-50.00"},
		{"2024-03-15", "-75.00"},
		{"2024-06-15", "-100.00"},
	} {
		if err := txnRepo.Create(transaction.NewTransaction(acct.ID, types.MustParseDate(pair.date), types.MustNewMoney(pair.amount))); err != nil {
			t.Fatalf("failed to create transaction: %v", err)
		}
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "q1.csv")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
		"--from", "2024-01-01",
		"--to", "2024-03-31",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(export --from --to) returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Transactions: 2") {
		t.Errorf("output should show 2 transactions for Q1, got: %s", stdout.String())
	}
}

func TestExport_FormatOverrideCSV(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	if err := txnRepo.Create(transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))); err != nil {
		t.Fatalf("failed to create transaction: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "export.txt")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(export --format csv) returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "CSV") {
		t.Error("output should show CSV format")
	}
}

func TestExport_AccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("export with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

func TestExport_NoTransactions(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Empty", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	exportPath := filepath.Join(tmpDir, "out.csv")
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"export", exportPath,
		"--file", dbPath,
		"--account", "Empty",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("export with no transactions should return error")
	}
	if !strings.Contains(err.Error(), "no transactions") {
		t.Errorf("error should mention no transactions, got: %v", err)
	}
}

func TestExport_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := ExecuteWith([]string{"export", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(export --help): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"--account", "--format", "--from", "--to"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `export --help` to mention %q; got:\n%s", want, out)
		}
	}
}
