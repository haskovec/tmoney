package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
)

// createCategoryRequest captures the user's intent to create a new category.
// ParentName == "" means the new category is top-level. NewParent reports
// whether ParentName names a parent that does not yet exist (and therefore
// must be created before the child).
type createCategoryRequest struct {
	Name       string
	ParentName string
	NewParent  bool
	Type       category.Type
}

// createCategoryRequestMsg is emitted when the create-category dialog is
// submitted with valid input. Consumers create the parent (when NewParent is
// true) and then the child via category.Service.Create.
type createCategoryRequestMsg struct {
	request createCategoryRequest
}

// buildCreateCategoryDialog constructs the inline create-category sub-dialog
// reachable from the category combo's [+ Add new category…] action row.
//
// Fields: Name (text, required), Parent (combo over "(top-level)" plus any
// existingParents), Type (radio: Expense, Income). The query argument seeds
// the Name field — typical use is to pass through the typed-but-uncommitted
// query from the parent transaction dialog so the user doesn't retype it.
//
// When Name is pre-filled, focus starts on Parent so the user can pick or
// type a parent without re-confirming the name. When Name is empty, focus
// starts on Name.
func buildCreateCategoryDialog(query string, existingParents []string) *Dialog {
	d := NewDialog("New Category")

	nameField := d.AddTextField("Name", query, "Category name", 0)
	nameField.Required = true

	parentOptions := append([]string{"(top-level)"}, existingParents...)
	d.AddComboField("Parent", parentOptions, 0)

	d.AddRadioField("Type", []string{"Expense", "Income"}, 0)

	if query == "" {
		d.SetFocusIndex(0)
	} else {
		d.SetFocusIndex(1)
	}

	d.SetVisible(true)
	return d
}

// collectCreateCategoryRequest validates the dialog's input and, on success,
// returns a populated createCategoryRequest. Returns ok=false and sets an
// inline error on the offending field when validation fails.
func collectCreateCategoryRequest(d *Dialog, existingParents []string) (createCategoryRequest, bool) {
	fields := d.Fields()
	if len(fields) < 3 {
		return createCategoryRequest{}, false
	}
	d.ClearErrors()

	name := strings.TrimSpace(fields[0].Value)
	if name == "" {
		fields[0].Error = "Name is required"
		return createCategoryRequest{}, false
	}

	parentName, newParent := readParentCombo(fields[1], existingParents)

	catType := category.TypeExpense
	if fields[2].SelectedIndex == 1 {
		catType = category.TypeIncome
	}

	return createCategoryRequest{
		Name:       name,
		ParentName: parentName,
		NewParent:  newParent,
		Type:       catType,
	}, true
}

// readParentCombo reads the parent name out of the Parent combo field. Any
// non-empty Query (typed but not committed) takes precedence: if it
// case-insensitively matches an existing parent, that existing parent is
// returned; otherwise it's flagged as a new top-level to create. When Query
// is empty, SelectedIndex is consulted — index 0 is "(top-level)" (no
// parent), index n>0 maps to existingParents[n-1].
func readParentCombo(f *Field, existingParents []string) (name string, newParent bool) {
	q := strings.TrimSpace(f.Query)
	if q != "" {
		for _, p := range existingParents {
			if strings.EqualFold(p, q) {
				return p, false
			}
		}
		return q, true
	}
	if f.SelectedIndex <= 0 || f.SelectedIndex-1 >= len(existingParents) {
		return "", false
	}
	return existingParents[f.SelectedIndex-1], false
}

// submitCreateCategoryDialog returns a tea.Cmd that emits a
// createCategoryRequestMsg when the dialog input is valid, or nil when
// validation fails (errors are set inline on the offending fields).
func submitCreateCategoryDialog(d *Dialog, existingParents []string) tea.Cmd {
	req, ok := collectCreateCategoryRequest(d, existingParents)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		return createCategoryRequestMsg{request: req}
	}
}
