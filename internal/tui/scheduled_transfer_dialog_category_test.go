package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// =============================================================================
// Phase 8: Scheduled-transfer category combo (dialog + preview) + posting
// =============================================================================

// schedTransferCategoryEnv is a DuckDB-backed App wired for the scheduled-
// transfer category tests: two regular accounts and two non-system expense
// categories.
type schedTransferCategoryEnv struct {
	app     *App
	schedS  *scheduled.Service
	txnRepo *transaction.Repository
	from    *account.Account
	to      *account.Account
	catA    *category.Category
	catB    *category.Category
}

func newSchedTransferCategoryEnv(t *testing.T) *schedTransferCategoryEnv {
	t.Helper()
	database := dbtest.New(t)

	accountRepo := account.NewRepository(database)
	payeeRepo := payee.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	schedRepo := scheduled.NewRepository(database)
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, payeeRepo, accountRepo, database)
	schedSvc := scheduled.NewService(schedRepo, txnRepo, txnSvc, database, accountRepo)
	// Transfer occurrences post through the transfer owner; production wires
	// this in app.NewServices (see scheduled/transfer_port.go).
	transferSvc := transfer.NewService(txnRepo,
		investment.NewRepository(database), splitRepo, accountRepo,
		category.NewRepository(database), database)
	schedSvc.SetTransferPort(transferSvc)
	accountSvc := account.NewService(accountRepo, database)
	payeeSvc := payee.NewService(payeeRepo, database)
	categorySvc := category.NewService(categoryRepo, database)

	from := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(from); err != nil {
		t.Fatalf("create from: %v", err)
	}
	to := account.NewAccount("Visa", account.TypeCreditCard, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(to); err != nil {
		t.Fatalf("create to: %v", err)
	}
	catA := category.NewCategory("Bills", category.TypeExpense)
	if err := categoryRepo.Create(catA); err != nil {
		t.Fatalf("create catA: %v", err)
	}
	catB := category.NewCategory("Rent", category.TypeExpense)
	if err := categoryRepo.Create(catB); err != nil {
		t.Fatalf("create catB: %v", err)
	}

	app := &App{
		currentView:     ViewScheduled,
		keys:            defaultKeyMap(),
		menubar:         widget.NewMenuBar(),
		statusbar:       widget.NewStatusBar(),
		sidebar:         NewSidebar(),
		accountSvc:      accountSvc,
		payeeSvc:        payeeSvc,
		categorySvc:     categorySvc,
		transactionSvc:  txnSvc,
		transferSvc:     transferSvc,
		scheduledTxnSvc: schedSvc,
		undoManager:     undo.NewManager(),
	}
	return &schedTransferCategoryEnv{
		app: app, schedS: schedSvc, txnRepo: txnRepo,
		from: from, to: to, catA: catA, catB: catB,
	}
}

// TestApp_ScheduledTransferDialogDataMsg_BuildsCategoryCombo asserts the
// transfer branch of the data handler builds a dialog with a Category combo and
// stashes the parallel category-ID slice (excluding system categories).
func TestApp_ScheduledTransferDialogDataMsg_BuildsCategoryCombo(t *testing.T) {
	env := newSchedTransferCategoryEnv(t)

	data := &scheduledDialogData{
		mode:       scheduledDialogModeNew,
		isTransfer: true,
		accounts:   []*account.Account{env.from, env.to},
	}
	model, _ := env.app.Update(scheduledDialogDataMsg{data: data})
	updated := model.(*App)

	if updated.schedDialog == nil {
		t.Fatal("scheduled transfer dialog should be built")
	}
	fields := updated.schedDialog.Fields()
	if len(fields) != schedXferFieldCount {
		t.Fatalf("dialog fields = %d, want %d", len(fields), schedXferFieldCount)
	}
	if fields[schedXferFieldCategory].Label != "Category" {
		t.Errorf("field[%d] label = %q, want Category", schedXferFieldCategory, fields[schedXferFieldCategory].Label)
	}
	// "(None)" + two seeded categories.
	if len(updated.schedDialogCategoryIDs) != 3 {
		t.Fatalf("schedDialogCategoryIDs len = %d, want 3", len(updated.schedDialogCategoryIDs))
	}
	if !updated.schedDialogCategoryIDs[0].IsNil() {
		t.Error("category index 0 should be the NilID (None) sentinel")
	}
}

// TestApp_SubmitScheduledTransferDialog_New_ThreadsCategory drives the create
// dialog end-to-end and asserts the selected category persists on the schedule.
func TestApp_SubmitScheduledTransferDialog_New_ThreadsCategory(t *testing.T) {
	env := newSchedTransferCategoryEnv(t)
	app := env.app

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{env.catA, env.catB})
	app.schedDialogData = &scheduledDialogData{mode: scheduledDialogModeNew, isTransfer: true}
	app.schedDialogAccountIDs = []types.ID{env.from.ID, env.to.ID}
	app.schedDialogCategoryIDs = categoryIDs
	app.schedDialog = buildNewScheduledTransferDialog([]string{"Checking", "Visa"}, categoryOptions)

	fields := app.schedDialog.Fields()
	fields[schedXferFieldFrom].SelectedIndex = 0
	fields[schedXferFieldTo].SelectedIndex = 1
	fields[schedXferFieldAmount].Value = "200.00"
	// Select catB ("Rent") — categoryIDs = [(None), Bills, Rent].
	fields[schedXferFieldCategory].SelectedIndex = 2

	_, cmd := app.submitScheduledTransferDialog()
	if cmd == nil {
		t.Fatal("expected a save cmd")
	}
	cmd() // execute the async save

	all, err := env.schedS.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(all))
	}
	st := all[0]
	if !st.IsTransfer() {
		t.Fatal("saved schedule should be a transfer")
	}
	if !st.HasCategory() || st.CategoryID.ID != env.catB.ID {
		t.Errorf("schedule category = %+v, want %s (Rent)", st.CategoryID, env.catB.ID)
	}
}

// TestApp_SubmitScheduledTransferDialog_Edit_ClearsCategory edits an existing
// categorized transfer schedule and selects "(None)", clearing the label.
func TestApp_SubmitScheduledTransferDialog_Edit_ClearsCategory(t *testing.T) {
	env := newSchedTransferCategoryEnv(t)
	app := env.app

	// Seed a categorized transfer schedule.
	st := scheduled.NewTransactionWithAmount(env.from.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(env.to.ID)
	st.SetCategory(env.catA.ID)
	if err := env.schedS.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{env.catA, env.catB})
	app.schedDialogData = &scheduledDialogData{mode: scheduledDialogModeEdit, isTransfer: true, scheduled: st}
	app.schedDialogAccountIDs = []types.ID{env.from.ID, env.to.ID}
	app.schedDialogCategoryIDs = categoryIDs
	app.schedDialog = buildEditScheduledTransferDialog(st, []string{"Checking", "Visa"}, categoryOptions, []types.ID{env.from.ID, env.to.ID}, categoryIDs)

	fields := app.schedDialog.Fields()
	// The edit dialog should have seeded the category to Bills (index 1).
	if fields[schedXferFieldCategory].SelectedIndex != 1 {
		t.Fatalf("edit dialog seeded category index = %d, want 1", fields[schedXferFieldCategory].SelectedIndex)
	}
	fields[schedXferFieldCategory].SelectedIndex = 0 // clear to (None)

	_, cmd := app.submitScheduledTransferDialog()
	if cmd == nil {
		t.Fatal("expected a save cmd")
	}
	cmd()

	updated, err := env.schedS.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated.HasCategory() {
		t.Errorf("category should be cleared, got %+v", updated.CategoryID)
	}
}

// TestApp_OpenCreateCategorySubDialogFromSchedTransfer verifies the inline-
// creation divert from the scheduled transfer dialog records the correct source
// and hides the dialog, and that the applier lands the new category back.
func TestApp_OpenCreateCategorySubDialogFromSchedTransfer(t *testing.T) {
	env := newSchedTransferCategoryEnv(t)
	app := env.app

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{env.catA})
	app.schedDialogData = &scheduledDialogData{mode: scheduledDialogModeNew, isTransfer: true}
	app.schedDialogAccountIDs = []types.ID{env.from.ID, env.to.ID}
	app.schedDialogCategoryIDs = categoryIDs
	app.schedDialog = buildNewScheduledTransferDialog([]string{"Checking", "Visa"}, categoryOptions)
	app.schedDialog.Fields()[schedXferFieldCategory].Query = "Groceries"

	model, _ := app.openCreateCategorySubDialogFromSchedTransfer()
	updated := model.(*App)

	if updated.createCatSource != createCatSourceSchedTransferDialog {
		t.Errorf("createCatSource = %d, want createCatSourceSchedTransferDialog (%d)",
			updated.createCatSource, createCatSourceSchedTransferDialog)
	}
	if updated.createCatDialog == nil {
		t.Fatal("create-category sub-dialog should be open")
	}
	if updated.schedDialog.IsVisible() {
		t.Error("scheduled transfer dialog should be hidden during the divert")
	}

	// Apply a freshly-created category — it should land selected on the combo.
	groceries := category.NewCategory("Groceries", category.TypeExpense)
	updated.applyCreatedCategoryToSchedTransfer(groceries, []*category.Category{env.catA, groceries})

	catField := updated.schedDialog.Fields()[schedXferFieldCategory]
	if catField.Options[catField.SelectedIndex] != "Groceries" {
		t.Errorf("selected category = %q, want Groceries", catField.Options[catField.SelectedIndex])
	}
	if updated.createCatDialog != nil {
		t.Error("create-category sub-dialog should be cleared after apply")
	}
	if !updated.schedDialog.IsVisible() {
		t.Error("scheduled transfer dialog should be re-shown after apply")
	}
}

// TestSchedulePreviewDialog_TransferSeedsCategory asserts the transfer preview
// header seeds its Category combo from the template's category.
func TestSchedulePreviewDialog_TransferSeedsCategory(t *testing.T) {
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	visa := account.NewAccount("Visa", account.TypeCreditCard, "USD", types.ZeroMoney, types.Today())
	accts := []*account.Account{checking, visa}
	catID := types.NewID()

	st := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(visa.ID)
	st.SetCategory(catID)

	categoryOptions := []string{"(None)", "Bills"}
	categoryIDs := []types.ID{types.NilID, catID}

	p := NewSchedulePreviewDialog(st, accts, nil, categoryOptions, categoryIDs, nil)
	if p == nil || !p.IsTransfer() {
		t.Fatal("expected a transfer preview")
	}
	fields := p.HeaderDialog().Fields()
	if fields[previewXferFieldCategory].Label != "Category" {
		t.Fatalf("preview field[%d] = %q, want Category", previewXferFieldCategory, fields[previewXferFieldCategory].Label)
	}
	if fields[previewXferFieldCategory].SelectedIndex != 1 {
		t.Errorf("preview category index = %d, want 1", fields[previewXferFieldCategory].SelectedIndex)
	}
	if p.categoryFieldIndex() != previewXferFieldCategory {
		t.Errorf("categoryFieldIndex = %d, want %d", p.categoryFieldIndex(), previewXferFieldCategory)
	}
}

// TestApp_SubmitSchedulePreviewTransfer_OneOffCategory posts one occurrence of a
// categorized transfer schedule while relabeling it, and confirms the posted
// pair carries the one-off category while the template keeps its original.
func TestApp_SubmitSchedulePreviewTransfer_OneOffCategory(t *testing.T) {
	env := newSchedTransferCategoryEnv(t)
	app := env.app

	st := scheduled.NewTransactionWithAmount(env.from.ID, scheduled.FrequencyMonthly, types.Today(), types.MustNewMoney("-200.00"))
	st.SetTransfer(env.to.ID)
	st.SetCategory(env.catA.ID) // template label = Bills
	if err := env.schedS.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	categoryOptions, categoryIDs := buildCategoryOptions([]*category.Category{env.catA, env.catB})
	p := NewSchedulePreviewDialog(st, []*account.Account{env.from, env.to}, nil, categoryOptions, categoryIDs, nil)
	app.schedPreviewDialog = p

	// Relabel this occurrence to Rent (index 2 in [(None), Bills, Rent]).
	p.HeaderDialog().Fields()[previewXferFieldCategory].SelectedIndex = 2

	_, cmd := app.submitSchedulePreviewDialog()
	if cmd == nil {
		t.Fatal("expected a post cmd")
	}
	cmd()

	// Posted From leg carries the one-off category (Rent).
	rows, err := env.txnRepo.ListByAccount(env.from.ID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 posted row, got %d", len(rows))
	}
	if !rows[0].HasCategory() || rows[0].CategoryID.ID != env.catB.ID {
		t.Errorf("posted leg category = %+v, want %s (Rent)", rows[0].CategoryID, env.catB.ID)
	}

	// Template keeps its original label (Bills) — the edit was one-off.
	updated, err := env.schedS.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !updated.HasCategory() || updated.CategoryID.ID != env.catA.ID {
		t.Errorf("template category = %+v, want %s (Bills, unchanged)", updated.CategoryID, env.catA.ID)
	}
}
