package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Phase 5: Transfer-dialog category combo + inline creation
// =============================================================================

// TestEditTransferIncludesCategory pins which edit-mode shapes carry a Category
// combo: every transfer with a regular-side leg (bank↔bank, inv↔reg) does;
// inv↔inv (neither leg storable) does not; nil data does not.
func TestEditTransferIncludesCategory(t *testing.T) {
	bankID := types.NewID()
	invID := types.NewID()
	inv2ID := types.NewID()
	accts := []*account.Account{
		{BaseModel: types.BaseModel{ID: bankID}, Type: account.TypeChecking},
		{BaseModel: types.BaseModel{ID: invID}, Type: account.TypeInvestment},
		{BaseModel: types.BaseModel{ID: inv2ID}, Type: account.TypeInvestment},
	}

	tests := []struct {
		name string
		data *transferDialogData
		want bool
	}{
		{"nil", nil, false},
		{"bank↔bank", &transferDialogData{accounts: accts, existing: &transaction.TransferPair{}}, true},
		{
			"inv→reg (bank leg storable)",
			&transferDialogData{accounts: accts, existingInvestment: &investmentTransferEdit{fromAccountID: invID, toAccountID: bankID}},
			true,
		},
		{
			"reg→inv (bank leg storable)",
			&transferDialogData{accounts: accts, existingInvestment: &investmentTransferEdit{fromAccountID: bankID, toAccountID: invID}},
			true,
		},
		{
			"inv↔inv (neither storable)",
			&transferDialogData{accounts: accts, existingInvestment: &investmentTransferEdit{fromAccountID: invID, toAccountID: inv2ID}},
			false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := editTransferIncludesCategory(tc.data); got != tc.want {
				t.Errorf("editTransferIncludesCategory = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCategoryComboIndex(t *testing.T) {
	a := types.NewID()
	b := types.NewID()
	ids := []types.ID{types.NilID, a, b} // index 0 is the "(None)" sentinel

	if got := categoryComboIndex(ids, types.NullableID{}); got != 0 {
		t.Errorf("unset category → index %d, want 0", got)
	}
	if got := categoryComboIndex(ids, types.NullableID{ID: b, Valid: true}); got != 2 {
		t.Errorf("category b → index %d, want 2", got)
	}
	if got := categoryComboIndex(ids, types.NullableID{ID: types.NewID(), Valid: true}); got != 0 {
		t.Errorf("unknown category → index %d, want 0 (falls back to None)", got)
	}
}

func TestTransferCategoryFieldIndex(t *testing.T) {
	createApp := &App{transferDialogData: &transferDialogData{mode: transferDialogModeNew}}
	if got := createApp.transferCategoryFieldIndex(); got != 5 {
		t.Errorf("create-mode category field index = %d, want 5", got)
	}

	// bank↔bank edit carries a Category combo at index 3.
	bankEdit := &App{transferDialogData: &transferDialogData{
		mode:     transferDialogModeEdit,
		existing: &transaction.TransferPair{},
	}}
	if got := bankEdit.transferCategoryFieldIndex(); got != 3 {
		t.Errorf("bank↔bank edit category field index = %d, want 3", got)
	}

	// inv↔inv edit has no Category field → -1 (never points at the Status radio).
	invID := types.NewID()
	inv2ID := types.NewID()
	invInvEdit := &App{transferDialogData: &transferDialogData{
		mode: transferDialogModeEdit,
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: invID}, Type: account.TypeInvestment},
			{BaseModel: types.BaseModel{ID: inv2ID}, Type: account.TypeInvestment},
		},
		existingInvestment: &investmentTransferEdit{fromAccountID: invID, toAccountID: inv2ID},
	}}
	if got := invInvEdit.transferCategoryFieldIndex(); got != -1 {
		t.Errorf("inv↔inv edit category field index = %d, want -1 (no combo)", got)
	}
}

// TestApp_Update_TransferDialogDataMsg_BuildsCategoryCombo asserts the create
// dialog built by the data message includes a Category combo and that the
// parallel category-ID slice is stashed on the App for the submit handler.
func TestApp_Update_TransferDialogDataMsg_BuildsCategoryCombo(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	bills := category.NewCategory("Bills", category.TypeExpense)

	app := &App{
		currentView: ViewRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
	}
	data := &transferDialogData{
		accounts: []*account.Account{
			{BaseModel: types.BaseModel{ID: fromID}, Name: "Checking", Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: toID}, Name: "Savings", Type: account.TypeSavings},
		},
		accountIDs: []types.ID{fromID, toID},
		categories: []*category.Category{bills},
	}

	model, _ := app.Update(transferDialogDataMsg{data: data})
	updated := model.(*App)

	if updated.transferDialog == nil {
		t.Fatal("transfer dialog should be built")
	}
	fields := updated.transferDialog.Fields()
	if len(fields) != 6 {
		t.Fatalf("create dialog fields = %d, want 6 (incl Category)", len(fields))
	}
	if fields[5].Label != "Category" {
		t.Errorf("field[5] label = %q, want Category", fields[5].Label)
	}
	// "(None)" + "Bills"
	if len(updated.transferDialogCategoryIDs) != 2 {
		t.Fatalf("transferDialogCategoryIDs len = %d, want 2", len(updated.transferDialogCategoryIDs))
	}
	if !updated.transferDialogCategoryIDs[0].IsNil() {
		t.Error("category index 0 should be the NilID (None) sentinel")
	}
	if updated.transferDialogCategoryIDs[1] != bills.ID {
		t.Error("category index 1 should be the Bills category ID")
	}
}

// TestApp_SubmitTransferDialog_InvToInvRejectsCategory: an inv↔inv transfer has
// nowhere to store a category, so selecting one and submitting is refused with
// an inline error and the dialog stays open.
func TestApp_SubmitTransferDialog_InvToInvRejectsCategory(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()
	catID := types.NewID()

	accountOptions := []string{"IRA A", "IRA B"}
	catOptions := []string{"(None)", "Bills"}
	app := &App{
		currentView:              ViewRegister,
		keys:                     defaultKeyMap(),
		menubar:                  widget.NewMenuBar(),
		statusbar:                widget.NewStatusBar(),
		sidebar:                  NewSidebar(),
		transferDialog:           buildTransferDialog(accountOptions, catOptions, 0),
		transferDialogAccountIDs: []types.ID{fromID, toID},
		transferDialogCategoryIDs: []types.ID{
			types.NilID,
			catID,
		},
		transferDialogData: &transferDialogData{
			mode: transferDialogModeNew,
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "IRA A", Type: account.TypeInvestment},
				{BaseModel: types.BaseModel{ID: toID}, Name: "IRA B", Type: account.TypeInvestment},
			},
		},
	}
	fields := app.transferDialog.Fields()
	fields[1].SelectedIndex = 1 // To = IRA B
	fields[2].Value = "1000.00"
	fields[3].Value = "01/15/2024"
	fields[5].SelectedIndex = 1 // Category = Bills

	model, cmd := app.submitTransferDialog()
	updated := model.(*App)

	if cmd != nil {
		t.Error("expected nil cmd: a categorized inv↔inv transfer must be refused")
	}
	if updated.transferDialog == nil {
		t.Fatal("dialog should stay open on the validation error")
	}
	if updated.transferDialog.Fields()[5].Error == "" {
		t.Error("Category field should carry the inv↔inv limitation error")
	}
}

// TestApp_SubmitTransferDialog_InvToInvAllowsNoCategory: the guard only fires
// when a category is selected — an uncategorized inv↔inv transfer submits
// normally.
func TestApp_SubmitTransferDialog_InvToInvAllowsNoCategory(t *testing.T) {
	fromID := types.NewID()
	toID := types.NewID()

	accountOptions := []string{"IRA A", "IRA B"}
	catOptions := []string{"(None)", "Bills"}
	app := &App{
		currentView:               ViewRegister,
		keys:                      defaultKeyMap(),
		menubar:                   widget.NewMenuBar(),
		statusbar:                 widget.NewStatusBar(),
		sidebar:                   NewSidebar(),
		transferDialog:            buildTransferDialog(accountOptions, catOptions, 0),
		transferDialogAccountIDs:  []types.ID{fromID, toID},
		transferDialogCategoryIDs: []types.ID{types.NilID, types.NewID()},
		transferDialogData: &transferDialogData{
			mode: transferDialogModeNew,
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "IRA A", Type: account.TypeInvestment},
				{BaseModel: types.BaseModel{ID: toID}, Name: "IRA B", Type: account.TypeInvestment},
			},
		},
	}
	fields := app.transferDialog.Fields()
	fields[1].SelectedIndex = 1
	fields[2].Value = "1000.00"
	fields[3].Value = "01/15/2024"
	fields[5].SelectedIndex = 0 // (None)

	_, cmd := app.submitTransferDialog()
	if cmd == nil {
		t.Fatal("an uncategorized inv↔inv transfer should submit (non-nil cmd)")
	}
}

// newTransferCategoryTestApp builds a DuckDB-backed App wired with the account,
// transaction, and category services plus an undo manager, seeded with two
// checking accounts and one non-system expense category. Used for the
// end-to-end category-threading tests.
func newTransferCategoryTestApp(t *testing.T) (*App, *transaction.Service, *account.Account, *account.Account, *category.Category) {
	t.Helper()
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	accountSvc := account.NewService(accountRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	from := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(from); err != nil {
		t.Fatalf("create from account: %v", err)
	}
	to := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(to); err != nil {
		t.Fatalf("create to account: %v", err)
	}
	bills := category.NewCategory("Bills", category.TypeExpense)
	if err := categoryRepo.Create(bills); err != nil {
		t.Fatalf("create category: %v", err)
	}

	app := &App{
		currentView:    ViewRegister,
		keys:           defaultKeyMap(),
		menubar:        widget.NewMenuBar(),
		statusbar:      widget.NewStatusBar(),
		sidebar:        NewSidebar(),
		accountSvc:     accountSvc,
		categorySvc:    categorySvc,
		transactionSvc: txnSvc,
		undoManager:    undo.NewManager(),
	}
	return app, txnSvc, from, to, bills
}

// TestApp_SubmitTransferDialog_RegToReg_ThreadsCategory drives the create
// dialog end-to-end and asserts the selected category lands on both legs of
// the resulting transfer pair.
func TestApp_SubmitTransferDialog_RegToReg_ThreadsCategory(t *testing.T) {
	app, txnSvc, from, to, bills := newTransferCategoryTestApp(t)

	data := &transferDialogData{
		accounts:   []*account.Account{from, to},
		accountIDs: []types.ID{from.ID, to.ID},
		categories: []*category.Category{bills},
	}
	model, _ := app.Update(transferDialogDataMsg{data: data})
	app = model.(*App)

	catIdx := categoryComboIndex(app.transferDialogCategoryIDs, types.NullableID{ID: bills.ID, Valid: true})
	if catIdx == 0 {
		t.Fatal("Bills category should resolve to a non-zero combo index")
	}

	fields := app.transferDialog.Fields()
	fields[0].SelectedIndex = 0 // From = Checking
	fields[1].SelectedIndex = 1 // To = Savings
	fields[2].Value = "500.00"
	fields[3].Value = types.Today().Time().Format("01/02/2006")
	fields[4].Value = "cc payment"
	fields[5].SelectedIndex = catIdx

	model, cmd := app.submitTransferDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("submit should return a non-nil cmd")
	}
	if app.transferDialog != nil {
		t.Error("dialog should close after a valid submit")
	}
	if msg := cmd(); isErrMsg(msg) {
		t.Fatalf("create cmd returned an error: %+v", msg)
	}

	legs, err := txnSvc.ListByAccount(from.ID)
	if err != nil {
		t.Fatalf("list from-account transactions: %v", err)
	}
	var transferID types.ID
	for _, l := range legs {
		if l.TransferID.Valid {
			transferID = l.TransferID.ID
			break
		}
	}
	if transferID.IsNil() {
		t.Fatal("no transfer leg found in the From account")
	}

	pair, err := txnSvc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("GetTransferPair: %v", err)
	}
	if !pair.FromTransaction.CategoryID.Valid || pair.FromTransaction.CategoryID.ID != bills.ID {
		t.Errorf("From leg category = %+v, want Bills", pair.FromTransaction.CategoryID)
	}
	if !pair.ToTransaction.CategoryID.Valid || pair.ToTransaction.CategoryID.ID != bills.ID {
		t.Errorf("To leg category = %+v, want Bills", pair.ToTransaction.CategoryID)
	}
}

// TestApp_SubmitEditTransferDialog_RegToReg_SetsThenClearsCategory drives the
// edit dialog end-to-end: first assigning a category to an uncategorized
// transfer, then clearing it back to none. Both legs mirror each change.
func TestApp_SubmitEditTransferDialog_RegToReg_SetsThenClearsCategory(t *testing.T) {
	app, txnSvc, from, to, bills := newTransferCategoryTestApp(t)

	pair, err := txnSvc.CreateTransfer(from.ID, to.ID, types.Today(), types.MustNewMoney("500.00"), "", types.NullableID{})
	if err != nil {
		t.Fatalf("seed transfer: %v", err)
	}
	transferID := pair.FromTransaction.TransferID.ID
	fromLegID := pair.FromTransaction.ID

	// --- Phase 1: assign the Bills category through the edit dialog. ---
	editMsg := app.loadEditTransferDialogData(fromLegID)()
	if isErrMsg(editMsg) {
		t.Fatalf("load edit dialog: %+v", editMsg)
	}
	model, _ := app.Update(editMsg)
	app = model.(*App)

	fields := app.transferDialog.Fields()
	if len(fields) != 5 {
		t.Fatalf("edit dialog fields = %d, want 5 (Amount, Date, Memo, Category, Status)", len(fields))
	}
	catIdx := categoryComboIndex(app.transferDialogCategoryIDs, types.NullableID{ID: bills.ID, Valid: true})
	if catIdx == 0 {
		t.Fatal("Bills should resolve to a non-zero combo index")
	}
	fields[3].SelectedIndex = catIdx // Category

	model, cmd := app.submitTransferDialog()
	app = model.(*App)
	if cmd == nil {
		t.Fatal("edit submit should return a non-nil cmd")
	}
	if msg := cmd(); isErrMsg(msg) {
		t.Fatalf("edit cmd returned an error: %+v", msg)
	}

	got, err := txnSvc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("GetTransferPair after set: %v", err)
	}
	if !got.FromTransaction.CategoryID.Valid || got.FromTransaction.CategoryID.ID != bills.ID {
		t.Errorf("From leg category after set = %+v, want Bills", got.FromTransaction.CategoryID)
	}
	if !got.ToTransaction.CategoryID.Valid || got.ToTransaction.CategoryID.ID != bills.ID {
		t.Errorf("To leg category after set = %+v, want Bills", got.ToTransaction.CategoryID)
	}

	// --- Phase 2: clear the category back to "(None)". ---
	editMsg = app.loadEditTransferDialogData(fromLegID)()
	if isErrMsg(editMsg) {
		t.Fatalf("reload edit dialog: %+v", editMsg)
	}
	model, _ = app.Update(editMsg)
	app = model.(*App)

	// The dialog should seed the combo at the Bills index; clear it to 0.
	if seeded := app.transferDialog.Fields()[3].SelectedIndex; seeded != catIdx {
		t.Errorf("edit dialog should seed the existing category (index %d), got %d", catIdx, seeded)
	}
	app.transferDialog.Fields()[3].SelectedIndex = 0 // (None)

	_, cmd = app.submitTransferDialog()
	if cmd == nil {
		t.Fatal("clear submit should return a non-nil cmd")
	}
	if msg := cmd(); isErrMsg(msg) {
		t.Fatalf("clear cmd returned an error: %+v", msg)
	}

	got, err = txnSvc.GetTransferPair(transferID)
	if err != nil {
		t.Fatalf("GetTransferPair after clear: %v", err)
	}
	if got.FromTransaction.CategoryID.Valid {
		t.Errorf("From leg category after clear = %+v, want cleared", got.FromTransaction.CategoryID)
	}
	if got.ToTransaction.CategoryID.Valid {
		t.Errorf("To leg category after clear = %+v, want cleared", got.ToTransaction.CategoryID)
	}
}

// TestApp_TransferDialog_AddNew_OpensCreateCategoryDialog: activating the
// Category combo's [+ Add new category…] row diverts to the create-category
// sub-dialog, keeps the transfer dialog alive but hidden, records the transfer
// source, and defaults the new category to Expense (transfers carry no
// income/expense signal in their always-positive amount).
func TestApp_TransferDialog_AddNew_OpensCreateCategoryDialog(t *testing.T) {
	app := newAppForTransferAddNew(t, "Donations")

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransferDialogKey(enter)
	updated := model.(*App)

	if updated.createCatDialog == nil || !updated.createCatDialog.IsVisible() {
		t.Fatal("createCatDialog should be visible after [+ Add new] is activated")
	}
	if updated.createCatSource != createCatSourceTransferDialog {
		t.Errorf("createCatSource = %v, want createCatSourceTransferDialog", updated.createCatSource)
	}
	if updated.transferDialog == nil || updated.transferDialog.IsVisible() {
		t.Error("transfer dialog should be kept but hidden during the divert")
	}
	// Type radio: 0 = Expense.
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("new-category Type = %d, want 0 (Expense default for transfers)", got)
	}
	// Name seeded from the typed query.
	if got := updated.createCatDialog.Fields()[0].Value; got != "Donations" {
		t.Errorf("new-category Name seed = %q, want %q", got, "Donations")
	}
}

// TestApp_TransferDialog_AddNew_CancelRestores: cancelling the sub-dialog
// re-shows the transfer dialog and resets the source discriminator.
func TestApp_TransferDialog_AddNew_CancelRestores(t *testing.T) {
	app := newAppForTransferAddNew(t, "")

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransferDialogKey(enter)
	app = model.(*App)

	esc := tea.KeyPressMsg{Code: tea.KeyEsc}
	model, _ = app.handleCreateCatDialogKey(esc)
	app = model.(*App)

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after cancel")
	}
	if app.transferDialog == nil || !app.transferDialog.IsVisible() {
		t.Error("transfer dialog should be re-shown after cancel")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %v, want createCatSourceNone after cancel", app.createCatSource)
	}
}

// TestApp_ApplyCreatedCategoryToTransfer_SelectsNewCategory: after the new
// category persists, the router applier reloads the Category combo options and
// selects the freshly-created category, then re-shows the transfer dialog.
func TestApp_ApplyCreatedCategoryToTransfer_SelectsNewCategory(t *testing.T) {
	app := newAppForTransferAddNew(t, "Donations")
	// Enter the divert so the transfer dialog is hidden.
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransferDialogKey(enter)
	app = model.(*App)

	donations := category.NewCategory("Donations", category.TypeExpense)
	cats := append(app.transferDialogData.categories, donations)

	app.applyCreatedCategoryToTransfer(donations, cats)

	if app.createCatDialog != nil {
		t.Error("createCatDialog should be cleared after applying the new category")
	}
	if app.transferDialog == nil || !app.transferDialog.IsVisible() {
		t.Fatal("transfer dialog should be re-shown after applying the new category")
	}
	catField := app.transferDialog.Fields()[5]
	sel := catField.SelectedIndex
	if sel <= 0 || sel >= len(app.transferDialogCategoryIDs) {
		t.Fatalf("Category SelectedIndex = %d, want it to point at the new category", sel)
	}
	if app.transferDialogCategoryIDs[sel] != donations.ID {
		t.Error("Category combo should select the freshly-created Donations category")
	}
}

// newAppForTransferAddNew builds a create-mode transfer dialog with the
// Category combo focused and its highlight parked on the [+ Add new category…]
// action row, so pressing Enter triggers the inline-creation divert.
func newAppForTransferAddNew(t *testing.T, query string) *App {
	t.Helper()
	fromID := types.NewID()
	toID := types.NewID()
	bills := category.NewCategory("Bills", category.TypeExpense)
	cats := []*category.Category{bills}
	options, ids := buildCategoryOptions(cats)

	app := &App{
		currentView:               ViewRegister,
		keys:                      defaultKeyMap(),
		menubar:                   widget.NewMenuBar(),
		statusbar:                 widget.NewStatusBar(),
		sidebar:                   NewSidebar(),
		transferDialogAccountIDs:  []types.ID{fromID, toID},
		transferDialogCategoryIDs: ids,
		transferDialogData: &transferDialogData{
			mode:       transferDialogModeNew,
			categories: cats,
			accounts: []*account.Account{
				{BaseModel: types.BaseModel{ID: fromID}, Name: "Checking", Type: account.TypeChecking},
				{BaseModel: types.BaseModel{ID: toID}, Name: "Savings", Type: account.TypeSavings},
			},
		},
	}
	d := buildTransferDialog([]string{"Checking", "Savings"}, options, 0)
	d.SetFocusIndex(5) // Category
	cat := d.Fields()[5]
	cat.Query = query
	cat.ComboHighlight = len(cat.FilteredIndices())
	app.transferDialog = d
	return app
}

// isErrMsg reports whether msg is the App's error message type.
func isErrMsg(msg tea.Msg) bool {
	_, ok := msg.(errMsg)
	return ok
}
