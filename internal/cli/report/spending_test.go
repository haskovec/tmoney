package report_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReportSpending_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-01"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestReportSpending_MissingPeriod(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending) without period should return error")
	}
	if !strings.Contains(err.Error(), "requires --month") {
		t.Errorf("expected 'requires --month' in error, got: %v", err)
	}
}

func TestReportSpending_ByMonth(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending --month): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "January 2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_ByYear(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--year", "2024", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending --year): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_ByDateRange(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--from", "2024-01-01", "--to", "2024-06-30", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending --from --to): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "2024-01-01 to 2024-06-30"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_WithData(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("5000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	catRepo := category.NewRepository(database)
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("-150.00"))
	txn.SetCategory(groceries.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("setup: create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"Groceries", "$150.00", "100.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_IncludeTransfersFlag(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("5000.00"), types.MustParseDate("2024-01-01"))
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD",
		types.MustNewMoney("0.00"), types.MustParseDate("2024-01-01"))
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("setup: create savings: %v", err)
	}

	catRepo := category.NewRepository(database)
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("setup: create groceries: %v", err)
	}
	cardPay := category.NewCategory("Card Payment", category.TypeExpense)
	if err := catRepo.Create(cardPay); err != nil {
		t.Fatalf("setup: create card payment: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	// A normal expense.
	grocTxn := transaction.NewTransaction(checking.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("-150.00"))
	grocTxn.SetCategory(groceries.ID)
	if err := txnRepo.Create(grocTxn); err != nil {
		t.Fatalf("setup: create groceries txn: %v", err)
	}
	// A categorized transfer pair (the legacy `transfer link` shape).
	transferID := types.NewID()
	out := transaction.NewTransaction(checking.ID, types.MustParseDate("2024-01-20"), types.MustNewMoney("-500.00"))
	out.SetCategory(cardPay.ID)
	out.SetTransfer(transferID, savings.ID)
	if err := txnRepo.Create(out); err != nil {
		t.Fatalf("setup: create transfer outflow: %v", err)
	}
	in := transaction.NewTransaction(savings.ID, types.MustParseDate("2024-01-20"), types.MustNewMoney("500.00"))
	in.SetCategory(cardPay.ID)
	in.SetTransfer(transferID, checking.ID)
	if err := txnRepo.Create(in); err != nil {
		t.Fatalf("setup: create transfer inflow: %v", err)
	}
	database.Close()

	// Default: transfer excluded, only groceries counts.
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending): %v\nstderr=%s", err, stderr)
	}
	out1 := stdout.String()
	if !strings.Contains(out1, "Groceries") {
		t.Errorf("default report should list Groceries; got:\n%s", out1)
	}
	if strings.Contains(out1, "Card Payment") {
		t.Errorf("default report must exclude the categorized transfer; got:\n%s", out1)
	}

	// With --include-transfers: the transfer folds in (outflow leg only).
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-01", "--include-transfers", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending --include-transfers): %v\nstderr=%s", err, stderr)
	}
	out2 := stdout.String()
	for _, want := range []string{"Groceries", "Card Payment", "$500.00"} {
		if !strings.Contains(out2, want) {
			t.Errorf("expected %q in --include-transfers output, got:\n%s", want, out2)
		}
	}
}

func TestReportSpending_InvalidMonthFormat(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "--month", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending --month=invalid) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --month format") {
		t.Errorf("expected 'invalid --month format' in error, got: %v", err)
	}
}

func TestReportSpending_InvalidMonthValue(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "--month", "2024-13", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending --month=2024-13) should return error")
	}
	if !strings.Contains(err.Error(), "month must be between 1 and 12") {
		t.Errorf("expected 'month must be between 1 and 12' in error, got: %v", err)
	}
}

func TestReportSpending_InvalidFromDate(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "--from", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending --from=not-a-date) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from date") {
		t.Errorf("expected 'invalid --from date' in error, got: %v", err)
	}
}

func TestReportSpending_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"report", "spending", "extra", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(report spending extra) should return error")
	}
}

func TestReportCmd_HelpListsSpending(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report --help): %v", err)
	}
	for _, want := range []string{"net-worth", "spending"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected `report --help` to list %q; got:\n%s", want, stdout.String())
		}
	}
}

func TestReportSpending_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"report", "spending", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(report spending --help): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"spending", "--month", "--year", "--from", "--to", "--include-transfers"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `report spending --help` to contain %q; got:\n%s", want, out)
		}
	}
}
