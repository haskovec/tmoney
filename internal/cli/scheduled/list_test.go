package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

func TestScheduledList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledList_NoTransactions(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "SCHEDULED TRANSACTIONS") {
		t.Error("output should contain SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "No scheduled transactions found") {
		t.Error("output should indicate no scheduled transactions found")
	}
}

func TestScheduledList_WithTransactions(t *testing.T) {
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
		t.Fatalf("failed to create test account: %v", err)
	}

	payeeRepo := payee.NewRepository(database)
	py := payee.NewPayee("Netflix")
	if err := payeeRepo.Create(py); err != nil {
		t.Fatalf("failed to create payee: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(
		acct.ID,
		scheduleddom.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-15.99"),
	)
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"SCHEDULED TRANSACTIONS", "Checking", "Netflix", "Monthly", "-$15.99", "Showing 1 scheduled transaction(s)"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestScheduledList_DueOnly(t *testing.T) {
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
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(
		acct.ID,
		scheduleddom.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-10.00"),
	)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--due", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list --due): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "DUE SCHEDULED TRANSACTIONS") {
		t.Error("output should contain DUE SCHEDULED TRANSACTIONS header")
	}
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Error("output should show count of due transactions")
	}
}

func TestScheduledList_FilterByAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("failed to create checking account: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("failed to create savings account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st1 := scheduleddom.NewTransactionWithAmount(checking.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-10.00"))
	if err := stRepo.Create(st1); err != nil {
		t.Fatalf("failed to create scheduled transaction 1: %v", err)
	}
	st2 := scheduleddom.NewTransactionWithAmount(savings.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-20.00"))
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction 2: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--account", "Checking", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list --account Checking): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "Showing 1 scheduled transaction(s)") {
		t.Errorf("output should show 1 scheduled transaction, got: %s", output)
	}
	if !strings.Contains(output, "-$10.00") {
		t.Error("output should contain the checking account scheduled transaction")
	}
}

func TestScheduledList_VariableAmount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransaction(acct.ID, scheduleddom.FrequencyMonthly, types.Today())
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "~") {
		t.Error("output should show ~ for variable amount")
	}
}

func TestScheduledList_WithOccurrences(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	st.SetOccurrences(5)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "(5 left)") {
		t.Error("output should show occurrences remaining")
	}
}

func TestScheduledList_ShowsAutoPostIndicator(t *testing.T) {
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
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(
		acct.ID,
		scheduleddom.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-1500.00"),
	)
	st.SetAutoPost(true)
	st.SetPostLeadDays(3)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	st2 := scheduleddom.NewTransactionWithAmount(
		acct.ID,
		scheduleddom.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-50.00"),
	)
	if err := stRepo.Create(st2); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto 3d]") {
		t.Errorf("output should contain [Auto 3d] indicator, got: %s", output)
	}
	if !strings.Contains(output, "Auto") {
		t.Error("output should contain Auto header column")
	}
}

func TestScheduledList_AutoPostZeroLeadDays(t *testing.T) {
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
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(
		acct.ID,
		scheduleddom.FrequencyMonthly,
		types.Today(),
		types.MustNewMoney("-100.00"),
	)
	st.SetAutoPost(true)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Auto]") {
		t.Errorf("output should contain [Auto] indicator, got: %s", output)
	}
}

func TestScheduledList_AccountNotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "list", "--account", "Nope", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled list) with unknown account should return error")
	}
	if !strings.Contains(err.Error(), "Nope") || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention unknown account, got: %v", err)
	}
}

func TestScheduledCmd_HelpListsList(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `scheduled --help` to list `list`; got:\n%s", stdout.String())
	}
}

func TestScheduledList_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `scheduled list --help` to describe the command; got:\n%s", stdout.String())
	}
}
