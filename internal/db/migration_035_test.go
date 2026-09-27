package db

import (
	"encoding/json"
	"testing"
)

// TestMigration035 inverts the stored merger ratio. Before 035 the service
// read exchange_ratio as source shares per target share; from 035 on it is
// target shares per source share, so every merger row already on file is
// rewritten to the ratio the new meaning needs to describe the same event.
// Other keys and other action types are left as they are.
func TestMigration035(t *testing.T) {
	database := openAtVersion(t, 34)

	rows := []struct {
		id, actionType, params string
	}{
		{"00000000-0000-7000-8000-000000000001", "merger", `{"exchange_ratio":2,"cash_per_share":5}`},
		{"00000000-0000-7000-8000-000000000002", "merger", `{"exchange_ratio":0.5}`},
		{"00000000-0000-7000-8000-000000000003", "split", `{"numerator":2,"denominator":1}`},
		{"00000000-0000-7000-8000-000000000004", "spin_off", `{"share_ratio":0.25,"parent_allocation_pct":80}`},
	}
	for _, r := range rows {
		if _, err := database.Conn().Exec(
			`INSERT INTO corporate_actions (id, action_type, security_id, action_date, parameters)
			 VALUES (?, ?, uuidv7(), DATE '2024-06-01', ?)`,
			r.id, r.actionType, r.params,
		); err != nil {
			t.Fatalf("insert %s: %v", r.actionType, err)
		}
	}

	applyVersion(t, database, 35)

	params := func(id string) map[string]float64 {
		t.Helper()
		var raw string
		if err := database.Conn().QueryRow(
			`SELECT parameters FROM corporate_actions WHERE CAST(id AS VARCHAR) = ?`, id,
		).Scan(&raw); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		var m map[string]float64
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("row %s: parameters %q are not a JSON object of numbers: %v", id, raw, err)
		}
		return m
	}
	want := map[string]map[string]float64{
		rows[0].id: {"exchange_ratio": 0.5, "cash_per_share": 5},
		rows[1].id: {"exchange_ratio": 2},
		rows[2].id: {"numerator": 2, "denominator": 1},
		rows[3].id: {"share_ratio": 0.25, "parent_allocation_pct": 80},
	}
	for _, r := range rows {
		got := params(r.id)
		w := want[r.id]
		if len(got) != len(w) {
			t.Errorf("%s row: keys = %v, want %v", r.actionType, got, w)
			continue
		}
		for k, v := range w {
			if got[k] != v {
				t.Errorf("%s row: %s = %v, want %v", r.actionType, k, got[k], v)
			}
		}
	}
}
