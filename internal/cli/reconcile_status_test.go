package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/reconciliation"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReconcileStatus_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"reconcile", "status", "--account", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(reconcile status) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestReconcileStatus_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{
		"reconcile", "status", "--file", "test.tdb",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(reconcile status) without --account should return error")
	}
	if !strings.Contains(err.Error(), `required flag(s) "account" not set`) {
		t.Errorf("expected Cobra required-flag error mentioning \"account\", got: %v", err)
	}
}

func TestReconcileStatus_NoSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{
		"reconcile", "status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(reconcile status): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Errorf("output should contain status header, got:\n%s", output)
	}
	if !strings.Contains(output, "Last reconciled:  Never") {
		t.Errorf("output should show 'Never' for last reconciled, got:\n%s", output)
	}
	if !strings.Contains(output, "Current session:  None") {
		t.Errorf("output should show 'None' for current session, got:\n%s", output)
	}
}

func TestReconcileStatus_WithActiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	reconRepo := reconciliation.NewRepository(database)
	session := reconciliation.NewSession(
		acct.ID,
		types.MustParseDate("2024-01-31"),
		types.MustNewMoney("5000.00"),
	)
	if err := reconRepo.Create(session); err != nil {
		t.Fatalf("failed to create reconciliation session: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{
		"reconcile", "status",
		"--file", dbPath,
		"--account", "Checking",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(reconcile status): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "RECONCILIATION STATUS: Checking") {
		t.Errorf("output should contain status header, got:\n%s", output)
	}
	if !strings.Contains(output, "In progress") {
		t.Errorf("output should show 'In progress', got:\n%s", output)
	}
	if !strings.Contains(output, "2024-01-31") {
		t.Errorf("output should contain statement date, got:\n%s", output)
	}
	if !strings.Contains(output, "$5000.00") {
		t.Errorf("output should contain statement balance, got:\n%s", output)
	}
}

func TestReconcileStatus_AccountNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{
		"reconcile", "status",
		"--file", dbPath,
		"--account", "Nope",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(reconcile status) for unknown account should return error")
	}
	if !strings.Contains(err.Error(), `account "Nope" not found`) {
		t.Errorf("expected account-not-found error, got: %v", err)
	}
}

func TestReconcileStatus_Help(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"reconcile", "status", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(reconcile status --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "status") {
		t.Errorf("expected `reconcile status --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestReconcileCmd_HelpListsStatus(t *testing.T) {
	_, _, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"reconcile", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(reconcile --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "status") {
		t.Errorf("expected `reconcile --help` to list `status`; got:\n%s", stdout.String())
	}
}
