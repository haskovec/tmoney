package db

import "testing"

// TestMigration036 deletes the reconciliation sessions of investment accounts.
// Reconcile reads only the register ledger, so such a session never matched
// anything; once Start refuses these accounts, an open one could never be
// cancelled either (cancel lives inside the reconcile view).
func TestMigration036(t *testing.T) {
	database := openAtVersion(t, 35)

	accounts := []struct{ id, typ string }{
		{"00000000-0000-7000-8000-00000000000a", "investment"},
		{"00000000-0000-7000-8000-00000000000b", "hsa_investment"},
		{"00000000-0000-7000-8000-00000000000c", "checking"},
		{"00000000-0000-7000-8000-00000000000d", "hsa"},
	}
	for _, a := range accounts {
		if _, err := database.Conn().Exec(
			`INSERT INTO accounts (id, name, type, opening_date) VALUES (?, ?, ?, DATE '2024-01-01')`,
			a.id, "acct "+a.typ, a.typ,
		); err != nil {
			t.Fatalf("insert %s: %v", a.typ, err)
		}
		for _, status := range []string{"in_progress", "completed"} {
			if _, err := database.Conn().Exec(
				`INSERT INTO reconciliation_sessions (account_id, statement_date, statement_balance, status)
				 VALUES (?, DATE '2024-02-01', 0, ?)`, a.id, status,
			); err != nil {
				t.Fatalf("insert %s session: %v", a.typ, err)
			}
		}
	}

	applyVersion(t, database, 36)

	want := map[string]int{"investment": 0, "hsa_investment": 0, "checking": 2, "hsa": 2}
	for _, a := range accounts {
		var n int
		if err := database.Conn().QueryRow(
			`SELECT COUNT(*) FROM reconciliation_sessions WHERE CAST(account_id AS VARCHAR) = ?`, a.id,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want[a.typ] {
			t.Errorf("%s sessions = %d, want %d", a.typ, n, want[a.typ])
		}
	}
}
