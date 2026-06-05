package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestScheduledPost_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", "abc123"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledPost_MissingID(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) without ID arg should return error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestScheduledPost_InvalidID(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", "invalid-uuid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) with invalid ID should return error")
	}
	if !strings.Contains(err.Error(), "invalid scheduled transaction ID") {
		t.Errorf("error should mention invalid ID, got: %v", err)
	}
}

func TestScheduledPost_NotFound(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", "00000000-0000-0000-0000-000000000000", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) with nonexistent ID should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestScheduledPost_Success(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

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

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	st.SetPayee(py.ID)
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", stID, "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled post): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"posted successfully", "Checking", "Netflix", "-$15.99", "Monthly", "Next:"} {
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
	if len(txns) != 1 {
		t.Errorf("expected 1 transaction, got %d", len(txns))
	}
}

func TestScheduledPost_WithCustomAmount(t *testing.T) {
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
	stID := st.ID.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", stID, "--amount", "-25.00", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled post --amount): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "-$25.00") {
		t.Errorf("output should contain custom amount, got:\n%s", output)
	}
}

func TestScheduledPost_WithCustomDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-100.00"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", stID, "--date", "2025-06-15", "--file", dbPath}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled post --date): %v\nstderr=%s", err, stderr)
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
	if len(txns) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txns))
	}
	if got := txns[0].Date.String(); got != "2025-06-15" {
		t.Errorf("expected posted date 2025-06-15, got %s", got)
	}
}

func TestScheduledPost_InvalidDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", stID, "--date", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "--date") {
		t.Errorf("error should mention --date, got: %v", err)
	}
}

func TestScheduledPost_InvalidAmount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-50.00"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("failed to create scheduled transaction: %v", err)
	}
	stID := st.ID.String()

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "post", stID, "--amount", "not-a-number", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(scheduled post) with invalid amount should return error")
	}
	if !strings.Contains(err.Error(), "--amount") {
		t.Errorf("error should mention --amount, got: %v", err)
	}
}

func TestScheduledPost_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "post", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled post --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "post") {
		t.Errorf("expected `scheduled post --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestScheduledCmd_HelpListsPost(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(scheduled --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "post") {
		t.Errorf("expected `scheduled --help` to list `post`; got:\n%s", stdout.String())
	}
}
