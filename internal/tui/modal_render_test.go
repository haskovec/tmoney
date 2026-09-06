package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/transferlink"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// newModalRenderTestApp returns an App with the chrome a full renderLayout
// pass needs, and nothing else. Every modal handle starts nil, so a test
// sets exactly the one surface it is about.
func newModalRenderTestApp() *App {
	return &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
}

// The import and link-transfers dialogs were key-routed and mouse-armed but
// appeared in neither renderLayout nor isDialogVisible, so File → Import and
// Transactions → Link Transfers opened a form the user could type into and
// could not see. These tests pin both halves of the fix.

func TestRenderLayout_ImportDialogOptionsStepRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.importDialog, _ = buildImportOptionsDialog(makeTestAccounts(), types.ID{})

	if got := app.viewContent(); !strings.Contains(got, "Import Transactions") {
		t.Error("renderLayout must paint the import dialog's options step")
	}
	if !app.isDialogVisible() {
		t.Error("isDialogVisible must report the import dialog as open")
	}
}

func TestRenderLayout_ImportDialogSourcePickerStepRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.importDialog = buildImportSourcePickerDialog([]string{"Checking", "Visa"}, "tmoney Checking")

	if got := app.viewContent(); !strings.Contains(got, "Pick Source Account") {
		t.Error("renderLayout must paint the import dialog's source-picker step")
	}
}

func TestRenderLayout_LinkTransfersDialogRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.linkTransfersDialog = buildLinkTransfersDialog(&transferlink.Result{Scanned: 3})

	if got := app.viewContent(); !strings.Contains(got, "Link Transfers") {
		t.Error("renderLayout must paint the link-transfers dialog")
	}
	if !app.isDialogVisible() {
		t.Error("isDialogVisible must report the link-transfers dialog as open")
	}
}

// A dialog absent from isDialogVisible has dead mouse arms, because
// handleMouseEvent gates on it before reaching handleDialogMouse. Assert the
// gate lets a click through to the dialog rather than to the view underneath.

func TestMouseGate_ImportDialogCancelClosesIt(t *testing.T) {
	app := newModalRenderTestApp()
	app.importDialog, _ = buildImportOptionsDialog(makeTestAccounts(), types.ID{})
	app.importDialogState = &importDialogState{}

	clickCancelButton(t, app, app.importDialog)

	if app.importDialog != nil {
		t.Error("clicking Cancel must close the import dialog")
	}
}

func TestMouseGate_LinkTransfersDialogCancelClosesIt(t *testing.T) {
	app := newModalRenderTestApp()
	app.linkTransfersDialog = buildLinkTransfersDialog(&transferlink.Result{Scanned: 3})

	clickCancelButton(t, app, app.linkTransfersDialog)

	if app.linkTransfersDialog != nil {
		t.Error("clicking Cancel must close the link-transfers dialog")
	}
}

// clickCancelButton clicks the right-hand (Cancel) button of a two-button
// dialog. Both dialogs here lay out [Primary] [Cancel] across the content
// width, so a click in the right three-quarters lands on Cancel.
func clickCancelButton(t *testing.T, app *App, d *dialog.Dialog) {
	t.Helper()
	// overlayDialog calls SetMaxHeight as a side effect of painting, and
	// DialogBounds reads that back through RenderedHeight. Render first so the
	// geometry the click is computed against is the geometry the user sees.
	_ = app.viewContent()

	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialog.DialogHorizontalOverhead
	btnY := startRow + 2 + d.ContentHeight() - 1 // last content row
	btnX := startCol + 3 + contentWidth*3/4

	app.Update(tea.MouseClickMsg{X: btnX, Y: btnY, Button: tea.MouseLeft})
}
