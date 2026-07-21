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
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestScheduledDelete_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "delete", "some-id"}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled delete without --file should return error")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestScheduledDelete_UnknownID(t *testing.T) {
	dbPath, _ := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"scheduled", "delete", types.NewID().String(), "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled delete with unknown id should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestScheduledDelete_HappyPath(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "delete", id.String(), "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("scheduled delete: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "deleted successfully") {
		t.Errorf("expected success message, got:\n%s", stdout.String())
	}

	// Gone from the list.
	listOut, listErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "list", "--file", dbPath}, listOut, listErr); err != nil {
		t.Fatalf("scheduled list: %v", err)
	}
	if !strings.Contains(listOut.String(), "No scheduled transactions found") {
		t.Errorf("schedule should be gone from list, got:\n%s", listOut.String())
	}
}

func TestScheduledDelete_MultiLineDeletable(t *testing.T) {
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
	if err := cli.ExecuteWith([]string{"scheduled", "delete", id.String(), "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("scheduled delete (multi-line): %v\nstderr=%s", err, stderr)
	}

	database2, repo := reopenScheduled(t, dbPath)
	defer database2.Close()
	if _, err := repo.GetByID(id); err == nil {
		t.Error("multi-line template should be gone after delete")
	}
}

func TestScheduledDelete_PostedHistoryUntouched(t *testing.T) {
	dbPath, id := seedScheduled(t, "-15.99")

	// Post once so a real transaction exists.
	postOut, postErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "post", id.String(), "--file", dbPath}, postOut, postErr); err != nil {
		t.Fatalf("scheduled post: %v\nstderr=%s", err, postErr)
	}

	// Delete the template.
	delOut, delErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "delete", id.String(), "--file", dbPath}, delOut, delErr); err != nil {
		t.Fatalf("scheduled delete: %v\nstderr=%s", err, delErr)
	}

	// The posted transaction still exists on the account.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer database.Close()
	acctRepo := account.NewRepository(database)
	acct, err := acctRepo.GetByName("Checking")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	txnRepo := transaction.NewRepository(database)
	txns, err := txnRepo.ListByAccount(acct.ID)
	if err != nil {
		t.Fatalf("list transactions: %v", err)
	}
	if len(txns) != 1 {
		t.Errorf("expected 1 posted transaction to survive template delete, got %d", len(txns))
	}
}
