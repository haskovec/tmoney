package loan_test

import (
	"strings"
	"testing"
)

func TestLoanList_MissingFile(t *testing.T) {
	_, err := runLoan(t, "loan", "list")
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Errorf("expected --file error, got: %v", err)
	}
}

func TestLoanList_Empty(t *testing.T) {
	dbPath := setupLoanDB(t)
	out, err := runLoan(t, "loan", "list", "--file", dbPath)
	if err != nil {
		t.Fatalf("loan list: %v", err)
	}
	if !strings.Contains(out, "No loan accounts found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestLoanList_ShowsLoanSummary(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking"); err != nil {
		t.Fatalf("loan add: %v", err)
	}

	out, err := runLoan(t, "loan", "list", "--file", dbPath)
	if err != nil {
		t.Fatalf("loan list: %v", err)
	}
	for _, want := range []string{"Mortgage", "312450.22", "6.5%", "2401.86", "2026-08-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("loan list missing %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "Showing 1 loan(s)") {
		t.Errorf("expected count footer; got:\n%s", out)
	}
}

// TestLoanList_NoScheduleShowsDashes creates a bare loan account (no loan-shaped
// schedule) and verifies the schedule-derived columns render as dashes.
func TestLoanList_NoScheduleShowsDashes(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "account", "add", "--file", dbPath,
		"--name", "Old Mortgage", "--type", "loan", "--interest-rate", "5",
		"--opening-balance", "-100000"); err != nil {
		t.Fatalf("account add: %v", err)
	}

	out, err := runLoan(t, "loan", "list", "--file", dbPath)
	if err != nil {
		t.Fatalf("loan list: %v", err)
	}
	if !strings.Contains(out, "Old Mortgage") {
		t.Errorf("expected the loan account in the list; got:\n%s", out)
	}
	if !strings.Contains(out, "100000") || !strings.Contains(out, "5%") {
		t.Errorf("expected balance owed and APR; got:\n%s", out)
	}
	if !strings.Contains(out, "—") {
		t.Errorf("expected dash placeholders for the schedule columns; got:\n%s", out)
	}
}
