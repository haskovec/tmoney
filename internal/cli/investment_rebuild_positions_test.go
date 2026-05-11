package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentRebuildPositions_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"investment", "rebuild-positions"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(rebuild-positions) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentRebuildPositions_NoInvestmentAccounts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	err = executeWith([]string{"investment", "rebuild-positions", "--file", dbPath}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("rebuild-positions error = %v", err)
	}
	if !strings.Contains(stdout.String(), "No investment accounts found") {
		t.Errorf("expected stdout to mention no investment accounts, got: %q", stdout.String())
	}
}

func TestInvestmentRebuildPositions_AllAccountsBasic(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	acctRepo := account.NewRepository(database)
	inv := account.NewAccount("Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := acctRepo.Create(inv); err != nil {
		t.Fatalf("failed to create investment account: %v", err)
	}
	database.Close()

	stdout := &bytes.Buffer{}
	err = executeWith([]string{"investment", "rebuild-positions", "--file", dbPath}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("rebuild-positions error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Brokerage:") {
		t.Errorf("expected stdout to mention Brokerage account, got: %q", stdout.String())
	}
}

func TestInvestmentRebuildPositions_UnknownAccount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	database.Close()

	err = executeWith([]string{
		"investment", "rebuild-positions",
		"--file", dbPath,
		"--account", "Nope",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error for unknown account")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}
