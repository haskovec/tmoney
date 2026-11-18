package tui

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// openPreviewLineAddNew focuses the Category picker of line row in the open
// multi-line preview, types query, highlights [+ Add new category…] and
// presses Enter, all through the preview's key handler.
func openPreviewLineAddNew(t *testing.T, a *App, row int, query string) *App {
	t.Helper()
	p := a.schedPreviewDialog
	if p == nil || !p.IsMultiLine() {
		t.Fatal("a multi-line preview must be open")
	}
	sd := p.SplitDialog()
	p.setSplitFocus(true)
	sd.focus = splitFocusRows
	sd.rowIndex = row
	sd.fieldFocus = splitFieldCategory

	press := func(m tea.KeyPressMsg) {
		model, _ := a.handleSchedulePreviewDialogKey(m)
		a = model.(*App)
	}
	for _, r := range query {
		press(letterKey(r))
	}
	picker := sd.focusedPicker()
	if picker == nil {
		t.Fatal("the line's Category picker is not open")
	}
	for i := 0; i < 100 && !picker.IsAddNewHighlighted(); i++ {
		press(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if !picker.IsAddNewHighlighted() {
		t.Fatal("Down did not reach [+ Add new category…]")
	}
	press(enterKey())
	return a
}

// submitNewCategory confirms the open create-category sub-dialog as a
// top-level category, with the Name and Type it shows, and lands the result.
func submitNewCategory(t *testing.T, a *App) *App {
	t.Helper()
	if a.createCat.dlg == nil {
		t.Fatal("the create-category sub-dialog is not open")
	}
	a.createCat.dlg.Fields()[1].SelectedIndex = 0 // (top-level)
	model, cmd := a.submitCreateCatDialog()
	a = model.(*App)
	if cmd == nil {
		t.Fatal("submit must return a command")
	}
	model, _ = a.Update(cmd())
	return model.(*App)
}

func findCategory(t *testing.T, a *App, name string) *category.Category {
	t.Helper()
	cats, err := a.services.Category.List()
	if err != nil {
		t.Fatalf("List categories: %v", err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("category %q was not saved", name)
	return nil
}

// lineCategoryID returns the category the split editor's line row is on.
func lineCategoryID(sd *SplitDialog, row int) types.ID {
	i := sd.rows[row].categoryIndex
	if sd.rows[row].transferMode || i < 0 || i >= len(sd.categoryIDs) {
		return types.NilID
	}
	return sd.categoryIDs[i]
}

// [+ Add new category…] on a line of a multi-line preview opens the
// sub-dialog for that line. The Type default comes from the line's amount
// (the tax line, -100.00: Expense), not from the header's (+900.00).
func TestSchedPreviewLine_AddNew_OpensForTheLine(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)

	a := openPreviewLineAddNew(t, env.app, 1, "Dues")

	if a.createCat.dlg == nil || !a.createCat.dlg.IsVisible() {
		t.Fatal("the create-category sub-dialog did not open")
	}
	if o := a.createCat.openedFrom(); o.surface != createCatSourceSchedPreviewSplit || o.splitRow != 1 {
		t.Errorf("origin = %+v, want the preview line 1", o)
	}
	if a.schedPreviewDialog.IsVisible() {
		t.Error("the preview stays on screen under the sub-dialog")
	}
	f := a.createCat.dlg.Fields()
	if f[0].Value != "Dues" {
		t.Errorf("Name = %q, want the typed query %q", f[0].Value, "Dues")
	}
	if f[2].SelectedIndex != 0 {
		t.Error("Type is Income; the line's -100.00 means Expense")
	}
}

// A confirmed category goes on the originating line. The other line keeps
// its category, the amounts stay, focus moves to the line's Amount, and the
// posted transaction carries the new category on that line.
func TestSchedPreviewLine_AddNew_AppliesToTheLineAndPosts(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)
	rows := env.app.schedPreviewDialog.SplitDialog().rows
	gross, tax := rows[0].amountField.Value, rows[1].amountField.Value
	a := openPreviewLineAddNew(t, env.app, 1, "Dues")

	a = submitNewCategory(t, a)
	dues := findCategory(t, a, "Dues")

	p := a.schedPreviewDialog
	if a.createCat.dlg != nil || a.createCat.source() != createCatSourceNone {
		t.Error("the sub-dialog is still open after the save")
	}
	if p == nil || !p.IsVisible() {
		t.Fatal("the preview did not come back")
	}
	sd := p.SplitDialog()
	if got := lineCategoryID(sd, 1); got != dues.ID {
		t.Errorf("line 1 category = %v, want the new category", got)
	}
	if got := lineCategoryID(sd, 0); got != env.incomeCat.ID {
		t.Errorf("line 0 category = %v, want Salary kept", got)
	}
	if sd.rows[0].amountField.Value != gross || sd.rows[1].amountField.Value != tax {
		t.Errorf("amounts = %q, %q, want %q, %q unchanged",
			sd.rows[0].amountField.Value, sd.rows[1].amountField.Value, gross, tax)
	}
	if sd.rowIndex != 1 || sd.fieldFocus != splitFieldAmount {
		t.Errorf("focus = row %d field %v, want line 1 Amount", sd.rowIndex, sd.fieldFocus)
	}
	if !slices.Contains(p.categoryIDs, dues.ID) {
		t.Error("the preview's category list (used by a loan reseed) lacks the new category")
	}

	// Save with Enter on the Save button.
	sd.focus = splitFocusSaveBtn
	_, cmd := a.handleSchedulePreviewDialogKey(enterKey())
	if cmd == nil {
		t.Fatalf("Save returned no command; header error %q, split error %q",
			p.HeaderDialog().ErrorMsg(), sd.errorMsg)
	}
	if msg, ok := cmd().(errMsg); ok {
		t.Fatalf("post failed: %v", msg.err)
	}
	txns, err := env.txnRepo.ListByAccount(env.acct.ID)
	if err != nil || len(txns) != 1 {
		t.Fatalf("posted transactions = %d (err %v), want 1", len(txns), err)
	}
	splits, err := env.splitRepo.ListByTransaction(txns[0].ID)
	if err != nil {
		t.Fatalf("ListByTransaction: %v", err)
	}
	var onDues int
	for _, s := range splits {
		if s.CategoryID == dues.ID {
			onDues++
			if !s.Amount.Equal(types.MustNewMoney("-100.00")) {
				t.Errorf("the Dues line posted %s, want -100.00", s.Amount)
			}
		}
	}
	if onDues != 1 {
		t.Errorf("posted lines on the new category = %d, want 1", onDues)
	}
}

// Esc in the sub-dialog brings the preview back with its lines unchanged.
func TestSchedPreviewLine_AddNew_CancelRestores(t *testing.T) {
	env := newSchedulePreviewMultiLineEnv(t)
	tax := env.app.schedPreviewDialog.SplitDialog().rows[1].amountField.Value
	a := openPreviewLineAddNew(t, env.app, 1, "Dues")

	model, _ := a.handleCreateCatDialogKey(escapeKey())
	a = model.(*App)

	p := a.schedPreviewDialog
	if a.createCat.dlg != nil {
		t.Error("the sub-dialog is still open after Esc")
	}
	if p == nil || !p.IsVisible() {
		t.Fatal("the preview did not come back")
	}
	sd := p.SplitDialog()
	if got := lineCategoryID(sd, 1); got != env.taxCat.ID {
		t.Errorf("line 1 category = %v after Esc, want Federal Tax kept", got)
	}
	if got := sd.rows[1].amountField.Value; got != tax {
		t.Errorf("line 1 amount = %q after Esc, want %q", got, tax)
	}
	if !p.FocusOnSplits() {
		t.Error("the lines lost focus")
	}
}

// A loan-shaped preview rebuilds its lines from the loan balance, at Save or
// after a Date edit, unless the user edited them. A new category on the
// interest line is such an edit. "Accrued Interest" sorts before "Interest",
// so the line keeps its index in the rebuilt list; only the category ID shows
// the edit. "Mortgage Interest" sorts after it and moves the index.
func TestSchedPreviewLine_AddNew_LoanLineKeepsTheCategory(t *testing.T) {
	for _, name := range []string{"Accrued Interest", "Mortgage Interest"} {
		t.Run(name+"/save", func(t *testing.T) {
			env, a, created := loanLineWithNewCategory(t, name)

			sd := a.schedPreviewDialog.SplitDialog()
			sd.focus = splitFocusSaveBtn
			_, cmd := a.handleSchedulePreviewDialogKey(enterKey())
			if cmd == nil {
				t.Fatalf("Save returned no command; split error %q", sd.errorMsg)
			}
			if msg, ok := cmd().(errMsg); ok {
				t.Fatalf("post failed: %v", msg.err)
			}
			txns, err := env.txnRepo.ListByAccount(env.funding.ID)
			if err != nil || len(txns) != 1 {
				t.Fatalf("posted transactions = %d (err %v), want 1", len(txns), err)
			}
			splits, err := env.splitRepo.ListByTransaction(txns[0].ID)
			if err != nil {
				t.Fatalf("ListByTransaction: %v", err)
			}
			var onNew, onOld int
			for _, s := range splits {
				switch s.CategoryID {
				case created.ID:
					onNew++
				case env.interest.ID:
					onOld++
				}
			}
			if onNew != 1 || onOld != 0 {
				t.Errorf("posted lines: %d on %s, %d on Interest; want 1 and 0", onNew, name, onOld)
			}
		})

		t.Run(name+"/date edit", func(t *testing.T) {
			_, a, created := loanLineWithNewCategory(t, name)
			p := a.schedPreviewDialog

			// A Date edit in the header runs the reseed check, as a typed
			// date does (see TestSchedulePreview_LoanReseedsOnDateChange).
			p.setSplitFocus(false)
			p.HeaderDialog().SetFocusIndex(previewFieldDate)
			p.HeaderDialog().Fields()[previewFieldDate].Value = "08/01/2024"
			model, _ := a.handleSchedulePreviewDialogKey(tea.KeyPressMsg{Code: tea.KeyRight})
			a = model.(*App)

			if got := lineCategoryID(a.schedPreviewDialog.SplitDialog(), 0); got != created.ID {
				t.Errorf("after a Date edit the interest line is on %v, want %s kept", got, name)
			}
		})
	}
}

// loanLineWithNewCategory opens the loan preview and puts a new category
// named name on its interest line through [+ Add new category…].
func loanLineWithNewCategory(t *testing.T, name string) (*loanPreviewEnv, *App, *category.Category) {
	t.Helper()
	env := newLoanPreviewEnv(t, "250000.00", "6.5", "1580.17", types.NewDate(2024, time.July, 1))
	env.openPreview(t)
	if p := env.app.schedPreviewDialog; p == nil || !p.IsLoanShaped() {
		t.Fatal("the preview should be loan-shaped")
	}
	a := openPreviewLineAddNew(t, env.app, 0, name)
	a = submitNewCategory(t, a)
	created := findCategory(t, a, name)
	if got := lineCategoryID(a.schedPreviewDialog.SplitDialog(), 0); got != created.ID {
		t.Fatalf("the interest line did not take %s", name)
	}
	return env, a, created
}
