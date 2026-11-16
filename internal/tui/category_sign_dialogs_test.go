package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// The category sign rule (specs/transactions.md, "Category Sign Rule") wired
// into each dialog: the New/Edit Transaction dialog, the New/Edit Scheduled
// Transaction dialog, the Post Scheduled preview and the split editor.

var (
	signTab      = tea.KeyPressMsg{Code: tea.KeyTab}
	signShiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	signEnter    = tea.KeyPressMsg{Code: tea.KeyEnter}
	signBksp     = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

// signFixture is a category list with one of each kind the rule tells apart.
type signFixture struct {
	groceries, dining, salary, valueAdj *category.Category
	cats                                []*category.Category
	kroger                              *payee.Payee
}

func newSignFixture() signFixture {
	f := signFixture{
		groceries: category.NewCategory("Groceries", category.TypeExpense),
		dining:    category.NewCategory("Dining", category.TypeExpense),
		salary:    category.NewCategory("Salary", category.TypeIncome),
		valueAdj:  category.NewCategory(category.ValueAdjustmentCategoryName, category.TypeExpense),
	}
	f.valueAdj.IsSystem = true
	f.cats = []*category.Category{f.groceries, f.dining, f.salary, f.valueAdj}
	f.kroger = payee.NewPayee("Kroger")
	f.kroger.DefaultCategoryID = types.NullableID{ID: f.groceries.ID, Valid: true}
	return f
}

// --- New/Edit Transaction ----------------------------------------------------

// newSignTxnApp opens the Transaction dialog through applyData, as the data
// load does. existing nil opens New; selected decides whether Value
// Adjustment is offered.
func newSignTxnApp(t *testing.T, f signFixture, existing *transaction.Transaction, selected *account.Account) *App {
	t.Helper()
	a := newModalTestApp()
	data := &transactionDialogData{
		categories: f.cats,
		payees:     []*payee.Payee{f.kroger},
		payeeMap:   map[string]*payee.Payee{"kroger": f.kroger},
	}
	if existing != nil {
		data.mode = transactionDialogModeEdit
		data.existing = existing
	}
	a.txn.applyData(data, selected, types.ZeroDate)
	return a
}

func txnType(a *App, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		a.handleTransactionDialogKey(k)
	}
}

// pickTxnCategory moves focus to Category, types query and commits it with
// Tab, the way a user picks by keyboard. Focus ends on Amount.
func pickTxnCategory(t *testing.T, a *App, query string) {
	t.Helper()
	a.txn.dlg.SetFocusIndex(2)
	txnType(a, typed(query)...)
	txnType(a, signTab)
}

func txnAmount(a *App) *dialog.Field { return a.txn.dlg.Fields()[3] }

func TestTxnSign_ExpensePickFillsMinusBeforeDigits(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	pickTxnCategory(t, a, "Groc")

	amt := txnAmount(a)
	if amt.Value != "-" {
		t.Fatalf("Amount after Expense pick = %q, want \"-\"", amt.Value)
	}
	if a.txn.dlg.FocusIndex() != 3 {
		t.Fatalf("focus = %d, want Amount (3)", a.txn.dlg.FocusIndex())
	}
	txnType(a, typed("52.10")...)
	if amt.Value != "-52.10" {
		t.Errorf("typed digits after the fill = %q, want -52.10", amt.Value)
	}
	// The old habit of typing the "-" changes nothing.
	amt.Value = "-"
	amt.MoveCursorEnd()
	txnType(a, typed("-52.10")...)
	if amt.Value != "-52.10" {
		t.Errorf("typed \"-52.10\" over the fill = %q, want -52.10", amt.Value)
	}
}

func TestTxnSign_ExpensePickNegatesTypedAmount(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(3)
	txnType(a, typed("52.10")...)
	pickTxnCategory(t, a, "Groc")
	if got := txnAmount(a).Value; got != "-52.10" {
		t.Errorf("Amount = %q, want -52.10", got)
	}
}

func TestTxnSign_IncomePickMirrors(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	pickTxnCategory(t, a, "Groc")
	pickTxnCategory(t, a, "Sal")
	if got := txnAmount(a).Value; got != "" {
		t.Errorf("lone \"-\" under Income = %q, want empty", got)
	}

	pickTxnCategory(t, a, "Groc")
	txnType(a, typed("52.10")...)
	pickTxnCategory(t, a, "Sal")
	if got := txnAmount(a).Value; got != "52.10" {
		t.Errorf("typed -52.10 under Income = %q, want 52.10", got)
	}
}

// Re-committing the same category (Tab back through the combo) is not a
// change, so a refund typed after the fill keeps its sign.
func TestTxnSign_RefundSurvivesRecommit(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	pickTxnCategory(t, a, "Groc")
	txnType(a, signBksp)
	txnType(a, typed("20")...)
	txnType(a, signShiftTab, signTab) // back to Category, commit, forward to Amount
	if got := txnAmount(a).Value; got != "20" {
		t.Errorf("refund after re-commit = %q, want 20", got)
	}
}

// A "+" typed over the fill gives a positive refund in one key.
func TestTxnSign_PlusOverFillGivesRefund(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	pickTxnCategory(t, a, "Groc")
	txnType(a, typed("+20")...)
	if got := txnAmount(a).Value; got != "+20" {
		t.Fatalf("Amount = %q, want +20", got)
	}
	m, err := parseAmountInput(txnAmount(a).Value)
	if err != nil || !m.IsPositive() {
		t.Errorf("+20 parses to %v, %v; want a positive amount", m, err)
	}
}

// Edit: the amount the dialog loaded keeps its sign through a recategorize;
// an amount the user changed takes the rule.
func TestTxnSign_EditKeepsLoadedAmount(t *testing.T) {
	f := newSignFixture()
	existing := transaction.NewTransactionFull(types.NewID(), types.Today(), types.MustNewMoney("20"),
		types.NilID, f.groceries.ID, "")
	a := newSignTxnApp(t, f, existing, nil)

	pickTxnCategory(t, a, "Din")
	if got := txnAmount(a).Value; got != "20" {
		t.Fatalf("loaded refund after recategorize = %q, want 20", got)
	}

	txnType(a, signBksp, signBksp)
	txnType(a, typed("25")...)
	pickTxnCategory(t, a, "Groc")
	if got := txnAmount(a).Value; got != "-25" {
		t.Errorf("edited amount after recategorize = %q, want -25", got)
	}
}

func TestTxnSign_SystemCategoryOnlyClearsLoneMinus(t *testing.T) {
	asset := &account.Account{Type: account.TypeAsset}
	a := newSignTxnApp(t, newSignFixture(), nil, asset)
	pickTxnCategory(t, a, "Groc")
	pickTxnCategory(t, a, "Value Adj")
	if got := txnAmount(a).Value; got != "" {
		t.Errorf("lone \"-\" under Value Adjustment = %q, want empty", got)
	}
	txnType(a, typed("-300")...)
	pickTxnCategory(t, a, "Groc")
	pickTxnCategory(t, a, "Value Adj")
	if got := txnAmount(a).Value; got != "-300" {
		t.Errorf("-300 under Value Adjustment = %q, want -300", got)
	}
}

func TestTxnSign_PayeeAutoFillAppliesRule(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(1)
	txnType(a, typed("Kroger")...)
	if got := a.txn.dlg.Fields()[2].SelectedOption(); got != "Groceries" {
		t.Fatalf("auto-filled category = %q, want Groceries", got)
	}
	if got := txnAmount(a).Value; got != "-" {
		t.Errorf("Amount after payee auto-fill = %q, want \"-\"", got)
	}
}

// Moving focus back to Payee must not put the payee default over the
// category the user picked (and so must not flip the sign again).
func TestTxnSign_PayeeRefocusKeepsUserCategory(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(1)
	txnType(a, typed("Kroger")...)
	txnType(a, signTab) // to Category
	txnType(a, typed("Sal")...)
	txnType(a, signTab)                   // commit Salary, focus Amount
	txnType(a, signShiftTab, signShiftTab) // back to Payee
	if got := a.txn.dlg.Fields()[2].SelectedOption(); got != "Salary" {
		t.Errorf("category after refocusing Payee = %q, want Salary", got)
	}
}

// Edit: Tab from Date into a pre-filled Payee is not a payee change.
func TestTxnSign_EditPayeeFocusKeepsSavedCategory(t *testing.T) {
	f := newSignFixture()
	existing := transaction.NewTransactionFull(types.NewID(), types.Today(), types.MustNewMoney("1000"),
		f.kroger.ID, f.salary.ID, "")
	a := newSignTxnApp(t, f, existing, nil)
	a.txn.dlg.SetFocusIndex(0)
	txnType(a, signTab)
	if got := a.txn.dlg.Fields()[2].SelectedOption(); got != "Salary" {
		t.Errorf("category after Tab into Payee = %q, want Salary", got)
	}
}

// Edit of an uncategorized transaction (an import): the payee default still
// fills when focus reaches Payee, and the loaded amount keeps its sign.
func TestTxnSign_EditUncategorizedFillsPayeeDefault(t *testing.T) {
	f := newSignFixture()
	existing := transaction.NewTransactionFull(types.NewID(), types.Today(), types.MustNewMoney("20"),
		f.kroger.ID, types.NilID, "")
	a := newSignTxnApp(t, f, existing, nil)
	a.txn.dlg.SetFocusIndex(0)
	txnType(a, signTab)
	if got := a.txn.dlg.Fields()[2].SelectedOption(); got != "Groceries" {
		t.Errorf("category after Tab into Payee = %q, want the payee default Groceries", got)
	}
	if got := txnAmount(a).Value; got != "20" {
		t.Errorf("loaded amount = %q, want 20", got)
	}
}

// A Save click can commit a category in the same event. Save never changes
// the sign, and the category is recorded so a later key does not apply it.
func TestTxnSign_SubmitRecordsWithoutApplying(t *testing.T) {
	f := newSignFixture()
	a := newSignTxnApp(t, f, nil, nil)
	fields := a.txn.dlg.Fields()
	fields[0].Value = "13/45/2024" // invalid date keeps the dialog open
	fields[3].Value = "52.10"
	fields[2].Query = "Groc"
	fields[2].ComboHighlight = 0
	fields[2].CommitComboHighlight() // what the Save click's blur commit does

	a.transactionDialogAction(dialog.DialogActionSubmit)
	if a.txn.dlg == nil {
		t.Fatal("dialog closed; the invalid date should keep it open")
	}
	if got := fields[3].Value; got != "52.10" {
		t.Fatalf("Amount after Save = %q, want 52.10 (Save never changes the sign)", got)
	}
	txnType(a, signTab)
	if got := fields[3].Value; got != "52.10" {
		t.Errorf("Amount after the next key = %q, want 52.10", got)
	}
}

func TestTxnSign_InlineCreateAppliesRule(t *testing.T) {
	f := newSignFixture()
	a := newSignTxnApp(t, f, nil, nil)
	txnAmount(a).Value = "40"
	fuel := category.NewCategory("Fuel", category.TypeExpense)
	cats := append(append([]*category.Category{}, f.cats...), fuel)
	a.applyCreatedCategoryToTxn(fuel, cats)
	if got := txnAmount(a).Value; got != "-40" {
		t.Errorf("Amount after inline create of an Expense = %q, want -40", got)
	}
}

// Cancel of the inline create must leave the previous selection: the next Tab
// used to commit the row under the stale add-new highlight.
func TestTxnAddNewCancel_KeepsSelectionOnNextTab(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	cat := a.txn.dlg.Fields()[2]
	a.txn.dlg.SetFocusIndex(2)
	txnType(a, typed("Groc")...)
	cat.ComboHighlight = len(cat.FilteredIndices()) // the add-new row
	txnType(a, signEnter)
	if a.createCat.dlg == nil {
		t.Fatal("Enter on the add-new row did not open the create-category dialog")
	}
	a.handleCreateCatDialogKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	txnType(a, signTab)
	if cat.SelectedIndex != 0 {
		t.Errorf("after Cancel and Tab, category = %q, want (None)", cat.SelectedOption())
	}
	if got := txnAmount(a).Value; got != "" {
		t.Errorf("Amount = %q, want empty", got)
	}
}

// A category picked by a mouse click on the combo panel takes the rule, the
// same as one picked by keyboard.
func TestTxnSign_MousePickAppliesRule(t *testing.T) {
	a := newSignTxnApp(t, newSignFixture(), nil, nil)
	a.txn.dlg.SetFocusIndex(2)
	txnType(a, typed("Groc")...)
	rendered := a.txn.dlg.Render(a.styles)
	startCol, startRow, _, _ := a.txn.dlg.DialogBounds(a.width, a.height)
	row, col := findLabel(t, rendered, "Groceries")
	a.handleMouseEvent(tea.MouseClickMsg{X: startCol + col + 1, Y: startRow + row, Button: tea.MouseLeft})
	if got := a.txn.dlg.Fields()[2].SelectedOption(); got != "Groceries" {
		t.Fatalf("mouse pick = %q, want Groceries", got)
	}
	if got := txnAmount(a).Value; got != "-" {
		t.Errorf("Amount after a mouse pick = %q, want \"-\"", got)
	}
}

// --- New/Edit Scheduled Transaction -------------------------------------------

func newSignSchedApp(t *testing.T, f signFixture, existing *scheduled.Transaction) *App {
	t.Helper()
	a := newModalTestApp()
	acct := &account.Account{BaseModel: types.BaseModel{ID: types.NewID()}, Name: "Checking",
		Type: account.TypeChecking, Active: true}
	data := &scheduledDialogData{
		mode:     scheduledDialogModeNew,
		accounts: []*account.Account{acct},
		payeeMap: map[string]*payee.Payee{},
	}
	if existing != nil {
		existing.AccountID = acct.ID
		data.mode = scheduledDialogModeEdit
		data.scheduled = existing
	}
	a.sched.applyData(data, f.cats)
	return a
}

func schedType(a *App, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		a.handleScheduledDialogKey(k)
	}
}

func pickSchedCategory(a *App, query string) {
	a.sched.dlg.SetFocusIndex(schedFieldCategory)
	schedType(a, typed(query)...)
	schedType(a, signTab)
}

func TestSchedSign_PickFillsAndLoneMinusSavesVariable(t *testing.T) {
	a := newSignSchedApp(t, newSignFixture(), nil)
	amt := a.sched.dlg.Fields()[schedFieldAmount]
	if !amt.SignedAmount {
		t.Error("schedule Amount is not a SignedAmount field")
	}
	pickSchedCategory(a, "Groc")
	if amt.Value != "-" {
		t.Fatalf("Amount after Expense pick = %q, want \"-\"", amt.Value)
	}
	a.submitScheduledDialog()
	if amt.Error != "" {
		t.Errorf("a lone \"-\" at Save gave Amount error %q; it must save as variable", amt.Error)
	}
}

func TestSchedSign_TypedAmountTakesRule(t *testing.T) {
	a := newSignSchedApp(t, newSignFixture(), nil)
	pickSchedCategory(a, "Groc")
	schedType(a, typed("120")...)
	pickSchedCategory(a, "Sal")
	if got := a.sched.dlg.Fields()[schedFieldAmount].Value; got != "120" {
		t.Errorf("-120 under Income = %q, want 120", got)
	}
}

// Edit of a variable bill: the empty Amount means "variable", so the form
// does not fill it as it opens, and the first key is not a category change.
func TestSchedSign_EditVariableBillOpensEmpty(t *testing.T) {
	f := newSignFixture()
	st := &scheduled.Transaction{
		BaseModel:  types.BaseModel{ID: types.NewID()},
		Frequency:  scheduled.FrequencyMonthly,
		Interval:   1,
		StartDate:  types.Today(),
		NextDate:   types.Today(),
		CategoryID: types.NullableID{ID: f.groceries.ID, Valid: true},
	}
	a := newSignSchedApp(t, f, st)
	amt := a.sched.dlg.Fields()[schedFieldAmount]
	if amt.Value != "" {
		t.Fatalf("variable bill opened with Amount %q, want empty", amt.Value)
	}
	a.sched.dlg.SetFocusIndex(schedFieldPayee)
	schedType(a, typed("x")...)
	if amt.Value != "" {
		t.Errorf("Amount after a Payee key = %q, want empty", amt.Value)
	}
}

func TestSchedSign_MousePickAppliesRule(t *testing.T) {
	a := newSignSchedApp(t, newSignFixture(), nil)
	a.height = 70 // the whole form fits, so the panel is on screen
	a.sched.dlg.SetFocusIndex(schedFieldCategory)
	schedType(a, typed("Groc")...)
	rendered := a.sched.dlg.Render(a.styles)
	startCol, startRow, _, _ := a.sched.dlg.DialogBounds(a.width, a.height)
	row, col := findLabel(t, rendered, "Groceries")
	a.handleMouseEvent(tea.MouseClickMsg{X: startCol + col + 1, Y: startRow + row, Button: tea.MouseLeft})
	if got := a.sched.dlg.Fields()[schedFieldCategory].SelectedOption(); got != "Groceries" {
		t.Fatalf("mouse pick = %q, want Groceries", got)
	}
	if got := a.sched.dlg.Fields()[schedFieldAmount].Value; got != "-" {
		t.Errorf("Amount after a mouse pick = %q, want \"-\"", got)
	}
}

// A Save click that commits a category records it without the rule; the
// next key does not apply it either.
func TestSchedSign_SubmitRecordsWithoutApplying(t *testing.T) {
	a := newSignSchedApp(t, newSignFixture(), nil)
	fields := a.sched.dlg.Fields()
	fields[schedFieldInterval].Value = "0" // invalid interval keeps the dialog open
	amt := fields[schedFieldAmount]
	amt.Value = "52.10"
	cat := fields[schedFieldCategory]
	cat.Query = "Groc"
	cat.ComboHighlight = 0
	cat.CommitComboHighlight() // what the Save click's blur commit does

	a.scheduledDialogAction(dialog.DialogActionSubmit)
	if a.sched.dlg == nil {
		t.Fatal("dialog closed; the invalid interval should keep it open")
	}
	if amt.Value != "52.10" {
		t.Fatalf("Amount after Save = %q, want 52.10 (Save never changes the sign)", amt.Value)
	}
	schedType(a, signTab)
	if amt.Value != "52.10" {
		t.Errorf("Amount after the next key = %q, want 52.10", amt.Value)
	}
}

func TestSchedSign_InlineCreateAppliesRule(t *testing.T) {
	f := newSignFixture()
	a := newSignSchedApp(t, f, nil)
	a.sched.dlg.Fields()[schedFieldAmount].Value = "75"
	fuel := category.NewCategory("Fuel", category.TypeExpense)
	a.applyCreatedCategoryToSched(fuel, append(append([]*category.Category{}, f.cats...), fuel))
	if got := a.sched.dlg.Fields()[schedFieldAmount].Value; got != "-75" {
		t.Errorf("Amount after inline create of an Expense = %q, want -75", got)
	}
}

// --- Post Scheduled preview ---------------------------------------------------

func newSignPreview(f signFixture, template *scheduled.Transaction) *SchedulePreviewDialog {
	options, ids := buildCategoryOptions(f.cats)
	p := NewSchedulePreviewDialog(template, nil, nil, options, ids, nil)
	p.SetCategories(f.cats)
	p.applyOpenSign()
	return p
}

func signTemplate(catID types.ID, amount string) *scheduled.Transaction {
	st := &scheduled.Transaction{
		BaseModel:  types.BaseModel{ID: types.NewID()},
		AccountID:  types.NewID(),
		Frequency:  scheduled.FrequencyMonthly,
		Interval:   1,
		StartDate:  types.Today(),
		NextDate:   types.Today(),
		CategoryID: types.NullableID{ID: catID, Valid: true},
	}
	if amount != "" {
		st.Amount = types.NullableMoney{Money: types.MustNewMoney(amount), Valid: true}
	}
	return st
}

func TestPreviewSign_VariableBillOpensWithMinus(t *testing.T) {
	f := newSignFixture()
	p := newSignPreview(f, signTemplate(f.groceries.ID, ""))
	amt := p.HeaderDialog().Fields()[previewSingleFieldAmount]
	if amt.Value != "-" || amt.CursorPos() != 1 {
		t.Fatalf("variable bill Amount = %q cursor %d, want \"-\" cursor 1", amt.Value, amt.CursorPos())
	}
	if !amt.SignedAmount {
		t.Error("preview Amount is not a SignedAmount field")
	}
}

func TestPreviewSign_OpenKeepsStoredAmountAndIncome(t *testing.T) {
	f := newSignFixture()
	p := newSignPreview(f, signTemplate(f.groceries.ID, "15"))
	if got := p.HeaderDialog().Fields()[previewSingleFieldAmount].Value; got != "15" {
		t.Errorf("stored positive amount under Expense = %q, want 15", got)
	}
	p = newSignPreview(f, signTemplate(f.salary.ID, ""))
	if got := p.HeaderDialog().Fields()[previewSingleFieldAmount].Value; got != "" {
		t.Errorf("variable Income bill = %q, want empty", got)
	}
}

func TestPreviewSign_CategoryPickAppliesRule(t *testing.T) {
	f := newSignFixture()
	a := newModalTestApp()
	a.schedPreviewDialog = newSignPreview(f, signTemplate(f.groceries.ID, ""))
	header := a.schedPreviewDialog.HeaderDialog()
	header.SetFocusIndex(previewSingleFieldCat)
	for _, k := range append(typed("Sal"), signTab) {
		a.handleSchedulePreviewDialogKey(k)
	}
	if got := header.Fields()[previewSingleFieldAmount].Value; got != "" {
		t.Errorf("lone \"-\" after an Income pick = %q, want empty", got)
	}
}

// The transfer preview has its own field layout (index 3 is Memo); the rule
// must leave it alone, at open and on a category pick.
func TestPreviewSign_TransferUntouched(t *testing.T) {
	f := newSignFixture()
	st := signTemplate(f.groceries.ID, "100")
	st.TransferAccountID = types.NullableID{ID: types.NewID(), Valid: true}
	a := newModalTestApp()
	a.schedPreviewDialog = newSignPreview(f, st)
	header := a.schedPreviewDialog.HeaderDialog()
	fields := header.Fields()
	if fields[previewXferFieldAmount].Value != "100" || fields[previewXferFieldMemo].Value != "" {
		t.Fatalf("transfer preview opened with Amount %q Memo %q, want 100 and empty",
			fields[previewXferFieldAmount].Value, fields[previewXferFieldMemo].Value)
	}

	header.SetFocusIndex(previewXferFieldCategory)
	for _, k := range append(typed("Din"), signTab) {
		a.handleSchedulePreviewDialogKey(k)
	}
	if got := fields[previewXferFieldCategory].SelectedOption(); got != "Dining" {
		t.Fatalf("category = %q, want Dining", got)
	}
	if fields[previewXferFieldAmount].Value != "100" || fields[previewXferFieldMemo].Value != "" {
		t.Errorf("after a category pick: Amount %q Memo %q, want 100 and empty",
			fields[previewXferFieldAmount].Value, fields[previewXferFieldMemo].Value)
	}
}

// A Save click that commits a category records it without the rule; the
// next key does not apply it either.
func TestPreviewSign_SubmitRecordsWithoutApplying(t *testing.T) {
	f := newSignFixture()
	a := newModalTestApp()
	a.schedPreviewDialog = newSignPreview(f, signTemplate(f.salary.ID, ""))
	header := a.schedPreviewDialog.HeaderDialog()
	fields := header.Fields()
	fields[previewFieldDate].Value = "13/45/2024" // invalid date keeps the dialog open
	amt := fields[previewSingleFieldAmount]
	amt.Value = "52.10"
	cat := fields[previewSingleFieldCat]
	cat.Query = "Groc"
	cat.ComboHighlight = 0
	cat.CommitComboHighlight() // what the Save click's blur commit does

	a.schedulePreviewAction(dialog.DialogActionSubmit)
	if a.schedPreviewDialog == nil {
		t.Fatal("preview closed; the invalid date should keep it open")
	}
	if amt.Value != "52.10" {
		t.Fatalf("Amount after Save = %q, want 52.10 (Save never changes the sign)", amt.Value)
	}
	a.handleSchedulePreviewDialogKey(signTab)
	if amt.Value != "52.10" {
		t.Errorf("Amount after the next key = %q, want 52.10", amt.Value)
	}
}

// A click outside the multi-line preview does nothing. It used to take header
// focus, which committed an open split picker and ran the rule on its line.
func TestPreviewSign_MultiLineOutsideClickCommitsNothing(t *testing.T) {
	f := newSignFixture()
	st := signTemplate(types.NilID, "-100")
	st.CategoryID = types.NullableID{}
	st.Splits = append(st.Splits,
		scheduled.NewCategorizedSplit(st.ID, f.groceries.ID, types.MustNewMoney("-60")),
		scheduled.NewCategorizedSplit(st.ID, f.dining.ID, types.MustNewMoney("-40")))
	a := newModalTestApp()
	a.schedPreviewDialog = newSignPreview(f, st)
	clickLabel(t, a, "[+ Add split]")
	sd := a.schedPreviewDialog.SplitDialog()
	for _, k := range typed("Gr") {
		a.handleKeyPress(k)
	}
	_, y := screenCellOf(t, a.viewContent(), "Memo:")
	a.handleMouseEvent(tea.MouseClickMsg{X: 1, Y: y, Button: tea.MouseLeft})

	row := sd.rows[2]
	if got := sd.categoryOptions[row.categoryIndex]; got != "(None)" || row.amountField.Value != "" {
		t.Errorf("click outside the dialog changed the new line: category %q Amount %q, want (None) and empty",
			got, row.amountField.Value)
	}
}

// End to end: the loader carries the category list to the preview, so a due
// variable bill opens with "-".
func TestPreviewSign_LoaderCarriesCategories(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)
	env.dueTxn.Amount = types.NullableMoney{}
	msg, ok := env.app.loadSchedulePreviewData()().(schedulePreviewDataMsg)
	if !ok {
		t.Fatal("loadSchedulePreviewData did not return schedulePreviewDataMsg")
	}
	env.app.Update(msg)
	got := env.app.schedPreviewDialog.HeaderDialog().Fields()[previewSingleFieldAmount].Value
	if got != "-" {
		t.Errorf("due variable Rent bill opened with Amount %q, want \"-\"", got)
	}
}

// --- Split editor ---------------------------------------------------------------

func newSignSplit(f signFixture, seed []*transaction.Split) *SplitDialog {
	options, ids := buildCategoryOptionsFor(f.cats, true)
	var sd *SplitDialog
	if seed != nil {
		sd = NewSplitDialogFromExisting(types.MustNewMoney("-100"), options, ids, seed)
	} else {
		sd = NewSplitDialog(types.MustNewMoney("-100"), options, ids)
	}
	sd.SetTransferTargets([]string{"Savings"}, []types.ID{types.NewID()}, types.NilID)
	sd.SetCategories(f.cats)
	return sd
}

func splitType(sd *SplitDialog, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		sd.HandleKey(k)
	}
}

func TestSplitSign_RowPickFillsMinus(t *testing.T) {
	sd := newSignSplit(newSignFixture(), nil)
	splitType(sd, typed("Groc")...)
	splitType(sd, signTab)
	row := &sd.rows[0]
	if row.amountField.Value != "-" {
		t.Fatalf("row Amount after Expense pick = %q, want \"-\"", row.amountField.Value)
	}
	if !row.amountField.SignedAmount {
		t.Error("row Amount is not a SignedAmount field")
	}
	splitType(sd, typed("-60")...)
	if row.amountField.Value != "-60" {
		t.Errorf("typed -60 over the fill = %q, want -60", row.amountField.Value)
	}
}

func TestSplitSign_SeededRowKeepsLoadedAmount(t *testing.T) {
	f := newSignFixture()
	seed := []*transaction.Split{{CategoryID: f.salary.ID, Amount: types.MustNewMoney("1000")}}
	sd := newSignSplit(f, seed)
	splitType(sd, typed("Groc")...)
	splitType(sd, signTab)
	if got := sd.rows[0].amountField.Value; got != "1000" {
		t.Errorf("seeded 1000 after recategorize = %q, want 1000", got)
	}
}

// A transfer pick never takes the rule, also from a line that was on an
// Expense category.
func TestSplitSign_TransferPickUntouched(t *testing.T) {
	sd := newSignSplit(newSignFixture(), nil)
	splitType(sd, typed("Groc")...)
	splitType(sd, signTab)
	sd.rows[0].amountField.Value = "50"
	sd.fieldFocus = splitFieldCategory
	splitType(sd, typed("Savings")...)
	splitType(sd, signTab)
	if !sd.rows[0].transferMode {
		t.Fatal("the pick did not make a transfer line")
	}
	if got := sd.rows[0].amountField.Value; got != "50" {
		t.Errorf("transfer line Amount = %q, want 50", got)
	}
}

// A Save click commits an open picker in the same click; Save never changes
// the sign.
func TestSplitSign_SaveClickSkipsRule(t *testing.T) {
	sd := newSignSplit(newSignFixture(), nil)
	sd.rows[0].amountField.Value = "100"
	splitType(sd, typed("Groc")...) // picker open with Groceries highlighted
	app, overlay, startCol, startRow := splitDialogMouseEnv(t, sd)
	x, y := findRenderedText(t, overlay, "[ Save ]", startCol, startRow)
	app.handleMouseEvent(tea.MouseClickMsg{X: x + 2, Y: y, Button: tea.MouseLeft})
	if got := sd.categoryOptions[sd.rows[0].categoryIndex]; got != "Groceries" {
		t.Fatalf("Save click kept category %q, want the picked Groceries", got)
	}
	if got := sd.rows[0].amountField.Value; got != "100" {
		t.Errorf("Amount after a Save click = %q, want 100", got)
	}
}

// A category picked by a mouse click on the picker panel takes the rule, the
// same as one picked by keyboard.
func TestSplitSign_MousePickAppliesRule(t *testing.T) {
	sd := newSignSplit(newSignFixture(), nil)
	splitType(sd, typed("Groc")...)
	app, overlay, startCol, startRow := splitDialogMouseEnv(t, sd)
	x, y := findRenderedText(t, overlay, "Groceries", startCol, startRow)
	app.handleMouseEvent(tea.MouseClickMsg{X: x + 1, Y: y, Button: tea.MouseLeft})
	if got := sd.categoryOptions[sd.rows[0].categoryIndex]; got != "Groceries" {
		t.Fatalf("mouse pick = %q, want Groceries", got)
	}
	if got := sd.rows[0].amountField.Value; got != "-" {
		t.Errorf("row Amount after a mouse pick = %q, want \"-\"", got)
	}
}

func TestSplitSign_InlineCreateAppliesRuleKeepsVAAndFocusesAmount(t *testing.T) {
	f := newSignFixture()
	sd := newSignSplit(f, nil)
	vaIdx := -1
	for i, o := range sd.categoryOptions {
		if o == category.ValueAdjustmentCategoryName {
			vaIdx = i
		}
	}
	if vaIdx < 0 {
		t.Fatal("fixture editor does not offer Value Adjustment")
	}
	sd.addRow()
	sd.rows[1].categoryIndex = vaIdx
	sd.rows[0].amountField.Value = "30"

	s := &splitSurface{editor: sd}
	fuel := category.NewCategory("Fuel", category.TypeExpense)
	s.applyCreatedCategory(fuel, append(append([]*category.Category{}, f.cats...), fuel), 0)

	if got := sd.rows[0].amountField.Value; got != "-30" {
		t.Errorf("origin row Amount = %q, want -30", got)
	}
	if got := sd.categoryOptions[sd.rows[1].categoryIndex]; got != category.ValueAdjustmentCategoryName {
		t.Errorf("other row's category after the create = %q, want Value Adjustment", got)
	}
	if sd.fieldFocus != splitFieldAmount {
		t.Errorf("focus after inline create = %v, want Amount", sd.fieldFocus)
	}
}
