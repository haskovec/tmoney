package tui

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// The mapping between the wizard and a scheduled transaction, in both
// directions: BuildSplits and its helpers derive the saved splits and the net
// total from the rows; looksLikePaycheck and NewPaycheckWizardFromSchedule
// recognise a saved paycheck and rebuild the wizard from it for editing.

// BuildSplits assembles the wizard's row state into a list of
// scheduled-split rows and computes the parent net amount (the signed
// sum). Empty amount fields are skipped — they don't produce zero-
// amount rows. Returns an error when validation fails (unparseable
// amount, missing category/account on a populated row).
func (w *PaycheckWizard) BuildSplits() (types.Money, []*scheduled.Split, error) {
	if w == nil {
		return types.ZeroMoney, nil, fmt.Errorf("nil wizard")
	}

	splits := make([]*scheduled.Split, 0)

	// Iterate sections in spec order (earnings → pre-tax → tax →
	// post-tax → net-pay-destination) so the persisted split ordering
	// matches the wizard layout.
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for _, line := range w.sections[s] {
			sp, err := w.buildLineSplit(line)
			if err != nil {
				return types.ZeroMoney, nil, err
			}
			if sp != nil {
				splits = append(splits, sp)
			}
		}
	}

	if len(splits) == 0 {
		return types.ZeroMoney, nil, fmt.Errorf("add at least one row")
	}

	parent := types.ZeroMoney
	for _, s := range splits {
		parent = parent.Add(s.Amount)
	}
	return parent, splits, nil
}

// buildLineSplit translates a single line into a scheduled.Split.
// Returns (nil, nil) for an empty/zero-amount row (silently skipped).
// The user's typed amount is preserved verbatim, including its sign —
// unlike the old wizard, the new flow does not flip signs because
// the user explicitly chooses positive/negative per row.
func (w *PaycheckWizard) buildLineSplit(line *PaycheckLine) (*scheduled.Split, error) {
	amtStr := strings.TrimSpace(line.amountField.Value)
	if amtStr == "" {
		return nil, nil
	}
	amt, err := parseAmountInput(amtStr)
	if err != nil {
		return nil, fmt.Errorf("%s row: %w", line.Section.Title(), err)
	}
	if amt.IsZero() {
		return nil, nil
	}

	sp := &scheduled.Split{
		BaseModel: types.NewBaseModel(),
		Amount:    amt,
	}
	if tag := line.Section.tagString(); tag != "" {
		sp.PaycheckSection = types.NullableString{String: tag, Valid: true}
	}
	if line.notesField != nil {
		if notes := strings.TrimSpace(line.notesField.Value); notes != "" {
			sp.Memo = types.NullableString{String: notes, Valid: true}
		}
	}
	if line.IsTransfer() {
		accountID := w.lookupAccountID(line.AccountIndex())
		if accountID.IsNil() {
			return nil, fmt.Errorf("%s row: pick a transfer destination", line.Section.Title())
		}
		sp.TransferAccountID = types.NullableID{ID: accountID, Valid: true}
		return sp, nil
	}
	catID := w.lookupCategoryID(line.CategoryIndex())
	if catID.IsNil() {
		return nil, fmt.Errorf("%s row: pick a category", line.Section.Title())
	}
	sp.CategoryID = types.NullableID{ID: catID, Valid: true}
	return sp, nil
}

// lookupCategoryID maps a select index to a category ID.
func (w *PaycheckWizard) lookupCategoryID(idx int) types.ID {
	if idx <= 0 || idx >= len(w.categoryIDs) {
		return types.NilID
	}
	return w.categoryIDs[idx]
}

// lookupAccountID maps a select index to an account ID.
func (w *PaycheckWizard) lookupAccountID(idx int) types.ID {
	if idx < 0 || idx >= len(w.accountIDs) {
		return types.NilID
	}
	return w.accountIDs[idx]
}

// computeTotal returns the signed sum of every populated row.
func (w *PaycheckWizard) computeTotal() types.Money {
	total := types.ZeroMoney
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		for _, line := range w.sections[s] {
			amtStr := strings.TrimSpace(line.amountField.Value)
			if amtStr == "" {
				continue
			}
			amt, err := parseAmountInput(amtStr)
			if err != nil {
				continue
			}
			total = total.Add(amt)
		}
	}
	return total
}

// ===========================================================================
// Edit-as-paycheck heuristic
// ===========================================================================

// looksLikePaycheck reports whether a scheduled transaction matches
// the v2 paycheck heuristic: the schedule is multi-line, **every**
// split carries a non-NULL `paycheck_section` tag (i.e. it was
// produced by the wizard, not by the generic multi-line split
// dialog), and at least one split is tagged `earnings`.
//
// A NULL tag on any single line treats the schedule as a generic
// multi-line split — the Edit-as-paycheck affordance stays hidden so
// the user can't lose tags by round-tripping through a heuristic that
// would have to guess the section for the untagged line.
func looksLikePaycheck(st *scheduled.Transaction) bool {
	if st == nil || len(st.Splits) == 0 {
		return false
	}
	hasEarnings := false
	for _, sp := range st.Splits {
		if sp == nil || !sp.PaycheckSection.Valid {
			return false
		}
		if sp.PaycheckSection.String == "earnings" {
			hasEarnings = true
		}
	}
	return hasEarnings
}

// sectionForTag maps a paycheck_section tag string (as persisted by
// BuildSplits via PaycheckSection.tagString) back to the enum value.
// Unknown or empty tags route to PaycheckPostTax as a defensive fallback
// — in practice looksLikePaycheck rejects schedules with any NULL tag
// so the Edit-as-paycheck affordance never opens such a schedule.
func sectionForTag(tag string) PaycheckSection {
	switch tag {
	case "earnings":
		return PaycheckEarnings
	case "pre_tax":
		return PaycheckPreTax
	case "tax":
		return PaycheckTax
	case "post_tax":
		return PaycheckPostTax
	case "net_pay_destination":
		return PaycheckNetPayDestination
	}
	return PaycheckPostTax
}

// NewPaycheckWizardFromSchedule builds a paycheck wizard pre-filled
// from a multi-line scheduled transaction. Sections come from the
// `paycheck_section` tag stamped on each split by BuildSplits — storage
// order within each section is preserved, but section assignment is
// driven by the tag, not the position. Untagged or unknown-tag splits
// land in Post-tax as a defensive fallback; in practice
// looksLikePaycheck rejects schedules with any NULL tag before this
// path is reached.
func NewPaycheckWizardFromSchedule(
	st *scheduled.Transaction,
	accounts []*account.Account,
	payees []*payee.Payee,
	categoryOptions []string,
	categoryIDs []types.ID,
) *PaycheckWizard {
	w := NewPaycheckWizard(categoryOptions, categoryIDs, accounts)
	if st == nil {
		return w
	}
	// Remember which schedule this is: the save path edits this record in
	// place rather than creating a second one.
	w.editSchedule = st

	// Drop the v2 default-seeded rows — the schedule's tagged splits are
	// the only content; defaults must not be appended on top.
	for s := PaycheckEarnings; s <= PaycheckNetPayDestination; s++ {
		w.sections[s] = nil
	}

	if st.HasPayee() {
		for _, p := range payees {
			if p == nil {
				continue
			}
			if p.ID == st.PayeeID.ID {
				prefillField(w.employerField, p.Name)
				break
			}
		}
	}

	w.frequencyField.SelectedIndex = paycheckFrequencyIndexFor(st)
	prefillField(w.nextPaydayField, st.NextDate.Time().Format("01/02/2006"))

	for i := range w.accountField.Options {
		if i >= len(w.accountIDs) {
			break
		}
		if w.accountIDs[i] == st.AccountID {
			w.accountField.SelectedIndex = i
			break
		}
	}

	if st.Memo.Valid {
		prefillField(w.memoField, st.Memo.String)
	}

	for _, sp := range st.Splits {
		if sp == nil {
			continue
		}
		section := sectionForTag(sp.PaycheckSection.String)
		if !sp.PaycheckSection.Valid {
			section = PaycheckPostTax
		}

		var selectIdx int
		if sp.TransferAccountID.Valid {
			for i, id := range w.accountIDs {
				if id == sp.TransferAccountID.ID {
					selectIdx = len(w.categoryOptions) + i
					break
				}
			}
		} else if sp.CategoryID.Valid {
			// Match on the ID, not the display name: two options can render
			// the same name (a subcategory under a system parent is shown
			// bare), and a name miss silently re-points the row at "(None)".
			for i, id := range w.categoryIDs {
				if id == sp.CategoryID.ID {
					selectIdx = i
					break
				}
			}
		}

		line := w.AddRow(section)
		line.selectField.SelectedIndex = selectIdx
		prefillField(line.amountField, sp.Amount.String())
		if sp.Memo.Valid {
			prefillField(line.notesField, sp.Memo.String)
		}
	}

	return w
}

// missingAccountForPaycheckEdit reports whether st names an account the
// paycheck wizard's pickers would not contain — its deposit account or any
// transfer-line destination. The candidate set comes from the same helper the
// wizard builds its pickers from, so the two cannot disagree.
func missingAccountForPaycheckEdit(st *scheduled.Transaction, accounts []*account.Account) bool {
	if st == nil {
		return false
	}
	_, ids := buildSplitTransferAccountOptions(accounts)
	offered := make(map[types.ID]bool, len(ids))
	for _, id := range ids {
		offered[id] = true
	}
	if !offered[st.AccountID] {
		return true
	}
	for _, sp := range st.Splits {
		if sp == nil || !sp.TransferAccountID.Valid {
			continue
		}
		if !offered[sp.TransferAccountID.ID] {
			return true
		}
	}
	return false
}
