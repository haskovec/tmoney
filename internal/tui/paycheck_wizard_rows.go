package tui

import (
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// The wizard's row model: PaycheckLine and the add/remove operations that
// keep the five sections in shape.

// PaycheckLine is one row in a section: a category-or-transfer
// select plus an amount input. The line is rendered with a [−]
// remove button.
type PaycheckLine struct {
	Section     PaycheckSection
	selectField *dialog.Field // category or transfer picker (combined list)
	amountField *dialog.Field // signed amount as typed by the user
	notesField  *dialog.Field // optional free-form description (stored as Split.Memo)

	// categoryCount is captured at construction so the line can
	// self-classify (IsTransfer/CategoryIndex/AccountIndex) without
	// holding a back-reference to the wizard.
	categoryCount int
}

// SelectField exposes the line's category-or-transfer select for
// tests and key handling.
func (l *PaycheckLine) SelectField() *dialog.Field { return l.selectField }

// AmountField exposes the line's amount input.
func (l *PaycheckLine) AmountField() *dialog.Field { return l.amountField }

// NotesField exposes the line's free-form notes input. The value is
// persisted as Split.Memo when the wizard saves.
func (l *PaycheckLine) NotesField() *dialog.Field { return l.notesField }

// IsTransfer reports whether the line's current select points at
// the transfer half of the combined picker (i.e. an account
// destination rather than a category). The trailing
// [+ Add new category…] sentinel sits past the transfer entries and is
// excluded.
func (l *PaycheckLine) IsTransfer() bool {
	if l.selectField == nil {
		return false
	}
	idx := l.selectField.SelectedIndex
	return idx >= l.categoryCount && idx < len(l.selectField.Options)-1
}

// IsAddNew reports whether the line's current select points at the
// trailing [+ Add new category…] action-row sentinel. Enter on a line
// in this state diverts into the inline create-category sub-dialog.
func (l *PaycheckLine) IsAddNew() bool {
	if l.selectField == nil || len(l.selectField.Options) == 0 {
		return false
	}
	return l.selectField.SelectedIndex == len(l.selectField.Options)-1
}

// CategoryIndex returns the index into categoryOptions for a
// categorized line (or 0 — the "(None)" sentinel — for transfer
// lines).
func (l *PaycheckLine) CategoryIndex() int {
	if l.IsTransfer() || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex
}

// AccountIndex returns the index into accountOptions for a
// transfer-line (or 0 for categorized lines).
func (l *PaycheckLine) AccountIndex() int {
	if !l.IsTransfer() || l.selectField == nil {
		return 0
	}
	return l.selectField.SelectedIndex - l.categoryCount
}

// SetCategoryIndex makes the line categorized with the given category
// option index. Out-of-range indices are clamped to 0.
func (l *PaycheckLine) SetCategoryIndex(idx int) {
	if l.selectField == nil {
		return
	}
	if idx < 0 || idx >= l.categoryCount {
		idx = 0
	}
	l.selectField.SelectedIndex = idx
}

// SetAccountIndex converts the line into a transfer-line targeting
// the given account index. The trailing [+ Add new category…] sentinel
// occupies the last slot of Options, so the valid transfer range stops
// one short of len(Options).
func (l *PaycheckLine) SetAccountIndex(idx int) {
	if l.selectField == nil {
		return
	}
	target := l.categoryCount + idx
	if target < l.categoryCount || target >= len(l.selectField.Options)-1 {
		return
	}
	l.selectField.SelectedIndex = target
}

// AddRow appends an empty row to the given section and returns it.
// The amount and category select default to empty/(None) and the
// line is positioned as the last focusable target in its section.
func (w *PaycheckWizard) AddRow(section PaycheckSection) *PaycheckLine {
	if section < PaycheckEarnings || section > PaycheckNetPayDestination {
		return nil
	}
	line := &PaycheckLine{
		Section: section,
		amountField: &dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "0.00",
			Width:       14,
		},
		selectField: &dialog.Field{
			Type:          dialog.FieldSelect,
			Options:       w.combinedOptions,
			SelectedIndex: 0,
		},
		notesField: &dialog.Field{
			Type:        dialog.FieldText,
			Placeholder: "Notes",
		},
		categoryCount: len(w.categoryOptions),
	}
	w.sections[section] = append(w.sections[section], line)
	return line
}

// AddEarningsLine appends a new row to the Earnings section,
// pre-selected with `Income > Salary` (or left at the leading `(None)`
// when that category isn't in the picker). Used by the
// `[+ Add earnings line]` affordance.
func (w *PaycheckWizard) AddEarningsLine() *PaycheckLine {
	line := w.AddRow(PaycheckEarnings)
	if line == nil {
		return nil
	}
	if idx := findCategoryOptionIndex(w.categoryOptions, "Income > Salary"); idx > 0 {
		line.SetCategoryIndex(idx)
	}
	return line
}

// AddPreTaxLine appends a new categorized row to the Pre-tax section.
// Pre-tax items vary by employer (401k, HSA, supplemental life, …) —
// the row defaults to `(None)` so the user picks the category or
// flips it to a transfer-line via the combined picker.
func (w *PaycheckWizard) AddPreTaxLine() *PaycheckLine {
	return w.AddRow(PaycheckPreTax)
}

// AddTaxLine appends a new categorized row to the Taxes section.
// Defaults to `(None)`; the three universal tax rows
// (Federal / Social Security / Medicare) are already pre-populated by
// `NewPaycheckWizard`, so any added row is for an additional tax
// (e.g., state income tax) the user picks themselves.
func (w *PaycheckWizard) AddTaxLine() *PaycheckLine {
	return w.AddRow(PaycheckTax)
}

// AddPostTaxLine appends a new categorized row to the Post-tax
// section. Defaults to `(None)`; post-tax deductions vary by employer.
func (w *PaycheckWizard) AddPostTaxLine() *PaycheckLine {
	return w.AddRow(PaycheckPostTax)
}

// AddAdditionalTransfer appends a new transfer-line row to the Net
// Pay Destinations section, pre-selected with the first available
// account other than the current deposit account (or the first
// account when no alternative exists). Net Pay Destinations holds
// *additional* transfers — the primary deposit is the schedule's
// parent account in the header picker.
func (w *PaycheckWizard) AddAdditionalTransfer() *PaycheckLine {
	line := w.AddRow(PaycheckNetPayDestination)
	if line == nil {
		return nil
	}
	depositIdx := -1
	if w.accountField != nil {
		depositIdx = w.accountField.SelectedIndex
	}
	for i := range w.accountIDs {
		if i == depositIdx {
			continue
		}
		line.SetAccountIndex(i)
		return line
	}
	if len(w.accountIDs) > 0 {
		line.SetAccountIndex(0)
	}
	return line
}

// addLineForSection dispatches `[+ Add …]` clicks/Enter to the
// section-specific helper so the new row picks up that section's
// defaults.
func (w *PaycheckWizard) addLineForSection(section PaycheckSection) *PaycheckLine {
	switch section {
	case PaycheckEarnings:
		return w.AddEarningsLine()
	case PaycheckPreTax:
		return w.AddPreTaxLine()
	case PaycheckTax:
		return w.AddTaxLine()
	case PaycheckPostTax:
		return w.AddPostTaxLine()
	case PaycheckNetPayDestination:
		return w.AddAdditionalTransfer()
	}
	return nil
}

// RemoveRow removes the given row from its section. Best-effort: a
// nil line or a line not found in any section is a no-op.
func (w *PaycheckWizard) RemoveRow(line *PaycheckLine) {
	if line == nil {
		return
	}
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for i, l := range w.sections[s] {
			if l == line {
				w.sections[s] = append(w.sections[s][:i], w.sections[s][i+1:]...)
				return
			}
		}
	}
}
