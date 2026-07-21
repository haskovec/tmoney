package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// reopenScheduled reopens the DB at path and returns a scheduled repository for
// verifying a command's persisted effect.
func reopenScheduled(t *testing.T, path string) (*db.DB, *scheduleddom.Repository) {
	t.Helper()
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	return database, scheduleddom.NewRepository(database)
}

// seedScheduled creates a single-line monthly schedule on a fresh Checking
// account and returns the db path plus the created schedule's ID.
func seedScheduled(t *testing.T, amount string) (string, types.ID) {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney(amount))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create scheduled transaction: %v", err)
	}
	id := st.ID
	database.Close()
	return dbPath, id
}

func TestScheduledEdit_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "edit", "--id", "x", "--amount", "-10"}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled edit without --file should return error")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledEdit_NoEditableFlag(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "edit", "--file", dbPath, "--id", id.String()}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled edit with no editable flag should return error")
	}
	if !strings.Contains(err.Error(), "at least one editable flag") {
		t.Errorf("expected at-least-one-flag error, got: %v", err)
	}
}

func TestScheduledEdit_UnknownID(t *testing.T) {
	dbPath, _ := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath,
		"--id", types.NewID().String(), "--amount", "-20",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled edit with unknown id should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestScheduledEdit_Amount(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--amount", "-42.50",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --amount: %v\nstderr=%s", err, stderr)
	}

	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	st, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !st.HasAmount() || st.Amount.Money.String() != types.MustNewMoney("-42.50").String() {
		t.Errorf("amount = %v, want -42.50", st.Amount)
	}
}

func TestScheduledEdit_ClearAmount(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--amount", "",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --amount \"\": %v\nstderr=%s", err, stderr)
	}

	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	st, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if st.HasAmount() {
		t.Errorf("amount should be cleared (variable), got %v", st.Amount)
	}
}

func TestScheduledEdit_Frequency(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--frequency", "weekly",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --frequency: %v\nstderr=%s", err, stderr)
	}

	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	st, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if st.Frequency != scheduleddom.FrequencyWeekly {
		t.Errorf("frequency = %s, want weekly", st.Frequency)
	}
}

func TestScheduledEdit_InvalidFrequency(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--frequency", "nope",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled edit with invalid frequency should return error")
	}
	if !strings.Contains(err.Error(), "invalid --frequency") {
		t.Errorf("expected invalid-frequency error listing valid values, got: %v", err)
	}
}

func TestScheduledEdit_NextDate(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--next-date", "2030-06-15",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --next-date: %v\nstderr=%s", err, stderr)
	}

	// The change is reflected in the list output.
	listOut, listErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, listOut, listErr); err != nil {
		t.Fatalf("scheduled list: %v", err)
	}
	if !strings.Contains(listOut.String(), "2030-06-15") {
		t.Errorf("list output should show new next date, got:\n%s", listOut.String())
	}
}

func TestScheduledEdit_Payee(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--payee", "Landlord",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --payee: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Landlord") {
		t.Errorf("summary should show new payee, got:\n%s", stdout.String())
	}

	// Clearing the payee.
	clr, clrErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--payee", "",
	}, clr, clrErr); err != nil {
		t.Fatalf("scheduled edit --payee \"\": %v\nstderr=%s", err, clrErr)
	}

	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	st, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if st.HasPayee() {
		t.Errorf("payee should be cleared, got %v", st.PayeeID)
	}
}

func TestScheduledEdit_Category(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Utilities", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}
	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create scheduled transaction: %v", err)
	}
	id := st.ID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--category", "Utilities",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --category: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenScheduled(t, dbPath)
	defer reDB.Close()
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.HasCategory() || got.CategoryID.ID != cat.ID {
		t.Errorf("category = %v, want %s", got.CategoryID, cat.ID)
	}
}

func TestScheduledEdit_MoveAccount(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}
	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(checking.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create scheduled transaction: %v", err)
	}
	id := st.ID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--account", "Savings",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled edit --account: %v\nstderr=%s", err, stderr)
	}

	reDB, repo := reopenScheduled(t, dbPath)
	defer reDB.Close()
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.AccountID != savings.ID {
		t.Errorf("account = %s, want savings %s", got.AccountID, savings.ID)
	}
}

func TestScheduledEdit_AutoPostToggle(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	// Enable.
	on, onErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--auto-post",
	}, on, onErr); err != nil {
		t.Fatalf("scheduled edit --auto-post: %v\nstderr=%s", err, onErr)
	}
	func() {
		database, repo := reopenScheduled(t, dbPath)
		defer database.Close()
		st, err := repo.GetByID(id)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if !st.AutoPost {
			t.Error("auto-post should be enabled")
		}
	}()

	// Disable.
	off, offErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--auto-post=false",
	}, off, offErr); err != nil {
		t.Fatalf("scheduled edit --auto-post=false: %v\nstderr=%s", err, offErr)
	}
	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	st, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if st.AutoPost {
		t.Error("auto-post should be disabled")
	}
}

func TestScheduledEdit_MultiLineRefused(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}

	// Build a multi-line template (two split lines summing to the parent amount).
	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-100.00"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create multi-line scheduled transaction: %v", err)
	}
	for _, amt := range []string{"-60.00", "-40.00"} {
		sp := scheduleddom.NewCategorizedSplit(st.ID, cat.ID, types.MustNewMoney(amt))
		if err := stRepo.SplitRepo().Create(sp); err != nil {
			t.Fatalf("setup: create split: %v", err)
		}
	}
	id := st.ID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--amount", "-120",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("editing a multi-line template should be refused")
	}
	if !strings.Contains(err.Error(), "multi-line") || !strings.Contains(err.Error(), "TUI") {
		t.Errorf("expected multi-line/TUI refusal, got: %v", err)
	}
}

func TestScheduledEdit_ClosedAccountRefused(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	acct := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}
	closed := account.NewAccount("Old Savings", account.TypeSavings, "USD", types.MustNewMoney("0.00"), types.Today())
	closed.Close(types.Today())
	if err := acctRepo.Create(closed); err != nil {
		t.Fatalf("setup: create closed account: %v", err)
	}

	stRepo := scheduleddom.NewRepository(database)
	st := scheduleddom.NewTransactionWithAmount(acct.ID, scheduleddom.FrequencyMonthly, types.Today(), types.MustNewMoney("-15.99"))
	if err := stRepo.Create(st); err != nil {
		t.Fatalf("setup: create scheduled transaction: %v", err)
	}
	id := st.ID
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "edit", "--file", dbPath, "--id", id.String(), "--account", "Old Savings",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("moving a schedule onto a closed account should be refused")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected closed-account refusal, got: %v", err)
	}

	// The schedule must still live on the original account.
	database, repo := reopenScheduled(t, dbPath)
	defer database.Close()
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.AccountID != acct.ID {
		t.Errorf("schedule moved to %s, want unchanged %s", got.AccountID, acct.ID)
	}
}
