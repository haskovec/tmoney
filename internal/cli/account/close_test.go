package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

func TestAccountClose_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("close without --file should return an error")
	}
}

func TestAccountClose_MissingNameArg(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("close without a positional name should return an error")
	}
}

func TestAccountClose_NotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Nope", "--file", dbPath}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found', got %v", err)
	}
}

func TestAccountClose_HappyPath(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath, "--date", "2024-06-15"}, stdout, stderr); err != nil {
		t.Fatalf("close: %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	for _, want := range []string{"Account closed.", "Checking", "2024-06-15"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in close output, got: %s", want, out)
		}
	}

	// The close date must persist — show it back.
	stdout.Reset()
	stderr.Reset()
	if err := cli.ExecuteWith([]string{"account", "show", "Checking", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("show after close: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Closed (2024-06-15)") {
		t.Errorf("expected 'Closed (2024-06-15)' in show output, got: %s", stdout.String())
	}
}

func TestAccountClose_DefaultsToToday(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("close (default date): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), types.Today().String()) {
		t.Errorf("expected today's date in close output, got: %s", stdout.String())
	}
}

func TestAccountClose_NonZeroBalanceRejected(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("100.00"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "zero") {
		t.Fatalf("expected zero-balance error, got %v", err)
	}
}

func TestAccountClose_FutureDateRejected(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	future := types.Today().AddYears(1).String()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath, "--date", future}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "must be between") {
		t.Fatalf("expected close-date-range error, got %v", err)
	}
}

func TestAccountClose_BadDateRejected(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath, "--date", "not-a-date"}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Fatalf("expected invalid --date error, got %v", err)
	}
}

func TestAccountClose_AlreadyClosedRejected(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	acct.Close(types.MustParseDate("2024-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath}, stdout, stderr)
	if err == nil || !strings.Contains(err.Error(), "already closed") {
		t.Fatalf("expected already-closed error, got %v", err)
	}
}

func TestAccountClose_ScheduledWarning(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	repo := accountdom.NewRepository(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD", types.MustNewMoney("0"), types.MustParseDate("2020-01-01"))
	if err := repo.Create(acct); err != nil {
		t.Fatalf("setup account: %v", err)
	}
	schedRepo := scheduled.NewRepository(database)
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := schedRepo.Create(st); err != nil {
		t.Fatalf("setup schedule: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "close", "Checking", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("close should succeed with a warning, got %v\nstderr=%s", err, stderr)
	}
	out := stdout.String()
	if !strings.Contains(out, "Account closed.") {
		t.Errorf("expected the close to succeed, got: %s", out)
	}
	if !strings.Contains(out, "Warning") || !strings.Contains(out, "scheduled") {
		t.Errorf("expected a scheduled-reference warning, got: %s", out)
	}
}

func TestAccountCmd_HelpListsClose(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(account --help): %v", err)
	}
	for _, want := range []string{"close", "reopen"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected `account --help` to list %q; got:\n%s", want, stdout.String())
		}
	}
}
