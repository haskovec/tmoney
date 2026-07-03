package loan_test

import (
	"strings"
	"testing"
)

func TestLoanShow_MissingFile(t *testing.T) {
	_, err := runLoan(t, "loan", "show", "Mortgage")
	if err == nil || !strings.Contains(err.Error(), "file") {
		t.Errorf("expected --file error, got: %v", err)
	}
}

func TestLoanShow_NotFound(t *testing.T) {
	dbPath := setupLoanDB(t)
	_, err := runLoan(t, "loan", "show", "--file", dbPath, "Nope")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestLoanShow_NotALoanAccount(t *testing.T) {
	dbPath := setupLoanDB(t)
	_, err := runLoan(t, "loan", "show", "--file", dbPath, "Checking")
	if err == nil || !strings.Contains(err.Error(), "not a loan") {
		t.Errorf("expected not-a-loan error, got: %v", err)
	}
}

func TestLoanShow_DetailsAndProjection(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking",
		"--escrow", "Housing:Property Tax=650"); err != nil {
		t.Fatalf("loan add: %v", err)
	}

	out, err := runLoan(t, "loan", "show", "--file", dbPath, "Mortgage")
	if err != nil {
		t.Fatalf("loan show: %v", err)
	}
	for _, want := range []string{
		"LOAN: Mortgage", "Balance owed:", "312450.22", "APR:", "6.5%",
		"P&I payment:", "2401.86", "Escrow:", "650", "Payments left:",
		"Payoff date:", "Interest remaining:",
		// First projection row: interest = round(312450.22*6.5/1200) = 1692.44.
		"1692.44",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("loan show missing %q; got:\n%s", want, out)
		}
	}
	// Default limit is 12, and this loan has far more rows → footer note.
	if !strings.Contains(out, "more payment") {
		t.Errorf("expected withheld-rows footer at default limit; got:\n%s", out)
	}
}

func TestLoanShow_AllShowsEveryRow(t *testing.T) {
	dbPath := setupLoanDB(t)
	// Short loan: 1200 @ 0% / 12mo → exactly 12 rows.
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Promo", "--principal", "1200", "--rate", "0", "--term-months", "12",
		"--next-payment-date", "2026-08-01", "--from-account", "Checking"); err != nil {
		t.Fatalf("loan add: %v", err)
	}

	out, err := runLoan(t, "loan", "show", "--file", dbPath, "Promo", "--all")
	if err != nil {
		t.Fatalf("loan show --all: %v", err)
	}
	if strings.Contains(out, "more payment") {
		t.Errorf("--all should not withhold rows; got:\n%s", out)
	}
	if !strings.Contains(out, "Payments left:      12") {
		t.Errorf("expected 12 payments; got:\n%s", out)
	}
}

func TestLoanShow_NoScheduleHint(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "account", "add", "--file", dbPath,
		"--name", "Old Mortgage", "--type", "loan", "--interest-rate", "5",
		"--opening-balance", "-100000"); err != nil {
		t.Fatalf("account add: %v", err)
	}

	out, err := runLoan(t, "loan", "show", "--file", dbPath, "Old Mortgage")
	if err != nil {
		t.Fatalf("loan show: %v", err)
	}
	if !strings.Contains(out, "No loan payment schedule") {
		t.Errorf("expected no-schedule hint; got:\n%s", out)
	}
	// It should still show the balance and APR it can compute.
	if !strings.Contains(out, "100000") || !strings.Contains(out, "5%") {
		t.Errorf("expected balance owed and APR in partial state; got:\n%s", out)
	}
}
