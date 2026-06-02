package price_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestPriceAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--ticker", "AAPL",
		"--date", "2024-01-15",
		"--price", "150.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestPriceAdd_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", "/fake.tdb",
		"--date", "2024-01-15",
		"--price", "150.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestPriceAdd_MissingDate(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--price", "150.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) without --date should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "date") {
		t.Errorf("expected Cobra required-flag error mentioning date, got: %v", err)
	}
}

func TestPriceAdd_MissingPrice(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--date", "2024-01-15",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) without --price or --fetch should return error")
	}
	if !strings.Contains(err.Error(), "--price") || !strings.Contains(err.Error(), "--fetch") {
		t.Errorf("expected an error mentioning --price and --fetch, got: %v", err)
	}
}

func TestPriceAdd_Success(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--date", "2024-01-15",
		"--price", "150.00",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(price add): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"Price added", "AAPL", "2024-01-15", "150.00"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	// Verify the price is persisted by listing prices.
	stdout.Reset()
	if err := cli.ExecuteWith([]string{"price", "list", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price list AAPL): %v", err)
	}
	if !strings.Contains(stdout.String(), "150.00") {
		t.Error("price should be visible in `price list` listing")
	}
}

func TestPriceAdd_SecurityNotFound(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", dbPath,
		"--ticker", "ZZZZ",
		"--date", "2024-01-15",
		"--price", "150.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}

func TestPriceAdd_DuplicateConflict(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--date", "2024-01-15",
		"--price", "155.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) with duplicate date should return error")
	}
}

func TestPriceAdd_InvalidDate(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--date", "not-a-date",
		"--price", "150.00",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected error to mention 'invalid --date', got: %v", err)
	}
}

func TestPriceAdd_InvalidPrice(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--date", "2024-01-15",
		"--price", "not-a-number",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price add) with invalid price should return error")
	}
	if !strings.Contains(err.Error(), "invalid --price") {
		t.Errorf("expected error to mention 'invalid --price', got: %v", err)
	}
}

func TestPriceAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `price add --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestPriceCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `price --help` to list `add`; got:\n%s", stdout.String())
	}
}
