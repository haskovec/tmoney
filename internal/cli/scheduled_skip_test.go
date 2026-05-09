package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestScheduledSkip_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"scheduled", "skip", "abc123"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(scheduled skip) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledSkip_MissingID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"scheduled", "skip", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(scheduled skip) without ID arg should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestScheduledSkip_InvalidID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"scheduled", "skip", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(scheduled skip) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestScheduledSkip_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"scheduled", "skip", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(scheduled skip) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestScheduledSkip_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Netflix")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	stRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()
	originalNextDate := st.NextDate.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"scheduled", "skip", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(scheduled skip): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"skipped", "Checking", "Netflix", "Monthly", "Skipped:", originalNextDate, "Next:"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen database: %v", err)
	}
	defer database.Close()

	txnRepo := transaction.NewRepository(database)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("expected 0 transactions after skip, got %d", len(txns))
	}
}

func TestScheduledSkip_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"scheduled", "skip", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(scheduled skip --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "skip") {
		t.Errorf("expected `scheduled skip --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestScheduledCmd_HelpListsSkip(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"scheduled", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(scheduled --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "skip") {
		t.Errorf("expected `scheduled --help` to list `skip`; got:\n%s", stdout.String())
	}
}
