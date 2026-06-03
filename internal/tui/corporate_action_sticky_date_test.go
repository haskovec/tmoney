package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// The corporate-action dialogs (Stock Split, Merger, Spin-Off) participate in
// the same session "sticky date" (a.txnDialogLastSavedDate) as the transaction
// and investment dialogs: a newly-opened dialog seeds its Date field from the
// last-saved date, and a successful save updates that shared date. These tests
// pin both directions for each of the three dialogs and the cross-dialog share.

func newStickyDateTestApp(seed types.Date) *App {
	return &App{
		currentView:            ViewInvestmentRegister,
		keys:                   defaultKeyMap(),
		menubar:                widget.NewMenuBar(),
		statusbar:              widget.NewStatusBar(),
		sidebar:                NewSidebar(),
		txnDialogLastSavedDate: seed,
	}
}

// ---- Spin-Off (date field index 2) -----------------------------------------

func TestApp_Update_SpinOffDialogDataMsg_SeedsFromStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.NewDate(2024, time.July, 23))

	model, _ := app.Update(spinOffDialogDataMsg{data: &spinOffDialogData{}})
	updatedApp := model.(*App)

	if updatedApp.spinOffDialog == nil {
		t.Fatal("spin-off dialog should be created")
	}
	if got := updatedApp.spinOffDialog.Fields()[2].Value; got != "07/23/2024" {
		t.Errorf("date field = %q, want %q (seeded from sticky date)", got, "07/23/2024")
	}
}

func TestApp_Update_SpinOffDialogDataMsg_DefaultsToTodayWhenNoStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	model, _ := app.Update(spinOffDialogDataMsg{data: &spinOffDialogData{}})
	updatedApp := model.(*App)

	today := time.Now().Format("01/02/2006")
	if got := updatedApp.spinOffDialog.Fields()[2].Value; got != today {
		t.Errorf("date field = %q, want %q (today)", got, today)
	}
}

func TestApp_Update_SpinOffDialogSavedMsg_StoresStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	saved := types.NewDate(2024, time.July, 23)
	model, _ := app.Update(spinOffDialogSavedMsg{savedDate: saved})
	updatedApp := model.(*App)

	if !updatedApp.txnDialogLastSavedDate.Equal(saved) {
		t.Errorf("txnDialogLastSavedDate = %s, want %s", updatedApp.txnDialogLastSavedDate, saved)
	}
}

// ---- Merger (date field index 2) -------------------------------------------

func TestApp_Update_MergerDialogDataMsg_SeedsFromStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.NewDate(2024, time.July, 23))

	model, _ := app.Update(mergerDialogDataMsg{data: &mergerDialogData{}})
	updatedApp := model.(*App)

	if updatedApp.mergerDialog == nil {
		t.Fatal("merger dialog should be created")
	}
	if got := updatedApp.mergerDialog.Fields()[2].Value; got != "07/23/2024" {
		t.Errorf("date field = %q, want %q (seeded from sticky date)", got, "07/23/2024")
	}
}

func TestApp_Update_MergerDialogDataMsg_DefaultsToTodayWhenNoStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	model, _ := app.Update(mergerDialogDataMsg{data: &mergerDialogData{}})
	updatedApp := model.(*App)

	today := time.Now().Format("01/02/2006")
	if got := updatedApp.mergerDialog.Fields()[2].Value; got != today {
		t.Errorf("date field = %q, want %q (today)", got, today)
	}
}

func TestApp_Update_MergerDialogSavedMsg_StoresStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	saved := types.NewDate(2024, time.July, 31)
	model, _ := app.Update(mergerDialogSavedMsg{savedDate: saved})
	updatedApp := model.(*App)

	if !updatedApp.txnDialogLastSavedDate.Equal(saved) {
		t.Errorf("txnDialogLastSavedDate = %s, want %s", updatedApp.txnDialogLastSavedDate, saved)
	}
}

// ---- Stock Split (date field index 1) --------------------------------------

func TestApp_Update_StockSplitDialogDataMsg_SeedsFromStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.NewDate(2024, time.July, 23))

	model, _ := app.Update(stockSplitDialogDataMsg{data: &stockSplitDialogData{}})
	updatedApp := model.(*App)

	if updatedApp.stockSplitDialog == nil {
		t.Fatal("stock split dialog should be created")
	}
	if got := updatedApp.stockSplitDialog.Fields()[1].Value; got != "07/23/2024" {
		t.Errorf("date field = %q, want %q (seeded from sticky date)", got, "07/23/2024")
	}
}

func TestApp_Update_StockSplitDialogDataMsg_DefaultsToTodayWhenNoStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	model, _ := app.Update(stockSplitDialogDataMsg{data: &stockSplitDialogData{}})
	updatedApp := model.(*App)

	today := time.Now().Format("01/02/2006")
	if got := updatedApp.stockSplitDialog.Fields()[1].Value; got != today {
		t.Errorf("date field = %q, want %q (today)", got, today)
	}
}

func TestApp_Update_StockSplitDialogSavedMsg_StoresStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	saved := types.NewDate(2024, time.June, 1)
	model, _ := app.Update(stockSplitDialogSavedMsg{savedDate: saved})
	updatedApp := model.(*App)

	if !updatedApp.txnDialogLastSavedDate.Equal(saved) {
		t.Errorf("txnDialogLastSavedDate = %s, want %s", updatedApp.txnDialogLastSavedDate, saved)
	}
}

// TestApp_CorporateActionDialogs_ShareTransactionStickyDate verifies the user's
// expectation that the spin-off dialog uses "our sticky date" — the single
// shared session date written by the regular transaction/investment dialogs.
// A buy saved on 2024-01-15 should seed a subsequently-opened spin-off dialog.
func TestApp_CorporateActionDialogs_ShareTransactionStickyDate(t *testing.T) {
	app := newStickyDateTestApp(types.Date{})

	// A buy saved on 2024-01-15 sets the shared sticky date.
	app.Update(buyDialogSavedMsg{savedDate: types.NewDate(2024, time.January, 15)})

	// Opening the spin-off dialog now seeds from that same shared date.
	model, _ := app.Update(spinOffDialogDataMsg{data: &spinOffDialogData{}})
	updatedApp := model.(*App)

	if got := updatedApp.spinOffDialog.Fields()[2].Value; got != "01/15/2024" {
		t.Errorf("spin-off date field = %q, want %q (shared sticky date from buy)", got, "01/15/2024")
	}
}
