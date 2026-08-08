package scheduled_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// seedTwoInvestmentAccounts creates two investment accounts and a category on a
// fresh database, and returns its path.
func seedTwoInvestmentAccounts(t *testing.T) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	acctRepo := account.NewRepository(database)
	for _, name := range []string{"Rollover IRA", "Roth IRA"} {
		acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
		if err := acctRepo.Create(acct); err != nil {
			t.Fatalf("setup: create %s: %v", name, err)
		}
	}
	// A bank account so the same category can be proved usable elsewhere.
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create Checking: %v", err)
	}
	catRepo := category.NewRepository(database)
	if err := catRepo.Create(category.NewCategory("Retirement", category.TypeExpense)); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}
	database.Close()
	return dbPath
}

// TestScheduledAdd_RefusesCategoryOnInvestmentToInvestmentTransfer pins the rule
// at the door it can be broken from. An investment↔investment pair keeps both
// legs in investment_transactions, which has no category column, so a schedule
// carrying one can never post — and in an auto-post batch its refusal used to
// abort every other schedule due that day.
func TestScheduledAdd_RefusesCategoryOnInvestmentToInvestmentTransfer(t *testing.T) {
	dbPath := seedTwoInvestmentAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Rollover IRA", "--frequency", "monthly",
		"--amount", "500", "--transfer-to", "Roth IRA",
		"--category", "Retirement",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("scheduled add accepted a category on an investment-to-investment transfer")
	}
	if !strings.Contains(err.Error(), "investment-to-investment") {
		t.Errorf("error = %v, want it to name the unsupported pair", err)
	}
}

// The same category on a pair that CAN hold one must still be accepted, or the
// guard is too broad.
func TestScheduledAdd_AllowsCategoryOnBankToInvestmentTransfer(t *testing.T) {
	dbPath := seedTwoInvestmentAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"scheduled", "add", "--file", dbPath,
		"--account", "Checking", "--frequency", "monthly",
		"--amount", "500", "--transfer-to", "Rollover IRA",
		"--category", "Retirement",
	}, stdout, stderr); err != nil {
		t.Fatalf("scheduled add on a bank-to-investment transfer: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Transfer to: Rollover IRA") {
		t.Errorf("expected the schedule to be created, got:\n%s", stdout.String())
	}
}
