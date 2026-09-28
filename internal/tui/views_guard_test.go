package tui

import "testing"

// The view table in views.go is the one list of views. These guards keep it
// the only list: every View constant has exactly one entry, and no other code
// spells the views out again.

// TestGuard_EveryViewHasOneEntry is guard 1: each View constant in app.go has
// exactly one entry in the table, and the table has no entry for a value that
// is not a constant.
func TestGuard_EveryViewHasOneEntry(t *testing.T) {
	consts := viewConstants(t)

	entries := map[View]int{}
	for _, e := range views() {
		entries[e.id]++
	}
	for _, vc := range consts {
		switch n := entries[vc.Value]; n {
		case 1:
		case 0:
			t.Errorf("%s has no entry in allViews (views.go); add one", vc.Name)
		default:
			t.Errorf("%s has %d entries in allViews (views.go); want exactly one", vc.Name, n)
		}
		delete(entries, vc.Value)
	}
	for id := range entries {
		t.Errorf("allViews has an entry for View(%d), which is not a View constant", id)
	}
}
