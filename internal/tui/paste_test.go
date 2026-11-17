package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin paste (bracketed paste arrives as one tea.PasteMsg). The
// text goes into the text field that has focus, as if typed, and nowhere
// else.

func paste(a *App, s string) tea.Cmd {
	_, cmd := a.update(tea.PasteMsg{Content: s})
	return cmd
}

func txnField(a *App, i int) *dialog.Field { return a.txn.dlg.Fields()[i] }

const (
	txnFieldDate     = 0
	txnFieldCategory = 2
	txnFieldMemo     = 4
)

func TestPasteText(t *testing.T) {
	for in, want := range map[string]string{
		"52.10":          "52.10",
		"52.10\n":        "52.10",
		"52.10\r\n":      "52.10",
		"Main St\nApt 4": "Main St Apt 4",
		"a\r\nb":         "a b",
		"a\tb":           "a b",
		"a\x1b[Ab\x07":   "a[Ab",
		"":               "",
		"\n":             "",
		"Café 🍕":         "Café 🍕",
	} {
		if got := pasteText(in); got != want {
			t.Errorf("pasteText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPaste_TypesIntoTheFocusedField(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(txnFieldMemo)

	paste(a, "Weekly shop\n")

	if got := txnField(a, txnFieldMemo).Value; got != "Weekly shop" {
		t.Errorf("Memo = %q, want %q", got, "Weekly shop")
	}
}

// A key's String() is its text, so a paste sent as one key could act as Esc,
// Tab or Enter. Each rune goes in alone, so a pasted key name stays text.
func TestPaste_KeyNamesStayText(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(txnFieldMemo)

	paste(a, "esc")
	paste(a, "tab")
	paste(a, "enter")

	if !a.txn.IsVisible() {
		t.Fatal("the paste closed the dialog")
	}
	if got := txnField(a, txnFieldMemo).Value; got != "esctabenter" {
		t.Errorf("Memo = %q, want the pasted text", got)
	}
	if a.txn.dlg.FocusIndex() != txnFieldMemo {
		t.Errorf("focus moved to %d, want it kept on Memo", a.txn.dlg.FocusIndex())
	}
}

// A pasted amount obeys the typed-amount rules: one sign at most.
func TestPaste_AmountKeepsOneSign(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(3)

	paste(a, "--52.10")

	if got := txnAmount(a).Value; got != "-52.10" {
		t.Errorf("Amount = %q, want -52.10", got)
	}
}

// A paste into a combo filters it like typing. Committing the match then runs
// the category sign rule, the same as after a typed query.
func TestPaste_ComboTakesTheQuery(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(txnFieldCategory)

	paste(a, "Groc")
	cat := txnField(a, txnFieldCategory)
	if cat.Query != "Groc" {
		t.Fatalf("Category query = %q, want Groc", cat.Query)
	}
	txnType(a, signTab)

	if got := cat.SelectedOption(); got != "Groceries" {
		t.Errorf("Category = %q, want Groceries", got)
	}
	if got := txnAmount(a).Value; got != "-" {
		t.Errorf("Amount = %q after an Expense pick, want \"-\"", got)
	}
}

// A date field takes the digits of a pasted date and skips its separators.
func TestPaste_DateField(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(txnFieldDate)
	date := txnField(a, txnFieldDate)
	date.DateCursorHome()

	paste(a, "03/04/2025")

	if date.Value != "03/04/2025" {
		t.Errorf("Date = %q, want 03/04/2025", date.Value)
	}
}

func TestPaste_DroppedWithoutTextFocus(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	before := fieldValues(a.txn.dlg)
	// Focus on the first button: a rune there types nothing.
	a.txn.dlg.SetFocusIndex(len(a.txn.dlg.Fields()))

	paste(a, "x")

	if !a.txn.IsVisible() {
		t.Fatal("the paste closed the dialog")
	}
	if after := fieldValues(a.txn.dlg); after != before {
		t.Errorf("fields changed from %q to %q", before, after)
	}
}

// Over a view a rune is a command ("q" quits, "2" switches the view, "?" opens
// help). A paste must not run any of them.
func TestPaste_DroppedOnAView(t *testing.T) {
	a := newModalTestApp()

	cmd := paste(a, "q2?")

	if a.quitting || cmd != nil {
		t.Error("the paste ran a command")
	}
	if a.currentView != ViewDashboard || a.showHelp {
		t.Errorf("view = %v, help = %v; want the dashboard and no help", a.currentView, a.showHelp)
	}
}

func TestPaste_DroppedWhileAnErrorShows(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(txnFieldMemo)
	a.err = errors.New("disk full")

	paste(a, "memo")

	if a.err == nil {
		t.Error("the paste dismissed the error")
	}
	if got := txnField(a, txnFieldMemo).Value; got != "" {
		t.Errorf("Memo = %q under the error, want it unchanged", got)
	}
}

func TestPaste_ViewSearch(t *testing.T) {
	a := newModalTestApp()
	a.currentView = ViewPrices
	a.prices = priceViewState{data: &priceViewData{searching: true}}

	paste(a, "Total Market\n")

	if got := a.prices.data.searchQuery; got != "Total Market" {
		t.Errorf("search = %q, want %q", got, "Total Market")
	}
	if !a.prices.data.searching {
		t.Error("the paste ended the search")
	}
}

func TestPaste_SplitEditorAmount(t *testing.T) {
	a := newModalTestApp()
	sd := NewSplitDialog(types.MustNewMoney("-100.00"), []string{"Groceries"}, []types.ID{types.NewID()})
	sd.fieldFocus = splitFieldAmount
	a.split.editor = sd

	paste(a, "-40.00")

	if got := sd.rows[0].amountField.Value; got != "-40.00" {
		t.Errorf("row Amount = %q, want -40.00", got)
	}
}

// Every surface that can hold a text field takes a paste. A new surface must
// implement textTarget, or a paste into it is lost without a word.
func TestGuard_EverySurfaceTakesPaste(t *testing.T) {
	noText := map[string]bool{"help": true, "mergerConfirm": true}
	for _, e := range newModalTestApp().modals() {
		if _, ok := e.modal.(textTarget); ok == noText[e.name] {
			t.Errorf("surface %q: implements textTarget = %v, want %v", e.name, ok, !noText[e.name])
		}
	}
}

func fieldValues(d *dialog.Dialog) string {
	s := ""
	for _, f := range d.Fields() {
		s += f.Value + "|" + f.Query + "|"
	}
	return s
}

// On a multi-line preview a paste goes to the panel that has focus. In the
// header, the split total follows a pasted Amount, as it does a typed one.
func TestPaste_SchedulePreviewPanels(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)
	app := env.app
	p := app.schedPreviewDialog
	sd := p.SplitDialog()

	header := p.HeaderDialog()
	header.SetFocusIndex(previewMultiFieldAmount)
	amount := header.Fields()[previewMultiFieldAmount]
	amount.Value = ""
	amount.MoveCursorEnd()
	paste(app, "899.99")

	if want := types.MustNewMoney("899.99"); !sd.totalAmount.Equal(want) {
		t.Fatalf("split total = %s, want %s (it follows the pasted Amount)", sd.totalAmount, want)
	}

	p.setSplitFocus(true)
	sd.focus, sd.rowIndex, sd.fieldFocus = splitFocusRows, 1, splitFieldMemo
	paste(app, "one-off")

	if got := sd.rows[1].memoField.Value; got != "one-off" {
		t.Errorf("line 2 Memo = %q, want one-off", got)
	}
	if got := amount.Value; got != "899.99" {
		t.Errorf("header Amount = %q, want it unchanged by a paste into the lines", got)
	}
}

func TestPaste_PaycheckWizard(t *testing.T) {
	fx := newPaycheckWizardFixture()
	w := NewPaycheckWizard(fx.categoryOptions, fx.categoryIDs, fx.accounts)
	a := newModalTestApp()
	a.paycheck.wizard = w

	w.focusIndex = 0 // Employer
	paste(a, "Acme Corp")
	if got := w.Employer().Value; got != "Acme Corp" {
		t.Fatalf("Employer = %q, want Acme Corp", got)
	}

	// Pay frequency is a select: it takes no text.
	w.focusIndex = 1
	before := w.Frequency().SelectedIndex
	paste(a, "x")
	if w.Frequency().SelectedIndex != before || w.Employer().Value != "Acme Corp" {
		t.Error("a paste on the frequency select changed the form")
	}
}
