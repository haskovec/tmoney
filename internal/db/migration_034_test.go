package db

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// openAtVersion creates a fresh file and applies every embedded migration up
// to and including version v, bypassing Migrate so a test can observe the
// schema between two versions.
func openAtVersion(t *testing.T, v int) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "step.tdb")
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	database := &DB{conn: conn, path: path}
	if err := database.initializeMetadata(); err != nil {
		t.Fatalf("initializeMetadata: %v", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if m.Version > v {
			break
		}
		if err := database.runMigration(m); err != nil {
			t.Fatalf("migration %03d: %v", m.Version, err)
		}
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// applyVersion runs exactly one embedded migration.
func applyVersion(t *testing.T, database *DB, v int) {
	t.Helper()
	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range migrations {
		if m.Version == v {
			if err := database.runMigration(m); err != nil {
				t.Fatalf("migration %03d: %v", v, err)
			}
			return
		}
	}
	t.Fatalf("no embedded migration %03d", v)
}

// catalogSQL returns every user table, view and index definition from the
// DuckDB catalog, sorted, so two schemas can be compared as text.
func catalogSQL(t *testing.T, database *DB) []string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT sql FROM duckdb_tables() WHERE NOT internal`,
		`SELECT sql FROM duckdb_views() WHERE NOT internal`,
		`SELECT sql FROM duckdb_indexes()`,
	} {
		rows, err := database.Conn().Query(q)
		if err != nil {
			t.Fatalf("catalog query: %v", err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan catalog: %v", err)
			}
			out = append(out, s)
		}
		_ = rows.Close()
	}
	sort.Strings(out)
	return out
}

// TestMigration034 pins the two things the rebuild must get right: the
// schema is unchanged apart from the widened CHECK and the portfolio view's
// type list, and every existing hsa account becomes hsa_investment with its
// rows intact.
func TestMigration034(t *testing.T) {
	database := openAtVersion(t, 33)

	seed := []string{
		`INSERT INTO accounts (id, name, type, opening_date, track_lots, closed_date)
		 VALUES ('11111111-1111-1111-1111-111111111111', 'Maple Invest HSA', 'hsa', '2024-01-01', TRUE, NULL),
		        ('22222222-2222-2222-2222-222222222222', 'Checking', 'checking', '2024-01-01', FALSE, NULL),
		        ('33333333-3333-3333-3333-333333333333', 'Old Savings', 'savings', '2020-01-01', FALSE, '2023-06-30')`,
		`INSERT INTO securities (id, ticker, name, security_type)
		 VALUES ('44444444-4444-4444-4444-444444444444', 'ACME', 'Acme Index Fund', 'etf')`,
		`INSERT INTO investment_transactions (id, account_id, date, transaction_type, security_id, shares, price_per_share, total_amount, transfer_id, transfer_account_id)
		 VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '2024-02-01', 'buy', '44444444-4444-4444-4444-444444444444', 10, 25.00, -250.00, NULL, NULL),
		        ('66666666-6666-6666-6666-666666666666', '11111111-1111-1111-1111-111111111111', '2024-01-15', 'transfer_cash', NULL, NULL, NULL, 500.00, '77777777-7777-7777-7777-777777777777', '22222222-2222-2222-2222-222222222222')`,
		`INSERT INTO investment_lots (id, account_id, security_id, shares, original_shares, cost_per_share, purchase_date, source_transaction_id)
		 VALUES ('88888888-8888-8888-8888-888888888888', '11111111-1111-1111-1111-111111111111', '44444444-4444-4444-4444-444444444444', 10, 10, 25.00, '2024-02-01', '55555555-5555-5555-5555-555555555555')`,
		`INSERT INTO transactions (id, account_id, date, amount, transfer_id, transfer_account_id)
		 VALUES ('99999999-9999-9999-9999-999999999999', '22222222-2222-2222-2222-222222222222', '2024-01-15', -500.00, '77777777-7777-7777-7777-777777777777', '11111111-1111-1111-1111-111111111111')`,
		`INSERT INTO scheduled_transactions (id, account_id, amount, frequency, start_date, next_date)
		 VALUES ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', '22222222-2222-2222-2222-222222222222', -50.00, 'monthly', '2024-01-01', '2024-02-01')`,
		`INSERT INTO reconciliation_sessions (id, account_id, statement_date, statement_balance)
		 VALUES ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', '22222222-2222-2222-2222-222222222222', '2024-01-31', 100.00)`,
	}
	for _, q := range seed {
		if _, err := database.Conn().Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	before := catalogSQL(t, database)
	countsBefore := tableCounts(t, database)

	applyVersion(t, database, 34)

	t.Run("catalog differs only in the two type lists", func(t *testing.T) {
		after := catalogSQL(t, database)
		// Normalize the two intended differences, then demand equality.
		norm := func(in []string) []string {
			out := make([]string, 0, len(in))
			for _, s := range in {
				s = strings.ReplaceAll(s, "'investment', 'hsa', 'hsa_investment', 'cash'", "'investment', 'hsa', 'cash'")
				s = strings.ReplaceAll(s, "('investment', 'hsa_investment')", "('investment', 'hsa')")
				out = append(out, s)
			}
			return out
		}
		b, a := norm(before), norm(after)
		if len(a) != len(b) {
			t.Fatalf("catalog object count changed: %d -> %d\nbefore:\n%s\nafter:\n%s",
				len(b), len(a), strings.Join(b, "\n"), strings.Join(a, "\n"))
		}
		for i := range b {
			if a[i] != b[i] {
				t.Errorf("catalog drift:\n  before: %s\n  after:  %s", b[i], a[i])
			}
		}
		// And the intended differences did happen.
		joined := strings.Join(after, "\n")
		if !strings.Contains(joined, "'hsa_investment'") {
			t.Error("accounts CHECK does not admit hsa_investment")
		}
		if !strings.Contains(joined, `IN ('investment', 'hsa_investment')`) {
			t.Error("portfolio_holdings does not gate on hsa_investment")
		}
	})

	t.Run("hsa becomes hsa_investment and nothing else moves", func(t *testing.T) {
		var typ string
		if err := database.Conn().QueryRow(
			`SELECT type FROM accounts WHERE name = 'Maple Invest HSA'`).Scan(&typ); err != nil {
			t.Fatal(err)
		}
		if typ != "hsa_investment" {
			t.Errorf("type = %q, want hsa_investment", typ)
		}
		var closed sql.NullString
		if err := database.Conn().QueryRow(
			`SELECT CAST(closed_date AS VARCHAR) FROM accounts WHERE name = 'Old Savings'`).Scan(&closed); err != nil {
			t.Fatal(err)
		}
		if !closed.Valid || closed.String != "2023-06-30" {
			t.Errorf("closed_date lost in rebuild: %+v", closed)
		}
		for table, n := range tableCounts(t, database) {
			if countsBefore[table] != n {
				t.Errorf("%s: %d rows before, %d after", table, countsBefore[table], n)
			}
		}
		var holdings int
		if err := database.Conn().QueryRow(
			`SELECT COUNT(*) FROM portfolio_holdings WHERE account_name = 'Maple Invest HSA' AND total_shares = 10`).Scan(&holdings); err != nil {
			t.Fatal(err)
		}
		if holdings != 1 {
			t.Errorf("portfolio_holdings rows for the HSA = %d, want 1", holdings)
		}
	})

	t.Run("a cash hsa is accepted by the CHECK", func(t *testing.T) {
		_, err := database.Conn().Exec(
			`INSERT INTO accounts (id, name, type, opening_date) VALUES (uuidv7(), 'Cedar Bank HSA', 'hsa', '2024-01-01')`)
		if err != nil {
			t.Errorf("insert hsa: %v", err)
		}
	})
}

func tableCounts(t *testing.T, database *DB) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{
		"accounts", "transactions", "scheduled_transactions", "reconciliation_sessions",
		"investment_transactions", "investment_lots", "investment_positions",
		"transaction_splits", "scheduled_split_items", "investment_transaction_lots",
	} {
		var n int
		if err := database.Conn().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}
