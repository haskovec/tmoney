package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func setupTransferAccounts(t *testing.T) (string, *account.Account, *account.Account) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	if err := repo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}
	database.Close()
	return dbPath, checking, savings
}

func TestTransferAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"transfer", "add", "--from", "Checking", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestTransferAdd_MissingFrom(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --from should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "from") {
		t.Errorf("expected Cobra required-flag error mentioning from, got: %v", err)
	}
}

func TestTransferAdd_MissingTo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --to should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "to") {
		t.Errorf("expected Cobra required-flag error mentioning to, got: %v", err)
	}
}

func TestTransferAdd_MissingAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) without --amount should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "amount") {
		t.Errorf("expected Cobra required-flag error mentioning amount, got: %v", err)
	}
}

func TestTransferAdd_InvalidAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "not-a-number"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "invalid --amount") {
		t.Errorf("expected 'invalid --amount', got: %v", err)
	}
}

func TestTransferAdd_NegativeAmount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "-100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with negative amount should return error")
	}
	if !strings.Contains(err.Error(), "must be positive") {
		t.Errorf("expected 'must be positive', got: %v", err)
	}
}

func TestTransferAdd_SourceAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Nonexistent", "--to", "Savings", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with nonexistent source account should return error")
	}
	if !strings.Contains(err.Error(), "source account") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'source account ... not found', got: %v", err)
	}
}

func TestTransferAdd_DestAccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Nonexistent", "--amount", "100"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with nonexistent destination account should return error")
	}
	if !strings.Contains(err.Error(), "destination account") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'destination account ... not found', got: %v", err)
	}
}

func TestTransferAdd_InvalidDate(t *testing.T) {
	dbPath, _, _ := setupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100",
		"--date", "not-a-date",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(transfer add) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected 'invalid --date', got: %v", err)
	}
}

func TestTransferAdd_Basic(t *testing.T) {
	dbPath, _, _ := setupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"transfer", "add", "--file", dbPath, "--from", "Checking", "--to", "Savings", "--amount", "100.00"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Transfer created successfully", "Checking", "Savings", "$100.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransferAdd_WithDateAndMemo(t *testing.T) {
	dbPath, _, _ := setupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "250.50",
		"--date", "2024-06-15",
		"--memo", "Monthly savings",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Transfer created successfully", "2024-06-15", "Monthly savings", "$250.50"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestTransferAdd_VerifyTransactions(t *testing.T) {
	dbPath, checking, savings := setupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"transfer", "add",
		"--file", dbPath,
		"--from", "Checking",
		"--to", "Savings",
		"--amount", "100.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(transfer add): %v\nstderr=%s", err, stderr)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("post: db.Open: %v", err)
	}
	defer database.Close()

	txnRepo := transaction.NewRepository(database)

	checkingTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list checking transactions: %v", err)
	}
	if len(checkingTxns) != 1 {
		t.Fatalf("expected 1 checking transaction, got %d", len(checkingTxns))
	}
	if !checkingTxns[0].Amount.Equal(types.MustNewMoney("-100.00")) {
		t.Errorf("checking amount = %s, want -100.00", checkingTxns[0].Amount.String())
	}
	if !checkingTxns[0].IsTransfer() {
		t.Error("checking transaction should be a transfer")
	}

	savingsTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("list savings transactions: %v", err)
	}
	if len(savingsTxns) != 1 {
		t.Fatalf("expected 1 savings transaction, got %d", len(savingsTxns))
	}
	if !savingsTxns[0].Amount.Equal(types.MustNewMoney("100.00")) {
		t.Errorf("savings amount = %s, want 100.00", savingsTxns[0].Amount.String())
	}
	if !savingsTxns[0].IsTransfer() {
		t.Error("savings transaction should be a transfer")
	}
	if checkingTxns[0].TransferID.ID != savingsTxns[0].TransferID.ID {
		t.Error("both transactions should share the same transfer ID")
	}
}

func TestTransferCmd_HelpListsAdd(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"transfer", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transfer --help` to list `add`; got:\n%s", stdout.String())
	}
}

func TestTransferAdd_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"transfer", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(transfer add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `transfer add --help` to describe the command; got:\n%s", stdout.String())
	}
}
