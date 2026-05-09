package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPriceList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list AAPL) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestPriceList_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list) without ticker should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestPriceList_SecurityNotFound(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "ZZZZ", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list ZZZZ) with unknown ticker should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found, got: %v", err)
	}
}

func TestPriceList_ShowsAll(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price list AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"PRICES: AAPL",
		"2024-01-15", "2024-02-15", "2024-03-15",
		"150.00", "160.50", "170.25",
		"Manual", "Transaction", "Import",
		"Total: 3 price(s)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}
}

func TestPriceList_WithFromFilter(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath, "--from", "2024-02-01"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(price list AAPL --from): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if strings.Contains(output, "2024-01-15") {
		t.Error("output should not contain price before --from date")
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain price on/after --from date")
	}
	if !strings.Contains(output, "2024-03-15") {
		t.Error("output should contain price on/after --from date")
	}
	if !strings.Contains(output, "Total: 2 price(s)") {
		t.Error("output should contain total count of 2")
	}
}

func TestPriceList_WithToFilter(t *testing.T) {
	dbPath, _ := createTestDBWithSecurityAndPrices(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath, "--to", "2024-02-28"}, stdout, stderr)
	if err != nil {
		t.Fatalf("executeWith(price list AAPL --to): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "2024-01-15") {
		t.Error("output should contain price before --to date")
	}
	if !strings.Contains(output, "2024-02-15") {
		t.Error("output should contain price on/before --to date")
	}
	if strings.Contains(output, "2024-03-15") {
		t.Error("output should not contain price after --to date")
	}
	if !strings.Contains(output, "Total: 2 price(s)") {
		t.Error("output should contain total count of 2")
	}
}

func TestPriceList_NoPrices(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price list AAPL): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "No prices found") {
		t.Error("output should indicate no prices found")
	}
}

func TestPriceList_InvalidFromDate(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath, "--from", "not-a-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list AAPL --from not-a-date) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from") {
		t.Errorf("expected error mentioning 'invalid --from', got: %v", err)
	}
}

func TestPriceList_InvalidToDate(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL", "--file", dbPath, "--to", "not-a-date"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list AAPL --to not-a-date) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --to") {
		t.Errorf("expected error mentioning 'invalid --to', got: %v", err)
	}
}

func TestPriceList_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"price", "list", "AAPL", "EXTRA", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(price list AAPL EXTRA) should return error")
	}
}

func TestPriceList_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `price list --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestPriceCmd_HelpListsList(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `price --help` to list `list`; got:\n%s", stdout.String())
	}
}
