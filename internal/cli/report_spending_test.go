package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func TestReportSpending_MissingFile(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"report", "spending", "--month", "2024-01"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending) without --file should return error")
	}
	if !strings.Contains(err.Error(), "--file") && !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention --file/file, got: %v", err)
	}
}

func TestReportSpending_MissingPeriod(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"report", "spending", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending) without period should return error")
	}
	if !strings.Contains(err.Error(), "requires --month") {
		t.Errorf("expected 'requires --month' in error, got: %v", err)
	}
}

func TestReportSpending_ByMonth(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report spending --month): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "January 2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_ByYear(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "spending", "--year", "2024", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report spending --year): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "2024"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_ByDateRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "spending", "--from", "2024-01-01", "--to", "2024-06-30", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report spending --from --to): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"SPENDING BY CATEGORY", "2024-01-01 to 2024-06-30"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_WithData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}

	acctRepo := account.NewRepository(database)
	acct := account.NewAccount(
		"Checking",
		account.TypeChecking,
		"USD",
		types.MustNewMoney("5000.00"),
		types.MustParseDate("2024-01-01"),
	)
	if err := acctRepo.Create(acct); err != nil {
		t.Fatalf("setup: create account: %v", err)
	}

	catRepo := category.NewRepository(database)
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	if err := catRepo.Create(groceries); err != nil {
		t.Fatalf("setup: create category: %v", err)
	}

	txnRepo := transaction.NewRepository(database)
	txn := transaction.NewTransaction(acct.ID, types.MustParseDate("2024-01-15"), types.MustNewMoney("-150.00"))
	txn.SetCategory(groceries.ID)
	if err := txnRepo.Create(txn); err != nil {
		t.Fatalf("setup: create transaction: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "spending", "--month", "2024-01", "--file", dbPath}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report spending): %v\nstderr=%s", err, stderr)
	}

	out := stdout.String()
	for _, want := range []string{"Groceries", "$150.00", "100.0%"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestReportSpending_InvalidMonthFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"report", "spending", "--month", "invalid", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending --month=invalid) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --month format") {
		t.Errorf("expected 'invalid --month format' in error, got: %v", err)
	}
}

func TestReportSpending_InvalidMonthValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"report", "spending", "--month", "2024-13", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending --month=2024-13) should return error")
	}
	if !strings.Contains(err.Error(), "month must be between 1 and 12") {
		t.Errorf("expected 'month must be between 1 and 12' in error, got: %v", err)
	}
}

func TestReportSpending_InvalidFromDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("setup: db.Create: %v", err)
	}
	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err = executeWith([]string{"report", "spending", "--from", "not-a-date", "--file", dbPath}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending --from=not-a-date) should return error")
	}
	if !strings.Contains(err.Error(), "invalid --from date") {
		t.Errorf("expected 'invalid --from date' in error, got: %v", err)
	}
}

func TestReportSpending_RejectsExtraArgs(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := executeWith([]string{"report", "spending", "extra", "--file", "x.tdb"}, stdout, stderr)
	if err == nil {
		t.Fatal("executeWith(report spending extra) should return error")
	}
}

func TestReportCmd_HelpListsSpending(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report --help): %v", err)
	}
	for _, want := range []string{"net-worth", "spending"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("expected `report --help` to list %q; got:\n%s", want, stdout.String())
		}
	}
}

func TestParseYearMonth(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantYear  int
		wantMonth int
		wantErr   bool
	}{
		{"valid January", "2024-01", 2024, 1, false},
		{"valid December", "2024-12", 2024, 12, false},
		{"invalid format", "2024/01", 0, 0, true},
		{"missing month", "2024", 0, 0, true},
		{"invalid year", "abcd-01", 0, 0, true},
		{"invalid month", "2024-ab", 0, 0, true},
		{"month too low", "2024-00", 0, 0, true},
		{"month too high", "2024-13", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, err := parseYearMonth(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseYearMonth(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseYearMonth(%q) unexpected error: %v", tt.input, err)
				return
			}
			if year != tt.wantYear {
				t.Errorf("parseYearMonth(%q) year = %d, want %d", tt.input, year, tt.wantYear)
			}
			if month != tt.wantMonth {
				t.Errorf("parseYearMonth(%q) month = %d, want %d", tt.input, month, tt.wantMonth)
			}
		})
	}
}

func TestReportSpending_Help(t *testing.T) {
	_, restore := stubLaunchers(t)
	defer restore()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := executeWith([]string{"report", "spending", "--help"}, stdout, stderr); err != nil {
		t.Fatalf("executeWith(report spending --help): %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"spending", "--month", "--year", "--from", "--to"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected `report spending --help` to contain %q; got:\n%s", want, out)
		}
	}
}
