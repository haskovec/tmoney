package db

import (
	"strings"
	"testing"
)

// TestMigration037 pins what the securities rebuild must get right: the
// schema is unchanged apart from the asset_class list and the five dropped
// secondary indexes, every row keeps its values (an 'index' security becomes
// 'unclassified'), and asset_class can be updated on a held security.
func TestMigration037(t *testing.T) {
	database := openAtVersion(t, 36)

	const (
		acct  = "11111111-1111-1111-1111-111111111111"
		acme  = "22222222-2222-2222-2222-222222222222"
		umbr  = "33333333-3333-3333-3333-333333333333"
		cedar = "44444444-4444-4444-4444-444444444444"
	)
	seed := []string{
		`INSERT INTO accounts (id, name, type, opening_date, track_lots)
		 VALUES ('` + acct + `', 'Maple Invest Brokerage', 'investment', '2024-01-01', FALSE)`,
		`INSERT INTO securities (id, ticker, name, security_type, asset_class, currency, exchange, hidden, isin)
		 VALUES ('` + acme + `', 'ACME', 'Acme Total Market Index', 'etf', 'index', 'USD', 'NYSE', FALSE, 'XS0000000009'),
		        ('` + umbr + `', 'UMBR', 'Umbrella Total Bond', 'mutual_fund', 'domestic_bond', 'USD', NULL, TRUE, ''),
		        ('` + cedar + `', '', 'Cedar 2045 Target Fund', 'other', 'asset_mixture', 'EUR', NULL, FALSE, NULL)`,
		`INSERT INTO security_prices (security_id, date, price, source)
		 VALUES ('` + acme + `', '2024-01-15', 245.10, 'manual')`,
		`INSERT INTO investment_positions (account_id, security_id, shares, average_cost_per_share)
		 VALUES ('` + acct + `', '` + acme + `', 412.5, 190.06),
		        ('` + acct + `', '` + umbr + `', 410, 76.10)`,
	}
	for _, q := range seed {
		if _, err := database.Conn().Exec(q); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}

	before := catalogSQL(t, database)
	rowsBefore := securityRows(t, database)
	holdingsBefore := portfolioHoldingRows(t, database)

	applyVersion(t, database, 37)

	t.Run("catalog differs only in the class list and the dropped indexes", func(t *testing.T) {
		after := catalogSQL(t, database)
		var b []string
		for _, s := range before {
			if strings.Contains(s, "INDEX idx_securities_") {
				continue
			}
			b = append(b, strings.ReplaceAll(s, "'international_stock', 'index', ", "'international_stock', "))
		}
		var a []string
		for _, s := range after {
			a = append(a, strings.ReplaceAll(s, "'mid_cap_stock', ", ""))
		}
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
		if !strings.Contains(joined, "'large_cap_stock', 'mid_cap_stock', 'small_cap_stock'") {
			t.Error("securities CHECK does not admit mid_cap_stock between large and small cap")
		}
		if strings.Contains(joined, "'index'") {
			t.Error("securities CHECK still admits index")
		}
		if strings.Contains(joined, "idx_securities_") {
			t.Error("a securities secondary index was recreated")
		}
	})

	t.Run("rows keep their values and index becomes unclassified", func(t *testing.T) {
		after := securityRows(t, database)
		if len(after) != len(rowsBefore) {
			t.Fatalf("securities: %d rows before, %d after", len(rowsBefore), len(after))
		}
		for id, row := range rowsBefore {
			want := row
			if id == acme {
				want = strings.Replace(row, "|index|", "|unclassified|", 1)
			}
			if after[id] != want {
				t.Errorf("security %s:\n  before: %s\n  after:  %s\n  want:   %s", id, row, after[id], want)
			}
		}
		if !strings.Contains(after[acme], "|XS0000000009") {
			t.Errorf("ACME lost its ISIN: %s", after[acme])
		}
	})

	t.Run("portfolio_holdings gives the same rows", func(t *testing.T) {
		after := portfolioHoldingRows(t, database)
		if strings.Join(after, "\n") != strings.Join(holdingsBefore, "\n") {
			t.Errorf("portfolio_holdings changed:\nbefore:\n%s\nafter:\n%s",
				strings.Join(holdingsBefore, "\n"), strings.Join(after, "\n"))
		}
	})

	t.Run("asset_class updates on a held, priced security", func(t *testing.T) {
		if _, err := database.Conn().Exec(
			`UPDATE securities SET asset_class = 'mid_cap_stock', updated_at = CURRENT_TIMESTAMP WHERE id = ?`, acme); err != nil {
			t.Fatalf("update asset_class: %v", err)
		}
		var class string
		if err := database.Conn().QueryRow(
			`SELECT asset_class FROM securities WHERE id = ?`, acme).Scan(&class); err != nil {
			t.Fatal(err)
		}
		if class != "mid_cap_stock" {
			t.Errorf("asset_class = %q, want mid_cap_stock", class)
		}
	})

	t.Run("the CHECK takes mid_cap_stock and rejects index", func(t *testing.T) {
		if _, err := database.Conn().Exec(
			`INSERT INTO securities (ticker, name, security_type, asset_class) VALUES ('MIDX', 'Maple Mid Cap Index', 'etf', 'mid_cap_stock')`); err != nil {
			t.Errorf("insert mid_cap_stock: %v", err)
		}
		if _, err := database.Conn().Exec(
			`INSERT INTO securities (ticker, name, security_type, asset_class) VALUES ('GLBX', 'Globex Index', 'etf', 'index')`); err == nil {
			t.Error("insert index: want a CHECK error, got none")
		}
	})

	t.Run("a new row gets a uuidv7 id", func(t *testing.T) {
		var id string
		if err := database.Conn().QueryRow(
			`SELECT CAST(id AS VARCHAR) FROM securities WHERE ticker = 'MIDX'`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		// The version nibble is the first digit of the third group.
		if len(id) != 36 || id[14] != '7' {
			t.Errorf("id %q is not a UUIDv7", id)
		}
	})
}

// securityRows returns every securities row as one string, keyed by id.
func securityRows(t *testing.T, database *DB) map[string]string {
	t.Helper()
	rows, err := database.Conn().Query(`
		SELECT CAST(id AS VARCHAR),
		       concat_ws('|', ticker, name, security_type, asset_class, currency,
		                 coalesce(exchange, '<null>'), CAST(hidden AS VARCHAR),
		                 CAST(created_at AS VARCHAR), CAST(updated_at AS VARCHAR),
		                 coalesce(isin, '<null>'))
		FROM securities`)
	if err != nil {
		t.Fatalf("read securities: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, row string
		if err := rows.Scan(&id, &row); err != nil {
			t.Fatalf("scan security: %v", err)
		}
		out[id] = "|" + row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate securities: %v", err)
	}
	return out
}

// portfolioHoldingRows returns the portfolio_holdings view as sorted text.
func portfolioHoldingRows(t *testing.T, database *DB) []string {
	t.Helper()
	rows, err := database.Conn().Query(`
		SELECT concat_ws('|', account_name, ticker, security_name,
		                 CAST(total_shares AS VARCHAR), CAST(total_cost_basis AS VARCHAR))
		FROM portfolio_holdings ORDER BY ALL`)
	if err != nil {
		t.Fatalf("read portfolio_holdings: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatalf("scan portfolio_holdings: %v", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate portfolio_holdings: %v", err)
	}
	return out
}
