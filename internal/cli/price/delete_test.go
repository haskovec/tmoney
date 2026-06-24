package price_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestPriceDelete_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--ticker", "AAPL",
		"--date", "2024-01-15",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestPriceDelete_MissingDate(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) without --date should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "date") {
		t.Errorf("expected Cobra required-flag error mentioning date, got: %v", err)
	}
}

func TestPriceDelete_NoSelector(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", dbPath,
		"--date", "2024-01-15",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) without a security selector should return error")
	}
	if !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected error to mention ticker selector, got: %v", err)
	}
}

func TestPriceDelete_InvalidDate(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--date", "not-a-date",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) with invalid date should return error")
	}
	if !strings.Contains(err.Error(), "invalid --date") {
		t.Errorf("expected error to mention 'invalid --date', got: %v", err)
	}
}

func TestPriceDelete_Success(t *testing.T) {
	// Fixture seeds AAPL prices on 2024-01-15 (150.00), 2024-02-15 (160.50),
	// and 2024-03-15 (170.25). Delete the middle one and confirm only it is gone.
	dbPath, _ := clitest.CreateTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--date", "2024-02-15",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(price delete): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{"Price deleted", "AAPL", "2024-02-15", "160.50"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	// Verify only the targeted price is gone; the others remain.
	stdout.Reset()
	if err := cli.ExecuteWith([]string{"price", "list", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price list AAPL): %v", err)
	}
	listing := stdout.String()
	if strings.Contains(listing, "160.50") {
		t.Errorf("deleted price 160.50 should be gone from listing, got:\n%s", listing)
	}
	for _, want := range []string{"150.00", "170.25"} {
		if !strings.Contains(listing, want) {
			t.Errorf("untouched price %q should still be present, got:\n%s", want, listing)
		}
	}
}

func TestPriceDelete_NoPriceOnDate(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--date", "2024-01-15",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) for a date with no price should return error")
	}
	if !strings.Contains(err.Error(), "no price recorded") {
		t.Errorf("expected error to mention 'no price recorded', got: %v", err)
	}
}

func TestPriceDelete_SecurityNotFound(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"price", "delete",
		"--file", dbPath,
		"--ticker", "ZZZZ",
		"--date", "2024-01-15",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price delete) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}

func TestPriceCmd_HelpListsDelete(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "delete") {
		t.Errorf("expected `price --help` to list `delete`; got:\n%s", stdout.String())
	}
}
