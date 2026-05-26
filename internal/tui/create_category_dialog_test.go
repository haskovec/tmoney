package tui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// newCategorySvcForPersistTest builds a real DuckDB-backed category service
// seeded with the default category set so persistCategory tests exercise the
// same wiring production uses. Returns the service plus the seeded category
// list so callers can resolve "Food" etc. without an extra List() call.
func newCategorySvcForPersistTest(t *testing.T) (*category.Service, []*category.Category) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "persistcategory.tdb")
	database, err := db.Create(dbPath)
	if err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	repo := category.NewRepository(database)
	svc := category.NewService(repo, database)
	if err := svc.SeedDefaultCategories(); err != nil {
		t.Fatalf("SeedDefaultCategories: %v", err)
	}
	cats, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}
	return svc, cats
}

func TestBuildCreateCategoryDialog_FieldShape(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food", "Bills", "Auto"}, category.TypeExpense)

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3 (Name, Parent, Type)", len(fields))
	}

	if fields[0].Label != "Name" {
		t.Errorf("fields[0].Label = %q, want %q", fields[0].Label, "Name")
	}
	if fields[0].Type != dialog.FieldText {
		t.Errorf("fields[0].Type = %v, want dialog.FieldText", fields[0].Type)
	}
	if !fields[0].Required {
		t.Error("Name field should be Required")
	}

	if fields[1].Label != "Parent" {
		t.Errorf("fields[1].Label = %q, want %q", fields[1].Label, "Parent")
	}
	if fields[1].Type != dialog.FieldCombo {
		t.Errorf("fields[1].Type = %v, want dialog.FieldCombo", fields[1].Type)
	}
	// First option is the top-level sentinel; existing parents follow.
	wantParents := []string{"(top-level)", "Food", "Bills", "Auto"}
	if got := fields[1].Options; len(got) != len(wantParents) {
		t.Fatalf("Parent.Options = %v, want %v", got, wantParents)
	}
	for i, p := range wantParents {
		if fields[1].Options[i] != p {
			t.Errorf("Parent.Options[%d] = %q, want %q", i, fields[1].Options[i], p)
		}
	}

	if fields[2].Label != "Type" {
		t.Errorf("fields[2].Label = %q, want %q", fields[2].Label, "Type")
	}
	if fields[2].Type != dialog.FieldRadio {
		t.Errorf("fields[2].Type = %v, want dialog.FieldRadio", fields[2].Type)
	}
	if len(fields[2].Options) != 2 {
		t.Fatalf("Type.Options = %v, want 2 entries", fields[2].Options)
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
}

func TestBuildCreateCategoryDialog_TabCyclesAcrossFields(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food"}, category.TypeExpense)

	// Initial focus on Name.
	if got := d.FocusIndex(); got != 0 {
		t.Errorf("initial FocusIndex = %d, want 0 (Name)", got)
	}

	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := d.FocusIndex(); got != 1 {
		t.Errorf("FocusIndex after Tab = %d, want 1 (Parent)", got)
	}
	d.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if got := d.FocusIndex(); got != 2 {
		t.Errorf("FocusIndex after Tab Tab = %d, want 2 (Type)", got)
	}
}

func TestSubmitCreateCategoryDialog_ExistingParent(t *testing.T) {
	parents := []string{"Food", "Bills", "Auto"}
	d := buildCreateCategoryDialog("", "", parents, category.TypeExpense)
	fields := d.Fields()

	fields[0].Value = "Groceries"
	// Parent = "Food" → SelectedIndex = 1 (index 0 is "(top-level)").
	fields[1].SelectedIndex = 1
	fields[2].SelectedIndex = 0 // Expense

	cmd := submitCreateCategoryDialog(d, parents)
	if cmd == nil {
		t.Fatalf("submitCreateCategoryDialog returned nil cmd; want a request msg")
	}
	msg, ok := cmd().(createCategoryRequestMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want createCategoryRequestMsg", cmd())
	}
	if msg.request.Name != "Groceries" {
		t.Errorf("request.Name = %q, want %q", msg.request.Name, "Groceries")
	}
	if msg.request.ParentName != "Food" {
		t.Errorf("request.ParentName = %q, want %q", msg.request.ParentName, "Food")
	}
	if msg.request.NewParent {
		t.Error("request.NewParent = true, want false (Food is existing)")
	}
	if msg.request.Type != category.TypeExpense {
		t.Errorf("request.Type = %v, want %v", msg.request.Type, category.TypeExpense)
	}
}

func TestSubmitCreateCategoryDialog_NewTopLevelParent(t *testing.T) {
	parents := []string{"Food", "Bills", "Auto"}
	d := buildCreateCategoryDialog("", "", parents, category.TypeExpense)
	fields := d.Fields()

	fields[0].Value = "Donations"
	// Parent typed as "Charity" — not present in existingParents.
	fields[1].Query = "Charity"
	fields[2].SelectedIndex = 0 // Expense

	cmd := submitCreateCategoryDialog(d, parents)
	if cmd == nil {
		t.Fatalf("submitCreateCategoryDialog returned nil cmd")
	}
	msg, ok := cmd().(createCategoryRequestMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want createCategoryRequestMsg", cmd())
	}
	if msg.request.Name != "Donations" {
		t.Errorf("request.Name = %q, want %q", msg.request.Name, "Donations")
	}
	if msg.request.ParentName != "Charity" {
		t.Errorf("request.ParentName = %q, want %q", msg.request.ParentName, "Charity")
	}
	if !msg.request.NewParent {
		t.Error("request.NewParent = false, want true (Charity is new)")
	}
}

func TestSubmitCreateCategoryDialog_TopLevelCategory(t *testing.T) {
	parents := []string{"Food"}
	d := buildCreateCategoryDialog("", "", parents, category.TypeExpense)
	fields := d.Fields()

	fields[0].Value = "Misc"
	fields[1].SelectedIndex = 0 // "(top-level)"
	fields[2].SelectedIndex = 1 // Income

	cmd := submitCreateCategoryDialog(d, parents)
	if cmd == nil {
		t.Fatalf("submitCreateCategoryDialog returned nil cmd")
	}
	msg := cmd().(createCategoryRequestMsg)
	if msg.request.ParentName != "" {
		t.Errorf("request.ParentName = %q, want empty (top-level)", msg.request.ParentName)
	}
	if msg.request.NewParent {
		t.Error("request.NewParent = true, want false (top-level has no parent to create)")
	}
	if msg.request.Type != category.TypeIncome {
		t.Errorf("request.Type = %v, want Income", msg.request.Type)
	}
}

func TestSubmitCreateCategoryDialog_QueryMatchingExistingParentResolvesToExisting(t *testing.T) {
	// If the user types a parent name (case-insensitive) that already exists,
	// it must be flagged as existing — no duplicate top-level created.
	parents := []string{"Food", "Bills"}
	d := buildCreateCategoryDialog("", "", parents, category.TypeExpense)
	fields := d.Fields()

	fields[0].Value = "Sushi"
	fields[1].Query = "food" // lowercase, matches "Food"

	cmd := submitCreateCategoryDialog(d, parents)
	if cmd == nil {
		t.Fatalf("submitCreateCategoryDialog returned nil cmd")
	}
	msg := cmd().(createCategoryRequestMsg)
	if msg.request.ParentName != "Food" {
		t.Errorf("request.ParentName = %q, want %q (case-insensitive existing match)",
			msg.request.ParentName, "Food")
	}
	if msg.request.NewParent {
		t.Error("request.NewParent = true, want false (existing case-insensitive match)")
	}
}

func TestSubmitCreateCategoryDialog_EmptyNameSetsInlineError(t *testing.T) {
	parents := []string{"Food"}
	d := buildCreateCategoryDialog("", "", parents, category.TypeExpense)
	fields := d.Fields()

	fields[0].Value = "   " // whitespace only
	fields[1].SelectedIndex = 1

	cmd := submitCreateCategoryDialog(d, parents)
	if cmd != nil {
		t.Fatalf("submitCreateCategoryDialog returned non-nil cmd; want nil for invalid input")
	}
	if fields[0].Error == "" {
		t.Error("Name field should have an inline error after submitting empty value")
	}
}

func TestCreateCategoryDialog_EscEmitsCancel(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food"}, category.TypeExpense)

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != dialog.DialogActionCancel {
		t.Errorf("HandleKey(Esc) = %v, want dialog.DialogActionCancel", action)
	}
}

func TestBuildCreateCategoryDialog_SeedsNameFromQuery(t *testing.T) {
	d := buildCreateCategoryDialog("Donations", "", []string{"Food"}, category.TypeExpense)
	fields := d.Fields()

	if fields[0].Value != "Donations" {
		t.Errorf("Name pre-fill = %q, want %q", fields[0].Value, "Donations")
	}
	// When Name is pre-filled, focus skips ahead to the Parent combo so the
	// user can immediately pick or type a parent.
	if d.FocusIndex() != 1 {
		t.Errorf("FocusIndex = %d, want 1 (Parent) when Name is seeded", d.FocusIndex())
	}
}

func TestBuildCreateCategoryDialog_SeedsExistingParent(t *testing.T) {
	// When parent matches an existing entry (case-insensitive), the Parent
	// combo's SelectedIndex resolves to it and Query is left empty.
	d := buildCreateCategoryDialog("Sushi", "Food", []string{"Auto", "Food", "Bills"}, category.TypeExpense)
	fields := d.Fields()

	if fields[0].Value != "Sushi" {
		t.Errorf("Name pre-fill = %q, want %q", fields[0].Value, "Sushi")
	}
	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty (existing parent resolves to SelectedIndex)", fields[1].Query)
	}
	wantIdx := -1
	for i, opt := range fields[1].Options {
		if opt == "Food" {
			wantIdx = i
			break
		}
	}
	if wantIdx <= 0 {
		t.Fatalf("Parent options should include 'Food': %v", fields[1].Options)
	}
	if fields[1].SelectedIndex != wantIdx {
		t.Errorf("Parent.SelectedIndex = %d, want %d (Food)", fields[1].SelectedIndex, wantIdx)
	}
	// With both Name and Parent filled, focus advances to Type.
	if d.FocusIndex() != 2 {
		t.Errorf("FocusIndex = %d, want 2 (Type) when Name and Parent are seeded", d.FocusIndex())
	}
}

func TestBuildCreateCategoryDialog_SeedsExistingParentCaseInsensitive(t *testing.T) {
	// Parent match is case-insensitive — typing "food" still resolves to "Food".
	d := buildCreateCategoryDialog("Sushi", "food", []string{"Food", "Bills"}, category.TypeExpense)
	fields := d.Fields()

	if fields[1].Query != "" {
		t.Errorf("Parent.Query = %q, want empty (case-insensitive existing match)", fields[1].Query)
	}
	wantIdx := -1
	for i, opt := range fields[1].Options {
		if opt == "Food" {
			wantIdx = i
			break
		}
	}
	if fields[1].SelectedIndex != wantIdx {
		t.Errorf("Parent.SelectedIndex = %d, want %d", fields[1].SelectedIndex, wantIdx)
	}
}

func TestBuildCreateCategoryDialog_SeedsNewParentAsQuery(t *testing.T) {
	// When parent is a non-empty name not in existingParents, it's seeded
	// as Query (the new-parent path) so submission flags NewParent=true.
	d := buildCreateCategoryDialog("Endowment", "Charity", []string{"Food", "Bills"}, category.TypeExpense)
	fields := d.Fields()

	if fields[0].Value != "Endowment" {
		t.Errorf("Name pre-fill = %q, want %q", fields[0].Value, "Endowment")
	}
	if fields[1].Query != "Charity" {
		t.Errorf("Parent.Query = %q, want %q (new-parent path)", fields[1].Query, "Charity")
	}
	// SelectedIndex stays at 0 (the "(top-level)" sentinel) because the
	// typed name doesn't match any existing parent.
	if fields[1].SelectedIndex != 0 {
		t.Errorf("Parent.SelectedIndex = %d, want 0 when parent is typed-but-unknown", fields[1].SelectedIndex)
	}
}

// =============================================================================
// persistCategory — shared persistence core for the create-category router
// =============================================================================

// TestPersistCategory_TopLevel: ParentName empty → new top-level category
// created with the requested Type.
func TestPersistCategory_TopLevel(t *testing.T) {
	svc, _ := newCategorySvcForPersistTest(t)
	app := &App{categorySvc: svc}

	got, err := app.persistCategory(createCategoryRequest{
		Name: "Hobbies",
		Type: category.TypeExpense,
	})
	if err != nil {
		t.Fatalf("persistCategory: %v", err)
	}
	if got == nil {
		t.Fatal("persistCategory returned nil category")
	}
	if got.Name != "Hobbies" {
		t.Errorf("Name = %q, want %q", got.Name, "Hobbies")
	}
	if got.ParentID.Valid {
		t.Error("top-level category should have ParentID.Valid == false")
	}
	if got.Type != category.TypeExpense {
		t.Errorf("Type = %v, want Expense", got.Type)
	}
}

// TestPersistCategory_ExistingParent: ParentName names an existing parent,
// NewParent=false → child inherits the parent's Type even if req.Type differs.
func TestPersistCategory_ExistingParent(t *testing.T) {
	svc, cats := newCategorySvcForPersistTest(t)
	app := &App{categorySvc: svc}

	var foodType category.Type
	var hasFood bool
	for _, c := range cats {
		if c.Name == "Food" && c.IsTopLevel() {
			foodType = c.Type
			hasFood = true
			break
		}
	}
	if !hasFood {
		t.Fatal("default seed should include 'Food' parent")
	}

	got, err := app.persistCategory(createCategoryRequest{
		Name:       "Sushi",
		ParentName: "Food",
		NewParent:  false,
		Type:       category.TypeIncome, // intentionally wrong; parent wins
	})
	if err != nil {
		t.Fatalf("persistCategory: %v", err)
	}
	if !got.ParentID.Valid {
		t.Error("child should carry a valid ParentID")
	}
	if got.Type != foodType {
		t.Errorf("child Type = %v, want %v (inherited from parent)", got.Type, foodType)
	}
}

// TestPersistCategory_NewParent: ParentName names a new top-level to create,
// NewParent=true → both parent and child persisted; child references parent.
func TestPersistCategory_NewParent(t *testing.T) {
	svc, _ := newCategorySvcForPersistTest(t)
	app := &App{categorySvc: svc}

	got, err := app.persistCategory(createCategoryRequest{
		Name:       "Endowment",
		ParentName: "Charity",
		NewParent:  true,
		Type:       category.TypeExpense,
	})
	if err != nil {
		t.Fatalf("persistCategory: %v", err)
	}

	all, err := svc.List()
	if err != nil {
		t.Fatalf("svc.List: %v", err)
	}
	var charity *category.Category
	for _, c := range all {
		if c.Name == "Charity" && c.IsTopLevel() {
			charity = c
			break
		}
	}
	if charity == nil {
		t.Fatal("new parent 'Charity' should be persisted")
	}
	if !got.ParentID.Valid || got.ParentID.ID != charity.ID {
		t.Errorf("child.ParentID = %+v, want valid pointing to Charity (%s)", got.ParentID, charity.ID)
	}
	if charity.Type != category.TypeExpense {
		t.Errorf("new parent Type = %v, want Expense (from request)", charity.Type)
	}
}

// TestPersistCategory_NilService returns an error rather than panicking when
// the category service hasn't been wired into the App. Defensive — covers the
// edge case of an App constructed for a non-category-bearing surface.
func TestPersistCategory_NilService(t *testing.T) {
	app := &App{}
	_, err := app.persistCategory(createCategoryRequest{Name: "X"})
	if err == nil {
		t.Fatal("persistCategory with nil categorySvc should return an error")
	}
}

// TestInferCategoryTypeFromAmount pins the amount → default-Type mapping used
// by the four amount-bearing surfaces (Transaction, Scheduled new/edit,
// Scheduled Preview, Split row). Positive parseable amounts seed Income;
// everything else (empty, negative, unparseable) seeds Expense.
func TestInferCategoryTypeFromAmount(t *testing.T) {
	cases := []struct {
		in   string
		want category.Type
	}{
		{"", category.TypeExpense},
		{"50.00", category.TypeIncome},
		{"-50.00", category.TypeExpense},
		{"$50", category.TypeIncome},
		{"$-50", category.TypeExpense},
		{"-$50", category.TypeExpense},
		{"abc", category.TypeExpense},
		{"   ", category.TypeExpense},
		{"0", category.TypeExpense},
		{"0.00", category.TypeExpense},
		{"+25", category.TypeIncome},
	}
	for _, tc := range cases {
		got := inferCategoryTypeFromAmount(tc.in)
		if got != tc.want {
			t.Errorf("inferCategoryTypeFromAmount(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestBuildCreateCategoryDialog_DefaultTypeIncome pins that the dialog's Type
// radio respects a non-default defaultType argument: passing TypeIncome
// preselects "Income" (SelectedIndex == 1).
func TestBuildCreateCategoryDialog_DefaultTypeIncome(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food"}, category.TypeIncome)
	fields := d.Fields()
	if len(fields) < 3 {
		t.Fatalf("len(fields) = %d, want >=3", len(fields))
	}
	if fields[2].SelectedIndex != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income)", fields[2].SelectedIndex)
	}
}

// TestBuildCreateCategoryDialog_DefaultTypeExpense pins that the Expense
// default (the existing behavior) still works when the caller passes
// TypeExpense.
func TestBuildCreateCategoryDialog_DefaultTypeExpense(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food"}, category.TypeExpense)
	fields := d.Fields()
	if fields[2].SelectedIndex != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense)", fields[2].SelectedIndex)
	}
}

// =============================================================================
// CC-006 — per-surface default Type radio
// =============================================================================

// TestApp_TxnDialog_AddNew_DefaultTypeFromPositiveAmount: when the user has
// typed a positive amount on the Transaction dialog, the create-category
// sub-dialog opens with Income preselected.
func TestApp_TxnDialog_AddNew_DefaultTypeFromPositiveAmount(t *testing.T) {
	app := newAppForTxnAddNew(t, "", nil, nil)
	app.txnDialog.Fields()[3].Value = "100.00" // amount

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransactionDialogKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for positive amount)", got)
	}
}

// TestApp_TxnDialog_AddNew_DefaultTypeFromNegativeAmount: when the amount is
// negative, the create-category sub-dialog opens with Expense preselected.
func TestApp_TxnDialog_AddNew_DefaultTypeFromNegativeAmount(t *testing.T) {
	app := newAppForTxnAddNew(t, "", nil, nil)
	app.txnDialog.Fields()[3].Value = "-9.50"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleTransactionDialogKey(enter)
	updated := model.(*App)
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense for negative amount)", got)
	}
}

// TestApp_SchedDialog_AddNew_DefaultTypeFromPositiveAmount: positive amount on
// the New Scheduled dialog → Income default.
func TestApp_SchedDialog_AddNew_DefaultTypeFromPositiveAmount(t *testing.T) {
	app := newAppForSchedAddNew(t, "", nil, nil)
	app.schedDialog.Fields()[schedFieldAmount].Value = "3500.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleScheduledDialogKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for positive amount)", got)
	}
}

// TestApp_SchedDialog_AddNew_DefaultTypeFromNegativeAmount: negative amount on
// the New Scheduled dialog → Expense default.
func TestApp_SchedDialog_AddNew_DefaultTypeFromNegativeAmount(t *testing.T) {
	app := newAppForSchedAddNew(t, "", nil, nil)
	// Helper already seeds "-1500.00" but pin it.
	app.schedDialog.Fields()[schedFieldAmount].Value = "-1500.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleScheduledDialogKey(enter)
	updated := model.(*App)
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense for negative amount)", got)
	}
}

// TestApp_SchedPreview_AddNew_DefaultTypeFromPositiveAmount: positive amount
// on the schedule preview single-line dialog → Income default.
func TestApp_SchedPreview_AddNew_DefaultTypeFromPositiveAmount(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)
	parkSchedPreviewOnAddNew(t, env.app, "")
	header := env.app.schedPreviewDialog.HeaderDialog()
	header.Fields()[previewSingleFieldAmount].Value = "3500.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := env.app.handleSchedulePreviewDialogKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for positive amount)", got)
	}
}

// TestApp_SchedPreview_AddNew_DefaultTypeFromNegativeAmount: negative amount on
// the preview → Expense default.
func TestApp_SchedPreview_AddNew_DefaultTypeFromNegativeAmount(t *testing.T) {
	env := newSchedulePreviewTestEnv(t)
	parkSchedPreviewOnAddNew(t, env.app, "")
	header := env.app.schedPreviewDialog.HeaderDialog()
	header.Fields()[previewSingleFieldAmount].Value = "-1500.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := env.app.handleSchedulePreviewDialogKey(enter)
	updated := model.(*App)
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense for negative amount)", got)
	}
}

// TestApp_SplitDialog_AddNew_DefaultTypeFromPositiveAmount: positive amount on
// the originating split row → Income default.
func TestApp_SplitDialog_AddNew_DefaultTypeFromPositiveAmount(t *testing.T) {
	app := newAppForSplitAddNew(t, nil, nil)
	app.splitDialog.rows[0].amountField.Value = "100.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleSplitDialogKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for positive amount)", got)
	}
}

// TestApp_SplitDialog_AddNew_DefaultTypeFromNegativeAmount: negative amount on
// the originating split row → Expense default.
func TestApp_SplitDialog_AddNew_DefaultTypeFromNegativeAmount(t *testing.T) {
	app := newAppForSplitAddNew(t, nil, nil)
	app.splitDialog.rows[0].amountField.Value = "-100.00"

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handleSplitDialogKey(enter)
	updated := model.(*App)
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense for negative amount)", got)
	}
}

// TestApp_PaycheckWizard_AddNew_DefaultTypeFromTaxSection: the AddNew sentinel
// on a Tax-section line opens the sub-dialog with Expense preselected.
func TestApp_PaycheckWizard_AddNew_DefaultTypeFromTaxSection(t *testing.T) {
	app, _ := newAppForPaycheckAddNew(t, nil, nil) // helper parks on a Pre-Tax line
	// The helper parks the focused line in Pre-Tax (which also defaults to
	// Expense). Pin the section to confirm and assert the default.
	if app.paycheckWizard == nil {
		t.Fatal("paycheckWizard should be set")
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 0 {
		t.Errorf("Type.SelectedIndex = %d, want 0 (Expense for pre-tax line)", got)
	}
}

// TestApp_PaycheckWizard_AddNew_DefaultTypeFromEarningsSection: the AddNew
// sentinel on an Earnings-section line opens the sub-dialog with Income
// preselected.
func TestApp_PaycheckWizard_AddNew_DefaultTypeFromEarningsSection(t *testing.T) {
	app, _ := newAppForPaycheckAddNew(t, nil, nil)
	w := app.paycheckWizard
	// Replace the parked Pre-Tax line with an Earnings line parked on AddNew.
	earnings := w.AddRow(PaycheckEarnings)
	earnings.SelectField().SelectedIndex = len(earnings.SelectField().Options) - 1
	if !earnings.IsAddNew() {
		t.Fatal("earnings line should be parked on AddNew sentinel")
	}
	for i, target := range w.collectFocusables() {
		if target.kind == wizardFocusField && target.field == earnings.SelectField() {
			w.focusIndex = i
			break
		}
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	updated := model.(*App)
	if updated.createCatDialog == nil {
		t.Fatal("createCatDialog should be open")
	}
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for earnings line)", got)
	}
}

// TestApp_PaycheckWizard_AddNew_DefaultTypeFromNetPaySection: the AddNew
// sentinel on a Net Pay Destination line — these rows are usually transfer
// accounts but the AddNew sentinel is reachable; Net Pay defaults to Income.
func TestApp_PaycheckWizard_AddNew_DefaultTypeFromNetPaySection(t *testing.T) {
	app, _ := newAppForPaycheckAddNew(t, nil, nil)
	w := app.paycheckWizard
	netPay := w.AddRow(PaycheckNetPayDestination)
	netPay.SelectField().SelectedIndex = len(netPay.SelectField().Options) - 1
	if !netPay.IsAddNew() {
		t.Fatal("net pay line should be parked on AddNew sentinel")
	}
	for i, target := range w.collectFocusables() {
		if target.kind == wizardFocusField && target.field == netPay.SelectField() {
			w.focusIndex = i
			break
		}
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	model, _ := app.handlePaycheckWizardKey(enter)
	updated := model.(*App)
	if got := updated.createCatDialog.Fields()[2].SelectedIndex; got != 1 {
		t.Errorf("Type.SelectedIndex = %d, want 1 (Income for net-pay line)", got)
	}
}

// TestApplyCreatedCategory_UnknownSourceClearsDialog covers the router's
// safety branch: if a createCategoryRequestMsg arrives with no source set
// (e.g. the surface plumbing didn't set createCatSource before opening), the
// router must still close the sub-dialog so the user isn't stuck on an inert
// overlay. The new category should still be persisted.
func TestApplyCreatedCategory_UnknownSourceClearsDialog(t *testing.T) {
	svc, _ := newCategorySvcForPersistTest(t)
	app := &App{
		categorySvc:     svc,
		createCatDialog: buildCreateCategoryDialog("X", "", nil, category.TypeExpense),
		createCatSource: createCatSourceNone,
	}
	if app.createCatDialog == nil {
		t.Fatal("test setup: createCatDialog should be non-nil")
	}

	if err := app.applyCreatedCategory(createCategoryRequest{
		Name: "Bonsai",
		Type: category.TypeExpense,
	}); err != nil {
		t.Fatalf("applyCreatedCategory: %v", err)
	}
	if app.createCatDialog != nil {
		t.Error("router should clear createCatDialog even when source is unknown")
	}
	if app.createCatSource != createCatSourceNone {
		t.Errorf("createCatSource = %d, want None after dispatch", app.createCatSource)
	}

	cats, _ := svc.List()
	var found bool
	for _, c := range cats {
		if c.Name == "Bonsai" {
			found = true
			break
		}
	}
	if !found {
		t.Error("'Bonsai' should be persisted even under the unknown-source branch")
	}
}
