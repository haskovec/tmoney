package security_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/db"
)

func TestSecurityAdd_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"security", "add",
		"--ticker", "AAPL",
		"--name", "Apple",
		"--type", "stock",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityAdd_MissingTicker(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"security", "add",
		"--file", "/fake.tdb",
		"--name", "Apple",
		"--type", "stock",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) without --ticker should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "ticker") {
		t.Errorf("expected Cobra required-flag error mentioning ticker, got: %v", err)
	}
}

func TestSecurityAdd_MissingName(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"security", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--type", "stock",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) without --name should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "name") {
		t.Errorf("expected Cobra required-flag error mentioning name, got: %v", err)
	}
}

func TestSecurityAdd_MissingType(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"security", "add",
		"--file", "/fake.tdb",
		"--ticker", "AAPL",
		"--name", "Apple",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) without --type should return error")
	}
	if !strings.Contains(err.Error(), "required flag") || !strings.Contains(err.Error(), "type") {
		t.Errorf("expected Cobra required-flag error mentioning type, got: %v", err)
	}
}

func TestSecurityAdd_InvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"security", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple",
		"--type", "invalid_type",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) with invalid --type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("expected error to mention 'invalid --type', got: %v", err)
	}
}

func TestSecurityAdd_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"security", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple Inc.",
		"--type", "stock",
		"--asset-class", "large_cap_stock",
		"--currency", "USD",
		"--exchange", "NASDAQ",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(security add): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	for _, want := range []string{
		"Security created successfully",
		"AAPL",
		"Apple Inc.",
		"NASDAQ",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got:\n%s", want, output)
		}
	}

	// Verify security is persisted by listing via the Cobra command.
	stdout.Reset()
	if err := cli.ExecuteWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security list): %v", err)
	}
	if !strings.Contains(stdout.String(), "AAPL") {
		t.Error("security should be persisted and visible in list")
	}
}

func TestSecurityAdd_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"security", "add",
		"--file", dbPath,
		"--ticker", "GOOG",
		"--name", "Alphabet Inc.",
		"--type", "stock",
	}, stdout, stderr)
	if err != nil {
		t.Fatalf("cli.ExecuteWith(security add): %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "USD") {
		t.Error("default currency should be USD")
	}
	if !strings.Contains(output, "Unclassified") {
		t.Error("default asset class should be Unclassified")
	}
}

func TestSecurityAdd_InvalidAssetClass(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = cli.ExecuteWith([]string{
		"security", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple Inc.",
		"--type", "stock",
		"--asset-class", "not_a_real_class",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("cli.ExecuteWith(security add) with invalid --asset-class should error")
	}
	if !strings.Contains(err.Error(), "invalid --asset-class") {
		t.Errorf("expected error to mention 'invalid --asset-class', got: %v", err)
	}
}

func TestSecurityAdd_Duplicate(t *testing.T) {
	dbPath, _ := clitest.CreateTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"security", "add",
		"--file", dbPath,
		"--ticker", "AAPL",
		"--name", "Apple Again",
		"--type", "stock",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("adding duplicate ticker should return error")
	}
}

func TestSecurityAdd_Help(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "add", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security add --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `security add --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsAdd(t *testing.T) {
	restore := cli.SwapTUILauncher(func(string) error { return nil })
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("cli.ExecuteWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "add") {
		t.Errorf("expected `security --help` to list `add`; got:\n%s", stdout.String())
	}
}
