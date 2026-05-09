package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
)

func TestBuildCreateCategoryDialog_FieldShape(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food", "Bills", "Auto"})

	fields := d.Fields()
	if len(fields) != 3 {
		t.Fatalf("len(Fields) = %d, want 3 (Name, Parent, Type)", len(fields))
	}

	if fields[0].Label != "Name" {
		t.Errorf("fields[0].Label = %q, want %q", fields[0].Label, "Name")
	}
	if fields[0].Type != FieldText {
		t.Errorf("fields[0].Type = %v, want FieldText", fields[0].Type)
	}
	if !fields[0].Required {
		t.Error("Name field should be Required")
	}

	if fields[1].Label != "Parent" {
		t.Errorf("fields[1].Label = %q, want %q", fields[1].Label, "Parent")
	}
	if fields[1].Type != FieldCombo {
		t.Errorf("fields[1].Type = %v, want FieldCombo", fields[1].Type)
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
	if fields[2].Type != FieldRadio {
		t.Errorf("fields[2].Type = %v, want FieldRadio", fields[2].Type)
	}
	if len(fields[2].Options) != 2 {
		t.Fatalf("Type.Options = %v, want 2 entries", fields[2].Options)
	}

	if !d.IsVisible() {
		t.Error("dialog should be visible")
	}
}

func TestBuildCreateCategoryDialog_TabCyclesAcrossFields(t *testing.T) {
	d := buildCreateCategoryDialog("", "", []string{"Food"})

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
	d := buildCreateCategoryDialog("", "", parents)
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
	d := buildCreateCategoryDialog("", "", parents)
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
	d := buildCreateCategoryDialog("", "", parents)
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
	d := buildCreateCategoryDialog("", "", parents)
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
	d := buildCreateCategoryDialog("", "", parents)
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
	d := buildCreateCategoryDialog("", "", []string{"Food"})

	action := d.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if action != DialogActionCancel {
		t.Errorf("HandleKey(Esc) = %v, want DialogActionCancel", action)
	}
}

func TestBuildCreateCategoryDialog_SeedsNameFromQuery(t *testing.T) {
	d := buildCreateCategoryDialog("Donations", "", []string{"Food"})
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
	d := buildCreateCategoryDialog("Sushi", "Food", []string{"Auto", "Food", "Bills"})
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
	d := buildCreateCategoryDialog("Sushi", "food", []string{"Food", "Bills"})
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
	d := buildCreateCategoryDialog("Endowment", "Charity", []string{"Food", "Bills"})
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
