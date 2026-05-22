package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// createCategorySource identifies which transaction-entry surface opened the
// inline create-category sub-dialog. The post-create router (applyCreatedCategory)
// dispatches on this so the new category lands back in the right field of the
// originating dialog without each surface having to own a copy of the
// persistence logic.
type createCategorySource int

const (
	createCatSourceNone createCategorySource = iota
	createCatSourceTxnDialog
	createCatSourceSchedDialog
	createCatSourceSchedPreview
	createCatSourceSplitDialog
	createCatSourcePaycheckWizard
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
// existingParents), Type (radio: Expense, Income). The name and parent
// arguments seed the corresponding fields; pass the parts of the typed-but-
// uncommitted query from the parent transaction dialog so the user doesn't
// retype them.
//
// When parent matches an existingParents entry case-insensitively, the
// Parent combo's SelectedIndex resolves to it. When parent is non-empty but
// not in existingParents, it is seeded as the combo's Query (the new-parent
// path) so submission flags NewParent=true.
//
// Focus starts on Name when name is empty; on Parent when name is filled
// but parent is empty; on Type when both are filled.
func buildCreateCategoryDialog(name, parent string, existingParents []string) *Dialog {
	d := NewDialog("New Category")

	nameField := d.AddTextField("Name", name, "Category name", 0)
	nameField.Required = true

	parentOptions := append([]string{"(top-level)"}, existingParents...)
	parentField := d.AddComboField("Parent", parentOptions, 0)

	if parent != "" {
		matchedIdx := -1
		for i, p := range existingParents {
			if strings.EqualFold(p, parent) {
				matchedIdx = i + 1 // +1 for the "(top-level)" sentinel
				break
			}
		}
		if matchedIdx > 0 {
			parentField.SelectedIndex = matchedIdx
		} else {
			parentField.Query = parent
		}
	}

	d.AddRadioField("Type", []string{"Expense", "Income"}, 0)

	switch {
	case name == "":
		d.SetFocusIndex(0)
	case parent == "":
		d.SetFocusIndex(1)
	default:
		d.SetFocusIndex(2)
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

// persistCategory creates the requested category (and its parent, when
// req.NewParent is true) using the App's category service and returns the new
// leaf category. Pure persistence — no UI side effects, no dialog mutations.
// The returned category's Type follows the request, except in the existing-
// parent path where the child inherits the parent's Type (matches the prior
// behavior of applyCreatedCategory).
func (a *App) persistCategory(req createCategoryRequest) (*category.Category, error) {
	if a.categorySvc == nil {
		return nil, fmt.Errorf("category service unavailable")
	}

	catType := req.Type
	var parentID types.ID
	if req.ParentName != "" {
		if req.NewParent {
			parent := category.NewCategory(req.ParentName, catType)
			if err := a.categorySvc.Create(parent); err != nil {
				return nil, fmt.Errorf("create parent: %w", err)
			}
			parentID = parent.ID
		} else {
			existing, err := a.categorySvc.GetByName(req.ParentName, nil)
			if err != nil {
				return nil, fmt.Errorf("lookup parent: %w", err)
			}
			parentID = existing.ID
			catType = existing.Type
		}
	}

	var newCat *category.Category
	if parentID == types.NilID {
		newCat = category.NewCategory(req.Name, catType)
	} else {
		newCat = category.NewSubcategory(req.Name, parentID, catType)
	}
	if err := a.categorySvc.Create(newCat); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return newCat, nil
}

// applyCreatedCategory is the router for the createCategoryRequestMsg path. It
// persists the requested category and dispatches to the per-surface applier
// matching a.createCatSource so the new category lands back in the originating
// field of whichever transaction-entry surface opened the sub-dialog. Surface
// scratch fields (e.g. createCatSource) are cleared after dispatch.
func (a *App) applyCreatedCategory(req createCategoryRequest) error {
	newCat, err := a.persistCategory(req)
	if err != nil {
		return err
	}

	cats, err := a.categorySvc.List()
	if err != nil {
		return fmt.Errorf("reload categories: %w", err)
	}

	switch a.createCatSource {
	case createCatSourceTxnDialog:
		a.applyCreatedCategoryToTxn(newCat, cats)
	case createCatSourceSchedDialog:
		a.applyCreatedCategoryToSched(newCat, cats)
	case createCatSourceSchedPreview:
		a.applyCreatedCategoryToSchedPreview(newCat, cats)
	case createCatSourceSplitDialog:
		a.applyCreatedCategoryToSplit(newCat, cats)
	case createCatSourcePaycheckWizard:
		a.applyCreatedCategoryToPaycheck(newCat, cats)
	default:
		// No source recorded — surface plumbing not wired (or the source
		// enum was reset before the router fired). Close the sub-dialog
		// so the user isn't stuck on an inert overlay.
		a.createCatDialog = nil
	}

	a.createCatSource = createCatSourceNone
	return nil
}
