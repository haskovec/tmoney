package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newAboutTestApp() *App {
	return &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   NewStatusBar(),
		menubar:     NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
}

func TestApp_MenuActionAboutOpensDialog(t *testing.T) {
	app := newAboutTestApp()

	app.menubar.Activate()
	_, _ = app.handleMenuAction(MenuActionAbout, "")

	if app.aboutDialog == nil || !app.aboutDialog.IsVisible() {
		t.Fatal("aboutDialog should be visible after MenuActionAbout")
	}
	if app.menubar.IsActive() {
		t.Error("menu bar should be deactivated after selecting About")
	}

	if got := app.aboutDialog.Title(); got != "Terminal Money" {
		t.Errorf("about dialog title = %q, want %q", got, "Terminal Money")
	}
	msg := app.aboutDialog.Message()
	if !strings.Contains(msg, "Author: Jeffrey Haskovec") {
		t.Errorf("about message missing author line, got %q", msg)
	}
	if !strings.Contains(msg, "Copyright 2026") {
		t.Errorf("about message missing copyright line, got %q", msg)
	}

	btns := app.aboutDialog.Buttons()
	if len(btns) != 1 || btns[0].Label != "OK" || !btns[0].Primary {
		t.Errorf("about dialog should have a single primary OK button, got %+v", btns)
	}
}

func TestApp_AboutDialogRendersInView(t *testing.T) {
	app := newAboutTestApp()

	_, _ = app.handleMenuAction(MenuActionAbout, "")

	view := app.View()
	if !strings.Contains(view.Content, "Terminal Money") {
		t.Error("rendered view should contain 'Terminal Money'")
	}
	if !strings.Contains(view.Content, "Jeffrey Haskovec") {
		t.Error("rendered view should contain author name")
	}
	if !strings.Contains(view.Content, "Copyright 2026") {
		t.Error("rendered view should contain copyright line")
	}
}

func TestApp_AboutDialogClosesOnEnter(t *testing.T) {
	app := newAboutTestApp()
	_, _ = app.handleMenuAction(MenuActionAbout, "")

	if app.aboutDialog == nil {
		t.Fatal("about dialog should be open")
	}

	// Enter on the focused OK button should dismiss it
	app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if app.aboutDialog != nil {
		t.Error("about dialog should be cleared after pressing Enter on OK")
	}
}

func TestApp_AboutDialogClosesOnEsc(t *testing.T) {
	app := newAboutTestApp()
	_, _ = app.handleMenuAction(MenuActionAbout, "")

	app.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	if app.aboutDialog != nil {
		t.Error("about dialog should be cleared after Esc")
	}
}

func TestApp_AboutDialogClosesOnOKClick(t *testing.T) {
	app := newAboutTestApp()
	_, _ = app.handleMenuAction(MenuActionAbout, "")

	d := app.aboutDialog
	if d == nil {
		t.Fatal("about dialog should be open")
	}

	// Find the OK button's screen coordinates and click on it.
	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialogHorizontalOverhead
	// The button row is the last content row; its rendered height equals
	// ContentHeight, so the absolute screen Y is startRow + 2 (top border
	// + top padding) + ContentHeight - 1 (last content row).
	btnY := startRow + 2 + d.ContentHeight() - 1
	// Click somewhere in the middle of the content width — the single
	// OK button is centered with even-distributed gaps.
	btnX := startCol + 3 + contentWidth/2

	app.Update(tea.MouseClickMsg{X: btnX, Y: btnY, Button: tea.MouseLeft})

	if app.aboutDialog != nil {
		t.Error("about dialog should be cleared after clicking OK")
	}
}

func TestApp_AboutDialogClosesOnCloseXClick(t *testing.T) {
	app := newAboutTestApp()
	_, _ = app.handleMenuAction(MenuActionAbout, "")

	d := app.aboutDialog
	if d == nil {
		t.Fatal("about dialog should be open")
	}

	// The [x] button sits in the top-right of the title row. Title row is
	// the first content row inside the dialog padding.
	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialogHorizontalOverhead
	titleY := startRow + 2                // top border + top padding
	xX := startCol + 3 + contentWidth - 2 // somewhere inside "[x]"

	app.Update(tea.MouseClickMsg{X: xX, Y: titleY, Button: tea.MouseLeft})

	if app.aboutDialog != nil {
		t.Error("about dialog should be cleared after clicking [x]")
	}
}
