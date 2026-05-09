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

func TestReportNetWorth_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"report", "net-worth"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report net-worth) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestReportNetWorth_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "empty.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report net-worth): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"NET WORTH REPORT", "ASSETS", "LIABILITIES", "NET WORTH:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportNetWorth_WithAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("5000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD",
		types.MustNewMoney("10000.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}
	creditCard := account.NewAccount("Credit Card", account.TypeCreditCard, "USD",
		types.MustNewMoney("0"), types.Today())
	if err := acctRepo.Create(creditCard); err != nil {
		t.Fatalf("setup: create credit card: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(creditCard.ID, types.Today(), types.MustNewMoney("-500.00"))
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("setup: create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report net-worth): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"Checking", "Savings", "Credit Card"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportNetWorth_WithAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--as-of", "2024-01-15", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report net-worth --as-of): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "January 15, 2024") {
		t.Errorf("expected 'January 15, 2024' in output, got:\n%s", stdout.String())
	}
}

func TestReportNetWorth_InvalidAsOf(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"report", "net-worth", "--as-of", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report net-worth --as-of=not-a-date) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --as-of date") {
		t.Errorf("expected 'invalid --as-of date' in error, got: %v", err)
	}
}

func TestReportNetWorth_IncludeClosed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	acctRepo := account.NewRepository(database)
	open := account.NewAccount("OpenCheck", account.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(open); err != nil {
		t.Fatalf("setup: create open: %v", err)
	}
	closed := account.NewAccount("ClosedSavings", account.TypeSavings, "USD",
		types.MustNewMoney("250.00"), types.Today())
	if err := acctRepo.Create(closed); err != nil {
		t.Fatalf("setup: create closed: %v", err)
	}
	closed.Close()
	if err := acctRepo.Update(closed); err != nil {
		t.Fatalf("setup: close savings: %v", err)
	}
	database.Close()

	stdoutDefault, stderrDefault := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--file", dbPath}, stdoutDefault, stderrDefault); err != nil {
		t.Fatalf("executeWith(report net-worth): %v\nstderr=%s", err, stderrDefault)
	}
	if strings.Contains(stdoutDefault.String(), "ClosedSavings") {
		t.Errorf("default report should not include closed account; got:\n%s", stdoutDefault.String())
	}

	stdoutAll, stderrAll := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--include-closed", "--file", dbPath}, stdoutAll, stderrAll); err != nil {
		t.Fatalf("executeWith(report net-worth --include-closed): %v\nstderr=%s", err, stderrAll)
	}
	if !strings.Contains(stdoutAll.String(), "ClosedSavings") {
		t.Errorf("--include-closed report should include closed account; got:\n%s", stdoutAll.String())
	}
}

func TestReportNetWorth_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"report", "net-worth", "extra", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report net-worth extra) should return error")
	}
}

func TestReportCmd_HelpListsNetWorth(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "net-worth") {
		t.Errorf("expected `report --help` to list `net-worth`; got:\n%s", stdout.String())
	}
}

func TestReportNetWorth_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "net-worth", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report net-worth --help): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"net-worth", "--as-of", "--include-closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `report net-worth --help` to contain %q; got:\n%s", want, out)
		}
	}
}
