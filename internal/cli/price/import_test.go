package price_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
)

func TestPriceImport_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"price", "import", "prices.csv"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price import prices.csv) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestPriceImport_MissingCSVPath(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"price", "import", "--file", "/fake.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price import) without CSV path should return error")
	}
	if !strings.Contains(err.Error(), "arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}

func TestPriceImport_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"price", "import", "a.csv", "b.csv", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price import a.csv b.csv) should return error")
	}
}

func TestPriceImport_FileNotFound(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{"price", "import", "/nonexistent/prices.csv", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(price import) with nonexistent CSV should return error")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error should mention file open failure, got: %v", err)
	}
}

func TestPriceImport_Success(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-16,AAPL,152.50\n2024-01-17,AAPL,148.75\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "import", csvPath, "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE") {
		t.Error("output should contain IMPORT COMPLETE")
	}
	if !strings.Contains(output, "Imported:       3") {
		t.Errorf("output should show 3 imported, got: %s", output)
	}

	stdout.Reset()
	if err := cli.ExecuteWith([]string{"price", "list", "AAPL", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price list AAPL): %v", err)
	}
	priceOutput := stdout.String()
	for _, want := range []string{"150.00", "152.50", "148.75"} {
		if !strings.Contains(priceOutput, want) {
			t.Errorf("price %q should be in prices listing, got:\n%s", want, priceOutput)
		}
	}
}

func TestPriceImport_WithOverwrite(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "add", "--ticker", "AAPL", "--date", "2024-01-15", "--price", "150.00", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price add): %v", err)
	}

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,155.00\n2024-01-16,AAPL,160.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout.Reset()
	if err := cli.ExecuteWith([]string{"price", "import", csvPath, "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import): %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Skipped:        1") {
		t.Errorf("expected 1 skipped, got: %s", output)
	}
	if !strings.Contains(output, "Imported:       1") {
		t.Errorf("expected 1 imported, got: %s", output)
	}

	stdout.Reset()
	if err := cli.ExecuteWith([]string{"price", "import", csvPath, "--overwrite", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import --overwrite): %v", err)
	}
	output = stdout.String()
	if !strings.Contains(output, "Imported:       2") {
		t.Errorf("expected 2 imported with overwrite, got: %s", output)
	}
	if !strings.Contains(output, "Skipped:        0") {
		t.Errorf("expected 0 skipped with overwrite, got: %s", output)
	}
}

func TestPriceImport_UnknownTicker(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-15,ZZZZ,99.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "import", csvPath, "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import): %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "unknown ticker") {
		t.Error("output should warn about unknown ticker")
	}
	if !strings.Contains(output, "ZZZZ") {
		t.Error("output should mention the unknown ticker ZZZZ")
	}
	if !strings.Contains(output, "Imported:       1") {
		t.Errorf("expected 1 imported (AAPL only), got: %s", output)
	}
	if !strings.Contains(output, "Unknown ticker: 1") {
		t.Errorf("expected 1 unknown ticker in summary, got: %s", output)
	}
}

func TestPriceImport_DisplaysSummary(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "prices.csv")
	csvContent := "Date,Ticker,Price\n2024-01-15,AAPL,150.00\n2024-01-16,AAPL,152.00\n"
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write test CSV: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "import", csvPath, "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import): %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "IMPORT COMPLETE: prices.csv") {
		t.Error("output should show import complete with filename")
	}
	for _, want := range []string{"Total rows:", "Imported:", "Skipped:"} {
		if !strings.Contains(output, want) {
			t.Errorf("output should contain %q, got:\n%s", want, output)
		}
	}
}

func TestPriceImport_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "import", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price import --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "import") {
		t.Errorf("expected `price import --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestPriceCmd_HelpListsImport(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"price", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(price --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "import") {
		t.Errorf("expected `price --help` to list `import`; got:\n%s", stdout.String())
	}
}
