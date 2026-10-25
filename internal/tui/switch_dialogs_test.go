package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// A File > Open runs as a command, so the user can open a dialog while it
// runs. When the open lands, that dialog shows the file we left: its IDs, its
// data and its Save belong to that file. These tests check that the switch
// closes it.

// keptAtASwitch names the surfaces that belong to no file, so a switch leaves
// them open. Every other registry surface declares a close.
var keptAtASwitch = map[string]bool{"help": true, "about": true}

// switchedApp opens a file, lets show open a surface on it, and runs a File >
// Open of a second file to its end.
func switchedApp(t *testing.T, i int, show func(*App)) *App {
	t.Helper()
	first, _ := dbtest.NewFile(t, fmt.Sprintf("fictional-a%d.tdb", i))
	second, _ := dbtest.NewFile(t, fmt.Sprintf("fictional-b%d.tdb", i))
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	if show != nil {
		show(a)
	}
	a.Update(fileDialogSavedMsg{db: second, path: second.Path()})
	return a
}

// lastNoteSays reports whether the last notification contains text.
func lastNoteSays(a *App, text string) bool {
	notes := a.statusbar.Notifications()
	return len(notes) > 0 && strings.Contains(notes[len(notes)-1].Text, text)
}

// TestEveryModalOfAFileHasAClose: a surface with no close stays open at a
// switch. Only the surfaces of no file may do that.
func TestEveryModalOfAFileHasAClose(t *testing.T) {
	for _, e := range newModalTestApp().modals() {
		switch {
		case e.close == nil && !keptAtASwitch[e.name]:
			t.Errorf("surface %q has no close, so a file switch leaves it open on the file it was opened for", e.name)
		case e.close != nil && keptAtASwitch[e.name]:
			t.Errorf("surface %q belongs to no file, but a file switch closes it", e.name)
		}
	}
}

// TestASwitchClosesEveryDialogOfTheOldFile: each surface of a file is open when
// File > Open of another file lands. The switch closes it, leaves none of its
// state behind, and says so.
func TestASwitchClosesEveryDialogOfTheOldFile(t *testing.T) {
	want := modalStateSnapshot(switchedApp(t, 0, nil))
	setters := surfaceSetters()
	for i, e := range newModalTestApp().modals() {
		if keptAtASwitch[e.name] {
			continue
		}
		t.Run(e.name, func(t *testing.T) {
			a := switchedApp(t, i+1, setters[e.name])

			if front := a.frontmostModal(); front != nil {
				t.Errorf("%q is still open after File > Open of another file (frontmost: %q)", e.name, front.name)
			}
			if left := diffSnapshots(want, modalStateSnapshot(a)); len(left) > 0 {
				slices.Sort(left)
				t.Errorf("the switch left state of %q behind: %v", e.name, left)
			}
			if !lastNoteSays(a, "Another file opened") {
				t.Errorf("notifications = %v, want one that says the dialog of the last file closed",
					a.statusbar.Notifications())
			}
		})
	}
}

// TestASwitchClosesTheOriginOfCreateCategory: Create Category hides the dialog
// that it came from and keeps it. The switch closes the hidden one too, or
// its data of the old file stays on the App.
func TestASwitchClosesTheOriginOfCreateCategory(t *testing.T) {
	a := switchedApp(t, 0, func(a *App) {
		surfaceSetters()["transaction"](a)
		a.txn.dlg.SetVisible(false)
		surfaceSetters()["createCategory"](a)
	})
	if a.txn.dlg != nil || a.txn.data != nil {
		t.Error("the hidden Transaction dialog of the old file is still on the App after the switch")
	}
}

// TestASwitchKeepsTheSurfacesOfNoFile: Help and About show nothing of a file,
// so they stay open, and the switch does not say that a dialog closed.
func TestASwitchKeepsTheSurfacesOfNoFile(t *testing.T) {
	setters := surfaceSetters()
	a := switchedApp(t, 0, func(a *App) {
		for name := range keptAtASwitch {
			setters[name](a)
		}
	})
	for _, e := range a.modals() {
		if keptAtASwitch[e.name] && !e.modal.IsVisible() {
			t.Errorf("%q closed at a file switch; it shows nothing of a file", e.name)
		}
	}
	if lastNoteSays(a, "Another file opened") {
		t.Error("the switch says that a dialog closed, but only Help and About were open")
	}
}

// TestAddPriceOpenedDuringAnOpenDoesNotCrash is the crash that a reviewer
// found: Add Price opens while File > Open runs, the open lands, and Save read
// the price view data that the switch cleared.
func TestAddPriceOpenedDuringAnOpenDoesNotCrash(t *testing.T) {
	first, _ := dbtest.NewFile(t, "fictional-a.tdb")
	second, _ := dbtest.NewFile(t, "fictional-b.tdb")
	a := NewApp(first, &config.Config{})
	closeAll(t, a)
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := a.services.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	oldPrices := a.services.Price
	a.switchView(ViewPrices)
	a.prices.data = &priceViewData{mode: pricesViewDetail, selectedSecurity: sec}
	a.prices.buildTable()

	a.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !a.price.IsVisible() {
		t.Fatal("setup: n on the price detail opened no Add Price dialog")
	}
	a.price.dlg.Fields()[1].Value = "185.50"
	a.price.dlg.SetFocusIndex(len(a.price.dlg.Fields())) // on Save

	a.Update(fileDialogSavedMsg{db: second, path: second.Path()})
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Enter after the switch crashed: %v", r)
			}
		}()
		a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	}()

	if a.price.IsVisible() {
		t.Error("the Add Price dialog of the old file is still open after the switch")
	}
	if p, err := oldPrices.GetBySecurityAndDate(sec.ID, types.Today()); err == nil && p != nil {
		t.Errorf("the closed dialog saved a price in the old file: %+v", p)
	}
}
