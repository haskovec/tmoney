package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

// seedActionsFixture builds a corporate-action test DB (Brokerage holding
// AAPL, plus a GOOG security) and records two actions via the CLI: a 4:1
// AAPL split dated 2024-01-15 and a GOOG→AAPL merger dated 2024-06-01.
// AAPL is therefore the subject of the split and the target of the merger.
func seedActionsFixture(t *testing.T) string {
	t.Helper()
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"investment", "split",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--ratio", "4:1",
		"--date", "2024-01-15",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed split failed: %v\nstderr: %s", err, stderr.String())
	}
	if err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "GOOG",
		"--target", "AAPL",
		"--exchange-ratio", "0.5",
		"--date", "2024-06-01",
	}, stdout, stderr); err != nil {
		t.Fatalf("seed merger failed: %v\nstderr: %s", err, stderr.String())
	}
	return dbPath
}

func TestInvestmentActions_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"investment", "actions"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment actions) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentActions_ListsNewestFirst(t *testing.T) {
	dbPath := seedActionsFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{"CORPORATE ACTIONS", "2024-01-15", "2024-06-01", "Stock Split", "Merger", "Ratio 4:1", "AAPL"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, out)
		}
	}
	// Newest first: the merger (2024-06-01) must appear before the split (2024-01-15).
	if strings.Index(out, "2024-06-01") > strings.Index(out, "2024-01-15") {
		t.Errorf("expected newest-first ordering, got:\n%s", out)
	}
}

func TestInvestmentActions_TickerFilter(t *testing.T) {
	dbPath := seedActionsFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions --ticker) failed: %v\nstderr: %s", err, stderr.String())
	}

	// AAPL is the split subject AND the merger target, so both rows appear.
	out := stdout.String()
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("expected AAPL split in filtered output, got:\n%s", out)
	}
	if !strings.Contains(out, "2024-06-01") {
		t.Errorf("expected merger targeting AAPL in filtered output, got:\n%s", out)
	}
}

func TestInvestmentActions_TypeFilter(t *testing.T) {
	dbPath := seedActionsFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
		"--type", "split",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions --type) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "2024-01-15") {
		t.Errorf("expected split row in filtered output, got:\n%s", out)
	}
	if strings.Contains(out, "2024-06-01") || strings.Contains(out, "Merger") {
		t.Errorf("expected --type split to hide the merger, got:\n%s", out)
	}
}

func TestInvestmentActions_InvalidType(t *testing.T) {
	dbPath := seedActionsFixture(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
		"--type", "bogus",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment actions --type bogus) should return error")
	}
	if !strings.Contains(err.Error(), "--type") {
		t.Errorf("expected error to mention --type, got: %v", err)
	}
}

func TestInvestmentActions_ShowIDs(t *testing.T) {
	dbPath := seedActionsFixture(t)

	svc := clitest.OpenSvc(t, dbPath)
	actions, err := svc.CorporateAction.ListAll()
	if err != nil {
		t.Fatalf("list corporate actions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("fixture produced no corporate actions")
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
		"--show-ids",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions --show-ids) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, ca := range actions {
		if !strings.Contains(out, ca.ID.String()) {
			t.Errorf("expected output to contain action ID %s, got:\n%s", ca.ID, out)
		}
	}
}

func TestInvestmentActions_NoIDsByDefault(t *testing.T) {
	dbPath := seedActionsFixture(t)

	svc := clitest.OpenSvc(t, dbPath)
	actions, err := svc.CorporateAction.ListAll()
	if err != nil {
		t.Fatalf("list corporate actions: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions) failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	for _, ca := range actions {
		if strings.Contains(out, ca.ID.String()) {
			t.Errorf("expected output to omit action IDs by default, found %s in:\n%s", ca.ID, out)
		}
	}
}

func TestInvestmentActions_Empty(t *testing.T) {
	dbPath := clitest.CreateInvestmentTestDB(t, false)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "actions",
		"--file", dbPath,
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions) on empty DB failed: %v\nstderr: %s", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "No corporate actions found.") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestInvestmentActions_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "actions", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment actions --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "actions") {
		t.Errorf("expected `investment actions --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsActions(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "actions") {
		t.Errorf("expected `investment --help` to list `actions`; got:\n%s", stdout.String())
	}
}
