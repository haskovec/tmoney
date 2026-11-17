package tui

import (
	"strings"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// The category sign rule (specs/transactions.md, "Category Sign Rule"): an
// amount field follows the type of the category chosen beside it, so an
// expense gets its leading "-" without the user typing it.

// categorySign is the sign a category asks of the amount beside it.
type categorySign int

const (
	// categorySignNone is "(None)", a system category (Value Adjustment
	// holds increases and decreases alike) or an unknown ID: the rule only
	// clears a lone "-".
	categorySignNone categorySign = iota
	// categorySignNegative is an Expense category.
	categorySignNegative
	// categorySignPositive is an Income category.
	categorySignPositive
)

// categorySignOf returns the sign category id asks for, looked up in cats.
func categorySignOf(cats []*category.Category, id types.ID) categorySign {
	if id.IsNil() {
		return categorySignNone
	}
	for _, c := range cats {
		if c == nil || c.ID != id {
			continue
		}
		if c.IsSystem {
			return categorySignNone
		}
		switch c.Type {
		case category.TypeExpense:
			return categorySignNegative
		case category.TypeIncome:
			return categorySignPositive
		}
		return categorySignNone
	}
	return categorySignNone
}

// categoryIDAt returns ids[idx] for a picker whose options parallel ids, or
// NilID when idx is outside ids. "(None)" is index 0 with a nil ID.
func categoryIDAt(ids []types.ID, idx int) types.ID {
	if idx < 0 || idx >= len(ids) {
		return types.NilID
	}
	return ids[idx]
}

// categorySignAt returns the sign of the category at idx in a picker whose
// options parallel ids. An index outside ids asks for no sign.
func categorySignAt(cats []*category.Category, ids []types.ID, idx int) categorySign {
	return categorySignOf(cats, categoryIDAt(ids, idx))
}

// categorySignState is a dialog's state for the category sign rule: the
// Amount text it opened with and the category the rule last saw.
type categorySignState struct {
	// loadedAmount is the Amount text the dialog opened with. The rule never
	// flips an amount still equal to it (an imported refund, a stored
	// schedule amount).
	loadedAmount string
	// categoryID is the category the rule last saw. The rule runs only when
	// the selected category differs from it: a commit runs on every Tab and
	// click away, and a re-run would undo a "-" the user deleted for a refund.
	categoryID types.ID
}

// open records the state a dialog opened with: its Amount text and selected
// category are not a change.
func (st *categorySignState) open(amount string, categoryID types.ID) {
	st.loadedAmount = amount
	st.categoryID = categoryID
}

// sync runs the rule on amount when categoryID differs from the category it
// last saw, reading the category's type from cats. With apply false it only
// records categoryID: a click on Save can commit a category in the same
// event, and Save never changes the sign.
func (st *categorySignState) sync(amount *dialog.Field, categoryID types.ID, cats []*category.Category, apply bool) {
	if categoryID == st.categoryID {
		return
	}
	st.categoryID = categoryID
	if apply {
		applyCategorySign(amount, st.loadedAmount, categorySignOf(cats, categoryID))
	}
}

// applyOpen runs the rule once for the category the dialog opened with. Only
// an empty Amount can change (to "-" under an Expense category): the text
// the dialog opened with is the loaded text, which keeps its sign.
func (st *categorySignState) applyOpen(amount *dialog.Field, cats []*category.Category) {
	applyCategorySign(amount, st.loadedAmount, categorySignOf(cats, st.categoryID))
}

// signedAmountText applies the category sign rule to an amount's text. loaded
// is the text the dialog loaded into the field when it opened: an amount still
// equal to it keeps its sign, so an imported refund or a stored schedule amount
// is never flipped by a recategorization. An empty or lone-"-" amount always
// takes the rule. It returns the new text and whether it differs from value.
func signedAmountText(value, loaded string, sign categorySign) (string, bool) {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "":
		if sign == categorySignNegative {
			return "-", true
		}
		return value, false
	case "-":
		if sign == categorySignNegative {
			return value, false
		}
		return "", true
	}
	if sign == categorySignNone || trimmed == strings.TrimSpace(loaded) {
		return value, false
	}
	m, err := parseAmountInput(trimmed)
	if err != nil {
		// Leave text the user is still typing (or mistyped) alone; Save
		// reports it.
		return value, false
	}
	i := dialog.AmountSignIndex(trimmed)
	switch {
	case sign == categorySignNegative && m.IsPositive():
		if i >= 0 { // a "+"
			return trimmed[:i] + "-" + trimmed[i+1:], true
		}
		return "-" + trimmed, true
	case sign == categorySignPositive && m.IsNegative():
		return trimmed[:i] + trimmed[i+1:], true // a negative amount has its "-" there
	}
	return value, false
}

// applyCategorySign rewrites f's amount for sign and moves the cursor to the
// end when the text changes, so digits typed next land after a new "-". A
// changed value also clears f's inline error, the same as typing does: the
// error was about the text it replaced. loaded is the text f held when its
// dialog opened.
func applyCategorySign(f *dialog.Field, loaded string, sign categorySign) {
	next, changed := signedAmountText(f.Value, loaded, sign)
	if !changed {
		return
	}
	f.Value = next
	f.MoveCursorEnd()
	f.Error = ""
}
