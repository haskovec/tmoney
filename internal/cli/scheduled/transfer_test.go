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

// seedTwoAccounts creates Checking + Savings on a fresh DB and returns the path.
func seedTwoAccounts(t *testing.T) string {
	t.Helper()
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
	database.Close()
	return dbPath
}

func TestScheduledAdd_TransferTo(t *testing.T) {
	dbPath := seedTwoAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "250", "--transfer-to", "Savings",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled add --transfer-to: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer to: Savings") {
		t.Errorf("summary should show transfer destination, got:\n%s", stdout.String())
	}

	// Persisted as a transfer schedule with a negative stored amount.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer database.Close()
	stRepo := scheduleddom.NewRepository(database)
	all, err := stRepo.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(all))
	}
	st := all[0]
	if !st.IsTransfer() {
		t.Error("schedule should be a transfer")
	}
	if !st.HasAmount() || !st.Amount.Money.IsNegative() {
		t.Errorf("transfer amount should be stored negative, got %v", st.Amount)
	}
}

func TestScheduledAdd_TransferTo_PayeeConflict(t *testing.T) {
	dbPath := seedTwoAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "250", "--transfer-to", "Savings", "--payee", "Landlord",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("--transfer-to with --payee should return error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected mutual-exclusivity error, got: %v", err)
	}
}

func TestScheduledAdd_TransferTo_SelfTransfer(t *testing.T) {
	dbPath := seedTwoAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "250", "--transfer-to", "Checking",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("--transfer-to same account should return error")
	}
	if !strings.Contains(err.Error(), "same account") {
		t.Errorf("expected self-transfer error, got: %v", err)
	}
}

func TestScheduledAdd_TransferTo_WithCategory(t *testing.T) {
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
	catRepo := category.NewRepository(database)
	cat := category.NewCategory("Savings Goal", category.TypeExpense)
	if err := catRepo.Create(cat); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "250", "--transfer-to", "Savings", "--category", "Savings Goal",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled add --transfer-to --category: %v\nstderr=%s", err, stderr)
	}

	reDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer reDB.Close()
	stRepo := scheduleddom.NewRepository(reDB)
	all, err := stRepo.List()
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(all) != 1 || !all[0].IsTransfer() || !all[0].HasCategory() {
		t.Fatalf("expected a categorized transfer schedule, got %+v", all)
	}
	if all[0].CategoryID.ID != cat.ID {
		t.Errorf("category = %v, want %s", all[0].CategoryID, cat.ID)
	}
}

func TestScheduledAdd_TransferTo_PostCreatesLinkedPair(t *testing.T) {
	dbPath := seedTwoAccounts(t)

	addOut, addErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "250", "--transfer-to", "Savings",
	}, addOut, addErr); err != nil {
		t.Fatalf("scheduled add --transfer-to: %v\nstderr=%s", err, addErr)
	}

	// Find the schedule's ID.
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	stRepo := scheduleddom.NewRepository(database)
	all, err := stRepo.List()
	if err != nil || len(all) != 1 {
		t.Fatalf("list schedules: %v (len=%d)", err, len(all))
	}
	id := all[0].ID
	database.Close()

	postOut, postErr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"scheduled", "post", id.String(), "--file", dbPath}, postOut, postErr); err != nil {
		t.Fatalf("scheduled post (transfer): %v\nstderr=%s", err, postErr)
	}

	// Both legs exist and their amounts cancel.
	reDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer reDB.Close()
	acctRepo := account.NewRepository(reDB)
	txnRepo := transaction.NewRepository(reDB)

	checking, err := acctRepo.GetByName("Checking")
	if err != nil {
		t.Fatalf("get checking: %v", err)
	}
	savings, err := acctRepo.GetByName("Savings")
	if err != nil {
		t.Fatalf("get savings: %v", err)
	}
	fromTxns, err := txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list from-leg: %v", err)
	}
	toTxns, err := txnRepo.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("list to-leg: %v", err)
	}
	if len(fromTxns) != 1 || len(toTxns) != 1 {
		t.Fatalf("expected one leg on each account, got from=%d to=%d", len(fromTxns), len(toTxns))
	}
	if !fromTxns[0].IsTransfer() || !toTxns[0].IsTransfer() {
		t.Error("both legs should be transfer transactions")
	}
	sum := fromTxns[0].Amount.Add(toTxns[0].Amount)
	if !sum.IsZero() {
		t.Errorf("transfer legs should cancel, got sum %v (from=%v to=%v)", sum, fromTxns[0].Amount, toTxns[0].Amount)
	}
}
