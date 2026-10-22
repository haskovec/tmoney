package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// While the Prices or Securities search is open, every key is search text or a
// search key: a digit must not switch the view, "?" must not open help, and Esc
// must close the search and stay in the view. The Register and Corporate
// Actions searches already work this way.
func TestSearchQuery_GlobalKeysAreText(t *testing.T) {
	for _, site := range querySites()[:2] {
		t.Run(site.name, func(t *testing.T) {
			a := site.newApp(t)
			view := a.currentView
			press(a, tea.KeyPressMsg{Code: '/', Text: "/"})
			press(a, keysFor("3M?1", "graphemes")...)
			if got := site.query(a); got != "3M?1" {
				t.Errorf("query = %q, want %q", got, "3M?1")
			}
			if a.currentView != view || a.showHelp {
				t.Errorf("view = %v, help = %v; want %v and no help", a.currentView, a.showHelp, view)
			}

			press(a, tea.KeyPressMsg{Code: tea.KeyEscape})
			if a.currentView != view {
				t.Errorf("Esc left the view: %v, want %v", a.currentView, view)
			}
			searching := a.prices.data != nil && a.prices.data.searching
			if site.name == "securities" {
				searching = a.securities.data.searching
			}
			if searching {
				t.Error("Esc did not close the search")
			}
		})
	}
}
