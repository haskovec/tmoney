package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/security"
)

func TestSecurityList_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "list"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security list) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestSecurityList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list): %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "No securities found") {
		t.Errorf("expected 'No securities found', got: %s", stdout.String())
	}
}

func TestSecurityList_WithData(t *testing.T) {
	dbPath, _ := createTestDBWithSecurity(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SECURITIES", "AAPL", "Apple Inc.", "Stock", "Large Cap Stock"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

func TestSecurityList_ExcludesHiddenByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := security.NewRepository(database)
	sec := security.NewSecurity("MSFT", "Microsoft Corp.", security.TypeStock)
	sec.Hide()
	if err := repo.Create(sec); err != nil {
		t.Fatalf("setup: create hidden security: %v", err)
	}
	database.Close()

	// Without --include-hidden
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list): %v", err)
	}
	if strings.Contains(stdout.String(), "MSFT") {
		t.Error("hidden security should not appear without --include-hidden")
	}

	// With --include-hidden
	stdout.Reset()
	if err := executeWith([]string{"security", "list", "--file", dbPath, "--include-hidden"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list --include-hidden): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "MSFT") {
		t.Error("hidden security should appear with --include-hidden")
	}
	if !strings.Contains(out, "[hidden]") {
		t.Error("output should indicate hidden status")
	}
}

func TestSecurityList_FilterByType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := security.NewRepository(database)
	stock := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := repo.Create(stock); err != nil {
		t.Fatalf("setup: create stock: %v", err)
	}
	etf := security.NewSecurity("SPY", "SPDR S&P 500 ETF", security.TypeETF)
	if err := repo.Create(etf); err != nil {
		t.Fatalf("setup: create etf: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--file", dbPath, "--type", "etf"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list --type etf): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "SPY") {
		t.Error("output should contain ETF when filtering by etf")
	}
	if strings.Contains(out, "AAPL") {
		t.Error("output should not contain stock when filtering by etf")
	}
}

func TestSecurityList_InvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "list", "--file", dbPath, "--type", "bogus_type"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security list) with invalid --type should return error")
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("expected error to mention 'invalid --type', got: %v", err)
	}
}

func TestSecurityList_FilterByAssetClass(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	repo := security.NewRepository(database)
	stock := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	stock.AssetClass = security.AssetClassLargeCapStock
	if err := repo.Create(stock); err != nil {
		t.Fatalf("setup: create stock: %v", err)
	}
	bond := security.NewSecurity("BND", "Vanguard Bond ETF", security.TypeETF)
	bond.AssetClass = security.AssetClassDomesticBond
	if err := repo.Create(bond); err != nil {
		t.Fatalf("setup: create bond: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--file", dbPath, "--asset-class", "domestic_bond"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list --asset-class): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "BND") {
		t.Error("output should contain bond ETF")
	}
	if strings.Contains(out, "AAPL") {
		t.Error("output should not contain stock when filtering by domestic_bond")
	}
}

func TestSecurityList_InvalidAssetClass(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"security", "list", "--file", dbPath, "--asset-class", "not_a_real_class"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security list) with invalid --asset-class should return error")
	}
	if !strings.Contains(err.Error(), "invalid --asset-class") {
		t.Errorf("expected error to mention 'invalid --asset-class', got: %v", err)
	}
}

func TestSecurityList_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"security", "list", "--file", "x.tdb", "extra"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(security list ... extra) should return error")
	}
}

func TestSecurityList_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "list", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security list --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `security list --help` to describe the command; got:\n%s", stdout.String())
	}
}

func TestSecurityCmd_HelpListsList(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"security", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(security --help): %v", err)
	}
	if !strings.Contains(stdout.String(), "list") {
		t.Errorf("expected `security --help` to list `list`; got:\n%s", stdout.String())
	}
}
