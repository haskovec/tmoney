package investment_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestInvestmentSpinOff_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") {
		t.Errorf("expected error to mention --file, got: %v", err)
	}
}

func TestInvestmentSpinOff_MissingParent(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --parent should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "parent") {
		t.Errorf("expected Cobra required-flag error mentioning parent, got: %v", err)
	}
}

func TestInvestmentSpinOff_MissingSpinoff(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --spinoff should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "spinoff") {
		t.Errorf("expected Cobra required-flag error mentioning spinoff, got: %v", err)
	}
}

func TestInvestmentSpinOff_MissingShareRatio(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --share-ratio should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "share-ratio") {
		t.Errorf("expected Cobra required-flag error mentioning share-ratio, got: %v", err)
	}
}

func TestInvestmentSpinOff_MissingParentAllocation(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--spin-off-price", "25",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --parent-allocation should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "parent-allocation") {
		t.Errorf("expected Cobra required-flag error mentioning parent-allocation, got: %v", err)
	}
}

func TestInvestmentSpinOff_MissingPrice(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(investment spin-off) without --spin-off-price should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "spin-off-price") {
		t.Errorf("expected Cobra required-flag error mentioning spin-off-price, got: %v", err)
	}
}

func TestInvestmentSpinOff_InvalidShareRatio(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "abc",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --share-ratio") {
		t.Errorf("expected invalid share-ratio error, got: %v", err)
	}
}

func TestInvestmentSpinOff_InvalidParentAllocation(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "xyz",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --parent-allocation") {
		t.Errorf("expected invalid parent-allocation error, got: %v", err)
	}
}

func TestInvestmentSpinOff_InvalidPrice(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "abc",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --spin-off-price") {
		t.Errorf("expected invalid spin-off-price error, got: %v", err)
	}
}

func TestInvestmentSpinOff_InvalidDate(t *testing.T) {
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", "test.tdb",
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
		"--date", "not-a-date",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected invalid date error, got: %v", err)
	}
}

func TestInvestmentSpinOff_ParentNotFound(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "ZZZZ",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected parent not found error, got: %v", err)
	}
}

func TestInvestmentSpinOff_ChildNotFound(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "AAPL",
		"--spinoff", "ZZZZ",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected spin-off security not found error, got: %v", err)
	}
}

func TestInvestmentSpinOff_Basic(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment spin-off) returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Spin-off applied successfully") {
		t.Error("output should confirm spin-off")
	}
	if !strings.Contains(output, "AAPL") {
		t.Error("output should contain parent ticker")
	}
	if !strings.Contains(output, "GOOG") {
		t.Error("output should contain spin-off ticker")
	}
	if !strings.Contains(output, "0.5") {
		t.Error("output should contain share ratio")
	}
	if !strings.Contains(output, "80%") {
		t.Error("output should contain parent allocation")
	}
	if !strings.Contains(output, "Action ID") {
		t.Error("output should contain action ID")
	}
}

func TestInvestmentSpinOff_WithDate(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, false, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
		"--date", "2025-03-15",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment spin-off with date) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "2025-03-15") {
		t.Error("output should contain the specified date")
	}
}

func TestInvestmentSpinOff_WithLotTracking(t *testing.T) {
	dbPath := clitest.CreateCorporateActionTestDB(t, true, true)

	stdout := &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"investment", "spin-off",
		"--file", dbPath,
		"--parent", "AAPL",
		"--spinoff", "GOOG",
		"--share-ratio", "0.5",
		"--parent-allocation", "80",
		"--spin-off-price", "25",
	}, stdout, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("cli.ExecuteWith(investment spin-off lot-tracking) returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Spin-off applied successfully") {
		t.Error("output should confirm spin-off for lot-tracking account")
	}
}

func TestInvestmentSpinOff_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "spin-off", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment spin-off --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "spin-off") {
		t.Errorf("expected `investment spin-off --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestInvestmentCmd_HelpListsSpinOff(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"investment", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(investment --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "spin-off") {
		t.Errorf("expected `investment --help` to list `spin-off`; got:\n%s", stdout.String())
	}
}
