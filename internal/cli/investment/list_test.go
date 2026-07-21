package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
)

// seedListFixture creates the standard investment test DB (Brokerage +
// AAPL + $50k cash) and records one buy and one dividend on known dates.
func seedListFixture(t *testing.T) string {
	t.Helper()
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "buy",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--shares", "10",
		"--amount", "1500",
		"--date", "2024-01-15",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed buy failed: %v", err)
	}
	if err := cli.ExecuteWith([]string{
		"investment", "dividend",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
		"--amount", "25.50",
		"--date", "2024-02-20",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed dividend failed: %v", err)
	}
	return dbPath
}

func TestInvestmentList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--account", "Brokerage",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentList_MissingAccount(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", "/fake.tdb",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment list) without --account should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "account") {
		t.Errorf("expected Cobra required-flag error mentioning account, got: %v", err)
	}
}

func TestInvestmentList_UnknownAccount(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Nope",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment list) with unknown account should return error")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("expected error to name the account, got: %v", err)
	}
}

func TestInvestmentList_ShowsTransactions(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"Brokerage", "2024-01-15", "2024-02-20", "AAPL", "Buy", "Dividend", "Deposit"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
	// Newest first: the dividend row must appear before the buy row.
	if strings.Index(out, "2024-02-20") > strings.Index(out, "2024-01-15") {
		t.Errorf("expected newest-first ordering, got:\n%s", out)
	}
}

func TestInvestmentList_TickerFilter(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list --ticker) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("expected AAPL buy in filtered output, got:\n%s", out)
	}
	// The cash deposit has no security and must be filtered out.
	if strings.Contains(out, "Deposit") {
		t.Errorf("expected --ticker filter to hide the cash deposit, got:\n%s", out)
	}
}

func TestInvestmentList_TypeFilter(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--type", "dividend",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list --type) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "2024-02-20") {
		t.Errorf("expected dividend row in filtered output, got:\n%s", out)
	}
	if strings.Contains(out, "2024-01-15") {
		t.Errorf("expected --type dividend to hide the buy, got:\n%s", out)
	}
}

func TestInvestmentList_InvalidType(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--type", "bogus",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment list --type bogus) should return error")
	}
	if !strings.Contains(err.Error(), "--type") {
		t.Errorf("expected error to mention --type, got: %v", err)
	}
}

func TestInvestmentList_DateRange(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--from", "2024-02-01",
		"--to", "2024-02-28",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list --from/--to) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "2024-02-20") {
		t.Errorf("expected in-range dividend, got:\n%s", out)
	}
	if strings.Contains(out, "2024-01-15") {
		t.Errorf("expected out-of-range buy to be hidden, got:\n%s", out)
	}
}

func TestInvestmentList_Limit(t *testing.T) {
	dbPath := seedListFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--limit", "1",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list --limit) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	// Newest row (the fixture deposit, dated today) survives; the
	// 2024-dated buy and dividend are cut.
	if !strings.Contains(out, "Deposit") {
		t.Errorf("expected newest row (deposit) with --limit 1, got:\n%s", out)
	}
	if strings.Contains(out, "2024-01-15") || strings.Contains(out, "2024-02-20") {
		t.Errorf("expected --limit 1 to drop the older rows, got:\n%s", out)
	}
}

func TestInvestmentList_ShowIDs(t *testing.T) {
	dbPath := seedListFixture(t)

	svc := clitest.OpenSvc(t, dbPath)
	acct, err := svc.Account.GetByName("Brokerage")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	rows, err := svc.InvestmentRepo.ListByAccount(acct.ID, investmentFilterAll())
	if err != nil {
		t.Fatalf("list investment txns: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("fixture produced no investment transactions")
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
		"--show-ids",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list --show-ids) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, row := range rows {
		if !strings.Contains(out, row.ID.String()) {
			t.Errorf("expected output to contain transaction ID %s, got:\n%s", row.ID, out)
		}
	}
}

func TestInvestmentList_NoIDsByDefault(t *testing.T) {
	dbPath := seedListFixture(t)

	svc := clitest.OpenSvc(t, dbPath)
	acct, err := svc.Account.GetByName("Brokerage")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	rows, err := svc.InvestmentRepo.ListByAccount(acct.ID, investmentFilterAll())
	if err != nil {
		t.Fatalf("list investment txns: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"investment", "list",
		"--file", dbPath,
		"--account", "Brokerage",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment list) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, row := range rows {
		if strings.Contains(out, row.ID.String()) {
			t.Errorf("expected output to omit transaction IDs by default, found %s in:\n%s", row.ID, out)
		}
	}
}

// investmentFilterAll returns an empty domain filter; kept as a helper so
// the tests read clearly at the call site.
func investmentFilterAll() investmentdom.TransactionFilter {
	return investmentdom.TransactionFilter{}
}
