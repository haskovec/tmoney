package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestInvestmentMerge_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment merge) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentMerge_MissingSource(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", "test.tdb",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment merge) without --source should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "source") {
		t.Errorf("expected Cobra required-flag error mentioning source, got: %v", err)
	}
}

func TestInvestmentMerge_MissingTarget(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", "test.tdb",
		"--source", "AAPL",
		"--exchange-ratio", "0.5",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment merge) without --target should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "target") {
		t.Errorf("expected Cobra required-flag error mentioning target, got: %v", err)
	}
}

func TestInvestmentMerge_MissingRatio(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", "test.tdb",
		"--source", "AAPL",
		"--target", "GOOG",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment merge) without --exchange-ratio should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "exchange-ratio") {
		t.Errorf("expected Cobra required-flag error mentioning exchange-ratio, got: %v", err)
	}
}

func TestInvestmentMerge_InvalidRatio(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "not-a-number",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --exchange-ratio") {
		t.Errorf("expected invalid exchange ratio error, got: %v", err)
	}
}

func TestInvestmentMerge_InvalidCashPerShare(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", "test.tdb",
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--cash-per-share", "abc",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --cash-per-share") {
		t.Errorf("expected invalid cash-per-share error, got: %v", err)
	}
}

func TestInvestmentMerge_InvalidDate(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", "test.tdb",
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

func TestInvestmentMerge_SourceNotFound(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "ZZZZ",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected source not found error, got: %v", err)
	}
}

func TestInvestmentMerge_TargetNotFound(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "ZZZZ",
		"--exchange-ratio", "0.5",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected target not found error, got: %v", err)
	}
}

func TestInvestmentMerge_Basic(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment merge) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain source ticker")
	}
	if !strings.Contains(output, "GOOG") {
		t.Error("output should contain target ticker")
	}
	if !strings.Contains(output, "0.5") {
		t.Error("output should contain exchange ratio")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestInvestmentMerge_WithCashPerShare(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--cash-per-share", "10.50",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment merge with cash) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger")
	}
	if !strings.Contains(output, "Cash/Share") {
		t.Error("output should show cash per share")
	}
}

func TestInvestmentMerge_WithDate(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
		"--date", "2025-06-01",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment merge with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-06-01") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentMerge_WithLotTracking(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, true, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "merge",
		"--file", dbPath,
		"--source", "AAPL",
		"--target", "GOOG",
		"--exchange-ratio", "0.5",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment merge lot-tracking) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Merger applied successfully") {
		t.Error("output should confirm merger for lot-tracking account")
	}
}

func TestInvestmentMerge_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "merge", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment merge --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "merge") {
		t.Errorf("expected `investment merge --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsMerge(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "merge") {
		t.Errorf("expected `investment --help` to list `merge`; got:\n%s", stdout.String())
	}
}
