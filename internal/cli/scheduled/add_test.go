package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

func TestScheduledAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "add", "--account", "Checking", "--frequency", "monthly"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledAdd_MissingAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "add", "--file", dbPath, "--frequency", "monthly"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled add) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestScheduledAdd_MissingFrequency(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "add", "--file", dbPath, "--account", "Checking"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled add) without --frequency should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "frequency") {
		t.Errorf("expected Cobra required-flag error mentioning frequency, got: %v", err)
	}
}

func TestScheduledAdd_InvalidFrequency(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "invalid",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled add) with invalid frequency should return error")
	}
	if !strings.Contains(err.Error(), "invalid --frequency") {
		t.Errorf("error should mention invalid frequency, got: %v", err)
	}
}

func TestScheduledAdd_Success(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-1500",
		"--payee", "Landlord",
		"--memo", "Monthly rent",
		"--day", "1",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled add): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"Scheduled transaction created successfully!",
		"Checking",
		"Monthly",
		"-$1500.00",
		"Landlord",
		"Monthly rent",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got: %s", want, output)
		}
	}
}

func TestScheduledAdd_WithAutoPost(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-150",
		"--payee", "Insurance",
		"--auto-post",
		"--lead-days", "3",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled add --auto-post): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Auto-post: Yes (3 days early)") {
		t.Errorf("output should contain auto-post indicator, got: %s", output)
	}
}

func TestScheduledAdd_LeadDaysWithoutAutoPost(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--lead-days", "3",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("--lead-days without --auto-post should return error")
	}
	if !strings.Contains(err.Error(), "requires --auto-post") {
		t.Errorf("error should mention --auto-post requirement, got: %v", err)
	}
}

func TestScheduledAdd_InvalidLeadDays(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--auto-post",
		"--lead-days", "5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("--lead-days 5 should return error")
	}
	if !strings.Contains(err.Error(), "must be 0, 3, or 7") {
		t.Errorf("error should mention valid lead-days values, got: %v", err)
	}
}

func TestScheduledAdd_WithOccurrences(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--amount", "-100",
		"--occurrences", "12",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled add --occurrences): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Scheduled transaction created successfully!") {
		t.Error("output should contain success message")
	}
}

func TestScheduledAdd_VariableAmount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("1000.00"),
		types.Today(),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add",
		"--file", dbPath,
		"--account", "Checking",
		"--frequency", "monthly",
		"--payee", "Electric Co",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled add variable amount): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Variable") {
		t.Error("output should show variable amount")
	}
}

func TestScheduledCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `scheduled --help` to list `add`; got:\n%s", stdout.String())
	}
}

func TestScheduledAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `scheduled add --help` to describe the command; got:\n%s", stdout.String())
	}
}
