package account_test

import (
	"bytes"
	"strings"
	"testing"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
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

// A brokerage keeps its history on the investment ledger.
func TestAccountDelete_BrokerageWithHistory(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	svc := app.NewServices(database)
	acct := accountdom.NewAccount("Northwind Brokerage", accountdom.TypeInvestment, "USD",
		types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := svc.Investment.Deposit(acct.ID, types.MustParseDate("2020-02-01"), types.MustNewMoney("100.00"), ""); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	out, err := runAcctDelete(t, "Northwind Brokerage", "--file", dbPath)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(out, "Warning: cannot delete account \"Northwind Brokerage\": it has 1 investment transactions") {
		t.Errorf("preview should warn that delete is blocked, got: %s", out)
	}

	_, err = runAcctDelete(t, "Northwind Brokerage", "--file", dbPath, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "1 investment transactions — close it instead") {
		t.Fatalf("expected the investment-history refusal, got %v", err)
	}
	if strings.Contains(err.Error(), "Constraint Error") {
		t.Errorf("the refusal leaked a driver error: %v", err)
	}
	if !accountExists(t, dbPath, "Northwind Brokerage") {
		t.Error("the brokerage was deleted")
	}
}

func TestAccountDelete_RefusedWithActiveReconciliation(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	svc := app.NewServices(database)
	acct := accountdom.NewAccount("Checking", accountdom.TypeChecking, "USD",
		types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := svc.Account.Create(acct); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := svc.Reconciliation.StartReconciliation(acct.ID, types.MustParseDate("2020-02-01"), types.ZeroMoney); err != nil {
		t.Fatalf("setup: %v", err)
	}
	database.Close()

	_, err := runAcctDelete(t, "Checking", "--file", dbPath, "--confirm")
	if err == nil || !strings.Contains(err.Error(), "finish it (tmoney reconcile finish) or cancel it in the TUI") {
		t.Fatalf("expected the active-reconciliation refusal, got %v", err)
	}
	if !accountExists(t, dbPath, "Checking") {
		t.Error("the account was deleted")
	}
}
