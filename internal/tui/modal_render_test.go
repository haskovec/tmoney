package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/transaction"
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

// A visible import or link-transfers dialog must appear in viewContent() and
// be reported by isDialogVisible(). The second half is what lets a click reach
// the dialog: handleMouseEvent gates on isDialogVisible.

func TestRenderLayout_ImportDialogOptionsStepRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.importer = newImportSurface(firstOf(buildImportOptionsDialog(makeTestAccounts(), types.ID{})))

	if got := app.viewContent(); !strings.Contains(got, "Import Transactions") {
		t.Error("renderLayout must paint the import dialog's options step")
	}
	if !app.isDialogVisible() {
		t.Error("isDialogVisible must report the import dialog as open")
	}
}

func TestRenderLayout_ImportDialogSourcePickerStepRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.importer = newImportSurface(buildImportSourcePickerDialog([]string{"Checking", "Visa"}, "tmoney Checking"))

	if got := app.viewContent(); !strings.Contains(got, "Pick Source Account") {
		t.Error("renderLayout must paint the import dialog's source-picker step")
	}
	if !app.isDialogVisible() {
		t.Error("isDialogVisible must report the import dialog as open")
	}
}

func TestRenderLayout_LinkTransfersDialogRenders(t *testing.T) {
	app := newModalRenderTestApp()
	app.linkTransfers = &linkTransfersSurface{modalSurface: modalSurface{dlg: buildLinkTransfersDialog(&transferlink.Result{Scanned: 3})}}

	if got := app.viewContent(); !strings.Contains(got, "Link Transfers") {
		t.Error("renderLayout must paint the link-transfers dialog")
	}
	if !app.isDialogVisible() {
		t.Error("isDialogVisible must report the link-transfers dialog as open")
	}
}

// A click on a visible dialog's Cancel button must reach that dialog and close
// it, rather than fall through to the view underneath.

func TestMouseGate_ImportDialogCancelClosesIt(t *testing.T) {
	app := newModalRenderTestApp()
	app.importer = newImportSurface(firstOf(buildImportOptionsDialog(makeTestAccounts(), types.ID{})))

	// Preview on an empty state keeps the dialog open, so only Cancel closes it.
	clickCancelButton(t, app, app.importer.dlg)

	if app.importer != nil {
		t.Error("clicking Cancel must close the import dialog")
	}
}

func TestMouseGate_LinkTransfersDialogCancelClosesIt(t *testing.T) {
	app := newModalRenderTestApp()
	// One clean pair, and the result installed on the surface: Submit would then
	// close the dialog AND return the execute command, so a Cancel click is
	// told apart from a Submit click by the absence of that command.
	res := &transferlink.Result{Scanned: 2, Clean: []*transferlink.Candidate{{
		From:        &transaction.Transaction{Date: types.NewDate(2024, 1, 15), Amount: types.MustNewMoney("-100.00")},
		To:          &transaction.Transaction{Date: types.NewDate(2024, 1, 15), Amount: types.MustNewMoney("100.00")},
		FromAccount: "Checking",
		ToAccount:   "Savings",
	}}}
	app.linkTransfers = &linkTransfersSurface{modalSurface: modalSurface{dlg: buildLinkTransfersDialog(res)}, result: res}

	cmd := clickCancelButton(t, app, app.linkTransfers.dlg)

	if app.linkTransfers != nil {
		t.Error("clicking Cancel must close the link-transfers dialog")
	}
	if cmd != nil {
		t.Error("clicking Cancel must not start the link execute command; the click landed on Submit")
	}
}

// clickCancelButton clicks the button labelled "Cancel" on d, located by the
// dialog's own hit test, and returns the command the click produced.
func clickCancelButton(t *testing.T, app *App, d *dialog.Dialog) tea.Cmd {
	t.Helper()
	// paintModals calls SetMaxHeight as a side effect of painting, and
	// DialogBounds reads that back through RenderedHeight. Render first so the
	// geometry the click is computed against is the geometry the user sees.
	_ = app.viewContent()

	cancelIdx := -1
	for i, b := range d.Buttons() {
		if b.Label == "Cancel" {
			cancelIdx = i
		}
	}
	if cancelIdx < 0 {
		t.Fatal("dialog has no Cancel button")
	}

	startCol, startRow, _, _ := d.DialogBounds(app.width, app.height)
	contentWidth := d.Width() - dialog.DialogHorizontalOverhead
	buttonRow := d.ContentHeight() - 1
	cancelX := -1
	for x := range contentWidth {
		hit := d.HitTestContent(x, buttonRow, contentWidth)
		if hit.Zone == dialog.DialogHitButton && hit.ButtonIndex == cancelIdx {
			cancelX = x
			break
		}
	}
	if cancelX < 0 {
		t.Fatal("could not locate the Cancel button by hit test")
	}

	_, cmd := app.Update(tea.MouseClickMsg{
		X:      startCol + 3 + cancelX,
		Y:      startRow + 2 + buttonRow,
		Button: tea.MouseLeft,
	})
	return cmd
}
