package db

import (
	"strings"
	"testing"
)

// TestMigration038 pins what the categories and payees rebuild must get right:
// the schema is unchanged apart from dropping the four low-value secondary
// indexes on categories/payees and the two outbound FKs (categories.parent_id
// and payees.default_category_id); every row and view output is preserved;
// in-place UPDATE works on FK-referenced categories and payees; and all inbound
// FKs from child tables remain enforced by DuckDB.
func TestMigration038(t *testing.T) {
	database := openAtVersion(t, 37)

	const (
		acct       = "11111111-1111-1111-1111-111111111111"
		parentCat  = "22222222-2222-2222-2222-222222222221"
		childCat   = "22222222-2222-2222-2222-222222222222"
		otherCat   = "22222222-2222-2222-2222-222222222223"
		payeeID    = "33333333-3333-3333-3333-333333333331"
		aliasID    = "33333333-3333-3333-3333-333333333332"
		txnID      = "44444444-4444-4444-4444-444444444441"
		splitID    = "44444444-4444-4444-4444-444444444442"
		schedID    = "55555555-5555-5555-5555-555555555551"
		schedSplit = "55555555-5555-5555-5555-555555555552"
	)

	seed := []string{
		`INSERT INTO accounts (id, name, type, opening_balance, opening_date)
		 VALUES ('` + acct + `', 'Checking', 'checking', 1000.00, '2025-01-01')`,
		`INSERT INTO categories (id, name, type, system_category)
		 VALUES ('` + parentCat + `', 'Food', 'expense', FALSE),
		        ('` + otherCat + `', 'Other', 'expense', FALSE)`,
		`INSERT INTO categories (id, name, parent_id, type, system_category)
		 VALUES ('` + childCat + `', 'Groceries', '` + parentCat + `', 'expense', FALSE)`,
		`INSERT INTO payees (id, name, default_category_id, notes)
		 VALUES ('` + payeeID + `', 'Trader Joes', '` + childCat + `', 'weekly')`,
		`INSERT INTO payee_aliases (id, payee_id, pattern, match_type)
		 VALUES ('` + aliasID + `', '` + payeeID + `', 'TRADER', 'contains')`,
		`INSERT INTO transactions (id, account_id, date, amount, payee_id, category_id, memo, status)
		 VALUES ('` + txnID + `', '` + acct + `', '2025-02-01', -75.50, '` + payeeID + `', '` + childCat + `', 'food run', 'cleared')`,
		`INSERT INTO transaction_splits (id, transaction_id, category_id, amount, memo)
		 VALUES ('` + splitID + `', '` + txnID + `', '` + childCat + `', -75.50, 'line')`,
		`INSERT INTO scheduled_transactions (id, account_id, payee_id, category_id, amount, frequency, start_date, next_date)
		 VALUES ('` + schedID + `', '` + acct + `', '` + payeeID + `', '` + childCat + `', -50.00, 'monthly', '2025-01-01', '2025-03-01')`,
		`INSERT INTO scheduled_split_items (id, scheduled_transaction_id, category_id, amount, memo)
		 VALUES ('` + schedSplit + `', '` + schedID + `', '` + childCat + `', -50.00, 'sched line')`,
	}
	for _, q := range seed {
		if _, err := database.Conn().Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	before := catalogSQL(t, database)
	countsBefore := tableCounts(t, database)

	applyVersion(t, database, 38)

	t.Run("catalog differs only in dropped indexes and the two outbound FKs", func(t *testing.T) {
		after := catalogSQL(t, database)
		var b []string
		for _, s := range before {
			if strings.Contains(s, "INDEX idx_categories_") || strings.Contains(s, "INDEX idx_payees_name") {
				continue
			}
			// Normalize the two intentionally removed outbound FK clauses.
			s = strings.ReplaceAll(s, "FOREIGN KEY (parent_id) REFERENCES categories(id)", "")
			s = strings.ReplaceAll(s, "FOREIGN KEY (default_category_id) REFERENCES categories(id)", "")
			s = strings.ReplaceAll(s, ", ,", ",")
			s = strings.ReplaceAll(s, ", )", ")")
			s = strings.ReplaceAll(s, ",)", ")")
			b = append(b, s)
		}
		if len(after) != len(b) {
			t.Fatalf("catalog object count changed: %d -> %d\nbefore:\n%s\nafter:\n%s",
				len(b), len(after), strings.Join(b, "\n"), strings.Join(after, "\n"))
		}
		for i := range b {
			if after[i] != b[i] {
				t.Errorf("catalog drift:\n  before: %s\n  after:  %s", b[i], after[i])
			}
		}

		joined := strings.Join(after, "\n")
		for _, dropped := range []string{
			"idx_categories_parent",
			"idx_categories_type",
			"idx_categories_name_parent",
			"idx_payees_name",
		} {
			if strings.Contains(joined, dropped) {
				t.Errorf("index %s should have been dropped", dropped)
			}
		}
	})

	t.Run("row counts are preserved across all tables", func(t *testing.T) {
		countsAfter := tableCounts(t, database)
		for table, want := range countsBefore {
			if got := countsAfter[table]; got != want {
				t.Errorf("table %s: %d rows before, %d after", table, want, got)
			}
		}
	})

	t.Run("in-place UPDATE succeeds on FK-referenced category and payee", func(t *testing.T) {
		if _, err := database.Conn().Exec(
			`UPDATE categories SET name = 'Supermarket', parent_id = CAST(? AS UUID), updated_at = CURRENT_TIMESTAMP WHERE id = CAST(? AS UUID)`,
			otherCat, childCat,
		); err != nil {
			t.Fatalf("UPDATE categories on FK-referenced row failed: %v", err)
		}
		if _, err := database.Conn().Exec(
			`UPDATE payees SET name = 'Trader Joe''s', default_category_id = CAST(? AS UUID), notes = 'updated', updated_at = CURRENT_TIMESTAMP WHERE id = CAST(? AS UUID)`,
			parentCat, payeeID,
		); err != nil {
			t.Fatalf("UPDATE payees on FK-referenced row failed: %v", err)
		}
	})

	t.Run("inbound FKs from child tables remain enforced", func(t *testing.T) {
		if _, err := database.Conn().Exec(`DELETE FROM categories WHERE id = CAST(? AS UUID)`, childCat); err == nil {
			t.Error("expected FK error deleting category referenced by transactions/splits/schedules")
		}
		if _, err := database.Conn().Exec(`DELETE FROM payees WHERE id = CAST(? AS UUID)`, payeeID); err == nil {
			t.Error("expected FK error deleting payee referenced by aliases/transactions/schedules")
		}
	})

	t.Run("uuidv7 defaults remain on rebuilt tables", func(t *testing.T) {
		for _, table := range []string{
			"categories", "payees", "payee_aliases",
			"transactions", "transaction_splits",
			"scheduled_transactions", "scheduled_split_items",
		} {
			var def string
			err := database.Conn().QueryRow(
				`SELECT column_default FROM duckdb_columns() WHERE table_name = ? AND column_name = 'id'`, table,
			).Scan(&def)
			if err != nil {
				t.Fatalf("read default for %s.id: %v", table, err)
			}
			if def != "uuidv7()" {
				t.Errorf("%s.id default = %q, want uuidv7()", table, def)
			}
		}
	})
}
