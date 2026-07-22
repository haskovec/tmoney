package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/scheduled"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func runAcctDelete(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(append([]string{"account", "delete"}, args...), stdout, stderr)
	return stdout.String(), err
}

// accountExists reports whether an account with the given name is present.
func accountExists(t *testing.T, dbPath, name string) bool {
	t.Helper()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database.Close()
	_, err = accountdom.NewRepository(database).GetByName(name)
	return err == nil
}

func TestAccountDelete_UnknownAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()
	_, err := runAcctDelete(t, "Nope", "--file", dbPath, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestAccountDelete_DryRunLeavesAccount(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	out, err := runAcctDelete(t, "Checking", "--file", dbPath)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	for _, want := range []string{"Would delete account", "Checking", "--confirm"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in dry-run output, got:\n%s", want, out)
		}
	}
	if !accountExists(t, dbPath, "Checking") {
		t.Errorf("dry-run should not delete the account")
	}
}

func TestAccountDelete_ConfirmDeletesEmptyAccount(t *testing.T) {
	dbPath := seedAccount(t, newChecking("Checking"))
	out, err := runAcctDelete(t, "Checking", "--file", dbPath, "--confirm")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "Deleted account") {
		t.Errorf("expected deletion confirmation, got %s", out)
	}
	if accountExists(t, dbPath, "Checking") {
		t.Errorf("expected the account to be gone")
	}
}

func TestAccountDelete_RefusedWithTransactions(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD",
		types.MustNewMoney("1000.00"), types.MustParseDate("2020-01-01"))
	if err := accountdom.NewRepository(database).Create(acct); err != nil {
		t.Fatalf("setup account: %v", err)
	}
	txn := transactiondom.NewTransaction(acct.ID, types.Today(), types.MustNewMoney("-50.00"))
	if err := transactiondom.NewRepository(database).Create(txn); err != nil {
		t.Fatalf("setup transaction: %v", err)
	}
	database.Close()

	_, err := runAcctDelete(t, "Checking", "--file", dbPath, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "transactions") {
		t.Fatalf("expected has-transactions refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), "account close") {
		t.Errorf("expected a hint to close instead, got %v", err)
	}
	if !accountExists(t, dbPath, "Checking") {
		t.Errorf("account with transactions should not be deleted")
	}
}

func TestAccountDelete_RefusedWithScheduledReference(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acct := newChecking("Checking")
	if err := accountdom.NewRepository(database).Create(acct); err != nil {
		t.Fatalf("setup account: %v", err)
	}
	st := scheduled.NewTransactionWithAmount(acct.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := scheduled.NewRepository(database).Create(st); err != nil {
		t.Fatalf("setup schedule: %v", err)
	}
	database.Close()

	// dry-run warns
	out, err := runAcctDelete(t, "Checking", "--file", dbPath)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(out, "Warning") || !strings.Contains(out, "scheduled") {
		t.Errorf("expected scheduled warning in dry-run, got:\n%s", out)
	}

	// --confirm refuses
	_, err = runAcctDelete(t, "Checking", "--file", dbPath, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "scheduled") {
		t.Fatalf("expected scheduled-reference refusal, got %v", err)
	}
	if !accountExists(t, dbPath, "Checking") {
		t.Errorf("account referenced by a schedule should not be deleted")
	}
}

func TestAccountCmd_HelpListsEditAndDelete(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"account", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("account --help: %v", err)
	}
	for _, want := range []string{"edit", "delete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected `account --help` to list %q; got:\n%s", want, stdout.String())
		}
	}
}
