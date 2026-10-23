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

// An open search takes its text and its own keys, but not the commands that
// work everywhere: quit, undo and the menu keys still work, and a menu opened
// with the mouse gets the keys, not the hidden search.
func TestSearchQuery_GlobalCommandsStillWork(t *testing.T) {
	for _, site := range querySites() {
		t.Run(site.name, func(t *testing.T) {
			a := site.newApp(t)
			press(a, tea.KeyPressMsg{Code: '/', Text: "/"})
			if _, cmd := a.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}); !a.quitting || cmd == nil {
				t.Error("ctrl+q did not quit while the search was open")
			}

			a = site.newApp(t)
			press(a, tea.KeyPressMsg{Code: '/', Text: "/"})
			if _, cmd := a.Update(tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl}); cmd == nil {
				t.Error("ctrl+z (undo) did nothing while the search was open")
			}

			for _, open := range []tea.Msg{
				tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt},
				tea.KeyPressMsg{Code: tea.KeyF10},
				tea.MouseClickMsg{X: 2, Y: 0, Button: tea.MouseLeft},
			} {
				a = site.newApp(t)
				press(a, tea.KeyPressMsg{Code: '/', Text: "/"})
				a.Update(open)
				if !a.menubar.IsActive() {
					t.Errorf("%v did not open the menu while the search was open", open)
					continue
				}
				press(a, tea.KeyPressMsg{Code: 'j', Text: "j"})
				if got := site.query(a); got != "" {
					t.Errorf("after %v, a key went to the hidden search: query %q", open, got)
				}
				press(a, tea.KeyPressMsg{Code: tea.KeyEscape})
				if a.menubar.IsActive() {
					t.Errorf("after %v, Esc did not close the menu", open)
				}
			}
		})
	}
}
