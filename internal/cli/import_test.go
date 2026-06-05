package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// writeImportTestFile is a small file-write helper local to the
// `tmoney import` tests.
func writeImportTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

func TestImport_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"import", "bank.csv", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestImport_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"import", "bank.csv", "--file", "test.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) without --account should return error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "account" not set`) {
		t.Errorf("expected Cobra required-flag error, got: %v", err)
	}
}

func TestImport_MissingPositionalFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{"import", "--file", "test.tdb", "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) without positional file should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
		t.Errorf("expected Cobra exact-args error, got: %v", err)
	}
}

func TestImport_MutuallyExclusiveDuplicateFlags(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--skip-duplicates",
		"--update-duplicates",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) with both --skip-duplicates and --update-duplicates should return error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutually exclusive, got: %v", err)
	}
}

func TestImport_InvalidFormat(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", "bank.csv",
		"--file", "test.tdb",
		"--account", "Checking",
		"--format", "xml",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) with invalid --format should return error")
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestImport_NonexistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", "/nonexistent/bank.csv",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(import) with nonexistent import file should return error")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error should mention failed to open, got: %v", err)
	}
}

func TestImport_CSVDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Category,Memo\n2024-03-01,-50.00,Coffee Shop,Food:Coffee,Morning coffee\n2024-03-02,-120.00,Electric Co,Bills:Utilities,March electric\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(import dry-run) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
	if !strings.Contains(output, "Checking") {
		t.Error("output should contain account name")
	}
	if !strings.Contains(output, "2 transactions") {
		t.Error("output should show 2 parsed transactions")
	}
	if !strings.Contains(output, "Run with --confirm") {
		t.Error("output should prompt to run with --confirm")
	}
}

func TestImport_CSVConfirm(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee,Memo\n2024-03-01,-50.00,Coffee Shop,Morning coffee\n2024-03-02,-120.00,Electric Co,March electric\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--confirm",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(import --confirm) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE") {
		t.Error("output should contain IMPORT COMPLETE header")
	}
	if !strings.Contains(output, "Created:  2") {
		t.Errorf("output should show 2 created transactions, got: %s", output)
	}

	database2, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database2.Close()

	txnRepo := transaction.NewRepository(database2)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("expected 2 transactions, got %d", len(txns))
	}
}

func TestImport_ClosedAccount(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	repo := account.NewRepository(database)
	acct := account.NewAccount("Closed Account", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	acct.Active = false
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Closed Account",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("import into closed account should return error")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should mention account is closed, got: %v", err)
	}
}

func TestImport_FormatOverride(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	repo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("0"), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.txt")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--format", "csv",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(import --format csv) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestImport_SkipDuplicates(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	accountRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000"), types.Today())
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	existingTxn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-03-01"), types.MustNewMoney("-50.00"))
	if err := txnRepo.Create(existingTxn); err != nil {
		t.Fatalf("failed to create existing transaction: %v", err)
	}

	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n2024-03-02,-75.00,Gas Station\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--skip-duplicates",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(import --skip-duplicates) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT PREVIEW") {
		t.Error("output should contain IMPORT PREVIEW header")
	}
}

func TestImport_AccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")
	database.Close()

	csvPath := filepath.Join(tmpDir, "import.csv")
	csvContent := "Date,Amount,Payee\n2024-03-01,-50.00,Coffee Shop\n"
	if err := writeImportTestFile(csvPath, csvContent); err != nil {
		t.Fatalf("failed to write CSV file: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("import with nonexistent account should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention account not found, got: %v", err)
	}
}

// setupMultiAccountImportFixture creates a test database with two
// accounts and a CSV file containing transactions for both. Returns the
// database and CSV paths.
func setupMultiAccountImportFixture(t *testing.T) (dbPath, csvPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	database, dbPath := dbtest.NewFileIn(t, tmpDir, "test.tdb")

	acctRepo := account.NewRepository(database)
	for _, name := range []string{"Checking", "Savings"} {
		acct := account.NewAccount(name, account.TypeChecking, "USD",
			types.MustNewMoney("0"), types.NewDate(2024, 1, 1))
		if err := acctRepo.Create(acct); err != nil {
			database.Close()
			t.Fatalf("failed to create account %s: %v", name, err)
		}
	}
	database.Close()

	csvPath = filepath.Join(tmpDir, "register.csv")
	csv := "Date,Account,Payee,Category,Amount,Memo,Check Number,Status,Transfer Account\n" +
		"2024-01-10,Checking,Coffee Shop,,-5.00,,,,\n" +
		"2024-01-11,Checking,Employer,,3000.00,,,,\n" +
		"2024-01-12,Savings,Interest,,5.00,,,,\n"
	if err := os.WriteFile(csvPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("failed to write CSV: %v", err)
	}
	return dbPath, csvPath
}

func TestImport_MultiAccountCSV_RequiresSourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error when CSV contains multiple accounts and no --source-account")
	}
	msg := err.Error()
	if !strings.Contains(msg, "transactions for 2 accounts") {
		t.Errorf("error should mention multiple accounts, got: %v", err)
	}
	if !strings.Contains(msg, "Checking") || !strings.Contains(msg, "Savings") {
		t.Errorf("error should list account names, got: %v", err)
	}
	if !strings.Contains(msg, "--source-account") {
		t.Errorf("error should suggest --source-account, got: %v", err)
	}
}

func TestImport_MultiAccountCSV_FiltersBySourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--source-account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "IMPORT PREVIEW") {
		t.Errorf("expected dry-run preview output, got: %s", out)
	}
	if !strings.Contains(out, "Parsed: 2 transactions") {
		t.Errorf("expected 2 parsed (Checking only), got: %s", out)
	}
}

func TestImport_MultiAccountCSV_UnknownSourceAccount(t *testing.T) {
	dbPath, csvPath := setupMultiAccountImportFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := ExecuteWith([]string{
		"import", csvPath,
		"--file", dbPath,
		"--account", "Checking",
		"--source-account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error when --source-account is not in the CSV")
	}
	if !strings.Contains(err.Error(), "not found in import file") {
		t.Errorf("error should mention source-account not found, got: %v", err)
	}
}

func TestImport_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := ExecuteWith([]string{"import", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(import --help): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"--account", "--confirm", "--source-account", "--format"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `import --help` to mention %q; got:\n%s", want, out)
		}
	}
}
