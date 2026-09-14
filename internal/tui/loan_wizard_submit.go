package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// Save: the new-loan path builds the loan account, the optional asset account
// and the payment schedule inside one compound undo command; the edit path
// rewrites the live schedule in place.

// resolveLoanPrincipalSelection reads the Principal Category combo into a
// save-time decision. useDefault=true means the synthetic Loan > Principal row
// is selected — resolve via GetOrCreateLoanPrincipalCategory at save. Otherwise
// pickedID is the chosen category, or NilID for "(None)" (an unlabeled
// principal line). A real Loan:Principal category selects normally (its own ID,
// no get-or-create needed).
func resolveLoanPrincipalSelection(st *loanWizardData, f *dialog.Field) (useDefault bool, pickedID types.ID) {
	idx := f.SelectedIndex
	if idx <= 0 || idx >= len(st.principalOptions) {
		return false, types.NilID // "(None)" or out of range → unlabeled
	}
	if st.principalOptions[idx] == loanPrincipalDefaultDisplay && st.principalIDs[idx].IsNil() {
		return true, types.NilID
	}
	return false, st.principalIDs[idx]
}

// submit dispatches to the new-loan or Edit-as-loan save path. A nil command
// means validation failed: the wizard stays open carrying its field errors.
// On success the surface is closed before the command runs.
func (s *loanSurface) submit(deps loanDeps) tea.Cmd {
	if s.dlg == nil {
		return nil
	}
	if s.state != nil && s.state.mode == loanWizardModeEdit {
		return s.submitEdit(deps)
	}
	return s.submitNew(deps)
}

// submitNew validates the wizard's fields and, on success, persists
// the loan account, optional asset account, and monthly loan-shaped schedule as
// one atomic, single-undo operation. Validation errors leave the wizard open
// with per-field errors set.
func (s *loanSurface) submitNew(deps loanDeps) tea.Cmd {
	d, st := s.dlg, s.state
	if d == nil || st == nil {
		return nil
	}
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return nil
	}
	d.ClearErrors()
	hasErr := false

	setErr := func(idx int, msg string) {
		fields[idx].Error = msg
		hasErr = true
	}

	name := strings.TrimSpace(fields[loanFieldName].Value)
	if name == "" {
		setErr(loanFieldName, "Loan name is required")
	}

	owed, err := parseAmountInput(fields[loanFieldCurrentBalance].Value)
	if err != nil || !owed.IsPositive() {
		setErr(loanFieldCurrentBalance, "Enter the (positive) balance you owe today")
	}

	apr, err := parseAmountInput(fields[loanFieldAPR].Value)
	if err != nil {
		setErr(loanFieldAPR, "Invalid APR")
	} else if apr.Float64() < 0 || apr.Float64() >= 100 {
		setErr(loanFieldAPR, "APR must be between 0 and 100")
	}

	pi, err := parseAmountInput(fields[loanFieldPayment].Value)
	if err != nil || !pi.IsPositive() {
		setErr(loanFieldPayment, "Enter the monthly P&I payment")
	}

	nextDate, err := parseDateInput(fields[loanFieldNextPaymentDate].Value)
	if err != nil {
		setErr(loanFieldNextPaymentDate, "Invalid date (MM/DD/YYYY)")
	}

	fromIdx := fields[loanFieldFromAccount].SelectedIndex
	if fromIdx < 0 || fromIdx >= len(st.accountIDs) {
		d.SetErrorMsg("Pick a funding account (no eligible accounts found)")
		hasErr = true
	}

	// Negative-amortization guard: the P&I payment must exceed the first
	// month's interest. Validated up front so the user sees a field error
	// rather than a failed save.
	if !hasErr {
		if _, _, _, sErr := loan.SplitPayment(owed, apr, pi); sErr != nil {
			setErr(loanFieldPayment, "Payment does not cover the first month's interest")
		}
	}

	// Escrow lines: each row with a category selected needs a positive amount.
	var escrow []scheduled.LoanEscrowLine
	for k := range loanMaxEscrowLines {
		catField := fields[loanEscrowCatIndex(k)]
		if catField.SelectedIndex <= 0 {
			continue // (None): an unused escrow row
		}
		catID := st.categoryIDs[catField.SelectedIndex]
		amtIdx := loanEscrowAmtIndex(k)
		amt, aErr := parseAmountInput(fields[amtIdx].Value)
		if aErr != nil || !amt.IsPositive() {
			setErr(amtIdx, "Enter a positive escrow amount")
			continue
		}
		escrow = append(escrow, scheduled.LoanEscrowLine{CategoryID: catID, Amount: amt})
	}

	// Asset section.
	trackAsset := fields[loanFieldTrackAsset].Checked
	assetName := strings.TrimSpace(fields[loanFieldAssetName].Value)
	var assetValue types.Money
	if trackAsset {
		if assetName == "" {
			setErr(loanFieldAssetName, "Asset name is required")
		}
		assetValue, err = parseAmountInput(fields[loanFieldAssetValue].Value)
		if err != nil {
			setErr(loanFieldAssetValue, "Invalid amount")
		}
	}

	if hasErr {
		return nil
	}

	// Resolve the funding account (for currency inheritance) and prefill-only
	// origination inputs.
	fromID := st.accountIDs[fromIdx]
	currency := "USD"
	for _, acc := range st.accounts {
		if acc != nil && acc.ID == fromID {
			currency = acc.Currency
			break
		}
	}

	openingDate := loanOpeningDate(fields, owed)

	institution := strings.TrimSpace(fields[loanFieldInstitution].Value)
	payeeName := strings.TrimSpace(fields[loanFieldPayee].Value)
	autoPost := fields[loanFieldAutoPost].Checked

	// Resolve the interest category selection: the default row get-or-creates
	// Loan:Interest at save time; any other row uses the selected category.
	aprPositive := apr.IsPositive()
	interestField := fields[loanFieldInterestCategory]
	interestDefault := false
	interestPickedID := types.NilID
	if aprPositive {
		idx := interestField.SelectedIndex
		if idx >= 0 && idx < len(st.interestOptions) {
			if st.interestOptions[idx] == loanInterestDefaultDisplay {
				interestDefault = true
			} else {
				interestPickedID = st.interestIDs[idx]
			}
		} else {
			interestDefault = true
		}
	}

	// Principal category (independent of APR; the principal line always exists).
	principalDefault, principalPickedID := resolveLoanPrincipalSelection(st, fields[loanFieldPrincipalCategory])

	s.close()

	return func() tea.Msg {
		accountSvc, categorySvc, payeeSvc, schedSvc, undoMgr :=
			deps.accounts(), deps.categories(), deps.payees(), deps.scheduled(), deps.undo()
		if accountSvc == nil || schedSvc == nil || undoMgr == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		// Payee (get-or-create outside the atomic unit, matching the paycheck
		// wizard: a shared payee must not be deleted on undo).
		var payeeID types.ID
		if payeeName != "" && payeeSvc != nil {
			py, _, pErr := payeeSvc.GetOrCreate(payeeName)
			if pErr != nil {
				return errMsg{err: fmt.Errorf("failed to create payee: %w", pErr)}
			}
			payeeID = py.ID
		}

		// Interest category (also get-or-create outside the atomic unit).
		interestCatID := interestPickedID
		if aprPositive && interestDefault {
			if categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := categorySvc.GetOrCreateLoanInterestCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve interest category: %w", cErr)}
			}
			interestCatID = cat.ID
		}

		// Principal category (also get-or-create outside the atomic unit).
		principalCatID := principalPickedID
		if principalDefault {
			if categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := categorySvc.GetOrCreateLoanPrincipalCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve principal category: %w", cErr)}
			}
			principalCatID = cat.ID
		}

		// Loan account: liabilities are stored negative.
		loanAcct := account.NewAccount(name, account.TypeLoan, currency, owed.Neg(), openingDate)
		loanAcct.SetInterestRate(apr)
		if institution != "" {
			loanAcct.SetInstitution(institution)
		}

		// Month-one snapshot + schedule assembly (mirrors ComputeLoanSplits;
		// clamped final + zero-interest omission handled inside the shared
		// BuildLoanSchedule, which the CLI `loan add` also uses so both create an
		// identical loan-shaped schedule).
		schedule, _, bErr := scheduled.BuildLoanSchedule(fromID, nextDate, payeeID, autoPost, scheduled.LoanSnapshotInput{
			LoanAccountID:  loanAcct.ID,
			APR:            apr,
			Owed:           owed,
			PIPayment:      pi,
			InterestCatID:  interestCatID,
			PrincipalCatID: principalCatID,
			Escrow:         escrow,
		})
		if bErr != nil {
			return errMsg{err: fmt.Errorf("failed to build loan schedule: %w", bErr)}
		}

		// Assemble the atomic, single-undo compound: loan account → optional
		// asset account → schedule. CompoundCommand rolls back earlier steps if
		// a later one fails.
		cmds := []undo.Command{undo.NewCreateAccountCommand(accountSvc, loanAcct)}
		if trackAsset {
			assetAcct := account.NewAccount(assetName, account.TypeAsset, currency, assetValue, openingDate)
			cmds = append(cmds, undo.NewCreateAccountCommand(accountSvc, assetAcct))
		}
		cmds = append(cmds, undo.NewCreateScheduledTransactionCommand(schedSvc, schedule))

		compound := undo.NewCompoundCommand("Create loan", cmds...)
		if err := undoMgr.Execute(compound); err != nil {
			return errMsg{err: fmt.Errorf("failed to create loan: %w", err)}
		}
		return loanWizardSavedMsg{}
	}
}

// submitEdit validates the Edit-as-loan form and, on success, applies
// the loan-account edits (name / institution / APR) and rewrites the schedule's
// month-one template snapshot (rebalanced, freshly tagged — this also (re)tags a
// loan-adoptable schedule, promoting it to strictly loan-shaped) as one atomic,
// single-undo operation. owed is the loan's live balance loaded when the wizard
// opened, so the rebuilt snapshot matches what the next post would compute.
func (s *loanSurface) submitEdit(deps loanDeps) tea.Cmd {
	d, st := s.dlg, s.state
	if d == nil || st == nil || st.existingSchedule == nil || st.loanAccount == nil {
		return nil
	}
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return nil
	}
	d.ClearErrors()
	hasErr := false
	setErr := func(idx int, msg string) {
		fields[idx].Error = msg
		hasErr = true
	}

	name := strings.TrimSpace(fields[loanFieldName].Value)
	if name == "" {
		setErr(loanFieldName, "Loan name is required")
	}

	apr, err := parseAmountInput(fields[loanFieldAPR].Value)
	if err != nil {
		setErr(loanFieldAPR, "Invalid APR")
	} else if apr.Float64() < 0 || apr.Float64() >= 100 {
		setErr(loanFieldAPR, "APR must be between 0 and 100")
	}

	pi, err := parseAmountInput(fields[loanFieldPayment].Value)
	if err != nil || !pi.IsPositive() {
		setErr(loanFieldPayment, "Enter the monthly P&I payment")
	}

	owed := st.owed
	if !owed.IsPositive() {
		d.SetErrorMsg("This loan appears to be paid off — nothing to edit.")
		hasErr = true
	}

	if !hasErr {
		if _, _, _, sErr := loan.SplitPayment(owed, apr, pi); sErr != nil {
			setErr(loanFieldPayment, "Payment does not cover the first month's interest")
		}
	}

	var escrow []scheduled.LoanEscrowLine
	for k := range loanMaxEscrowLines {
		catField := fields[loanEscrowCatIndex(k)]
		if catField.SelectedIndex <= 0 {
			continue
		}
		catID := st.categoryIDs[catField.SelectedIndex]
		amtIdx := loanEscrowAmtIndex(k)
		amt, aErr := parseAmountInput(fields[amtIdx].Value)
		if aErr != nil || !amt.IsPositive() {
			setErr(amtIdx, "Enter a positive escrow amount")
			continue
		}
		escrow = append(escrow, scheduled.LoanEscrowLine{CategoryID: catID, Amount: amt})
	}

	if hasErr {
		return nil
	}

	institution := strings.TrimSpace(fields[loanFieldInstitution].Value)
	autoPost := fields[loanFieldAutoPost].Checked

	aprPositive := apr.IsPositive()
	interestField := fields[loanFieldInterestCategory]
	interestDefault := false
	interestPickedID := types.NilID
	if aprPositive {
		idx := interestField.SelectedIndex
		if idx >= 0 && idx < len(st.interestOptions) {
			if st.interestOptions[idx] == loanInterestDefaultDisplay {
				interestDefault = true
			} else {
				interestPickedID = st.interestIDs[idx]
			}
		} else {
			interestDefault = true
		}
	}

	// Principal category (independent of APR; the principal line always exists).
	principalDefault, principalPickedID := resolveLoanPrincipalSelection(st, fields[loanFieldPrincipalCategory])

	schedule := st.existingSchedule
	loanAcct := st.loanAccount

	s.close()

	return func() tea.Msg {
		accountSvc, categorySvc, schedSvc, undoMgr :=
			deps.accounts(), deps.categories(), deps.scheduled(), deps.undo()
		if accountSvc == nil || schedSvc == nil || undoMgr == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		interestCatID := interestPickedID
		if aprPositive && interestDefault {
			if categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := categorySvc.GetOrCreateLoanInterestCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve interest category: %w", cErr)}
			}
			interestCatID = cat.ID
		}

		principalCatID := principalPickedID
		if principalDefault {
			if categorySvc == nil {
				return errMsg{err: fmt.Errorf("category service not available")}
			}
			cat, cErr := categorySvc.GetOrCreateLoanPrincipalCategory()
			if cErr != nil {
				return errMsg{err: fmt.Errorf("failed to resolve principal category: %w", cErr)}
			}
			principalCatID = cat.ID
		}

		parent, splits, _, bErr := scheduled.BuildLoanSnapshot(scheduled.LoanSnapshotInput{
			LoanAccountID:  loanAcct.ID,
			APR:            apr,
			Owed:           owed,
			PIPayment:      pi,
			InterestCatID:  interestCatID,
			PrincipalCatID: principalCatID,
			Escrow:         escrow,
		})
		if bErr != nil {
			return errMsg{err: fmt.Errorf("failed to rebuild loan schedule: %w", bErr)}
		}

		// Apply loan-account edits.
		loanAcct.Name = name
		loanAcct.SetInterestRate(apr)
		loanAcct.SetInstitution(institution)

		// Rewrite the template snapshot with fresh tags; preserve cadence,
		// routing (account/next date), payee, memo, and duration.
		schedule.SetAmount(parent)
		schedule.ClearCategory()
		schedule.SetAutoPost(autoPost)
		schedule.Splits = scheduled.SplitCollection(splits)

		// One atomic, single-undo operation: account edit + template rewrite.
		compound := undo.NewCompoundCommand("Edit loan",
			undo.NewEditAccountCommand(accountSvc, loanAcct),
			undo.NewEditScheduledTransactionCommand(schedSvc, schedule),
		)
		if err := undoMgr.Execute(compound); err != nil {
			return errMsg{err: fmt.Errorf("failed to save loan edit: %w", err)}
		}
		return loanWizardSavedMsg{}
	}
}

// loanOpeningDate applies the spec's opening-date rule: use the provided open
// date only for a new loan at origination — the open date is set and the
// current balance equals the original principal — otherwise today (a mid-life
// balance is a today snapshot with no history behind it).
func loanOpeningDate(fields []*dialog.Field, owed types.Money) types.Date {
	openField := fields[loanFieldOpenDate]
	if dialog.IsBlankDateInput(openField.Value) {
		return types.Today()
	}
	openDate, err := parseDateInput(openField.Value)
	if err != nil {
		return types.Today()
	}
	origStr := strings.TrimSpace(fields[loanFieldOrigPrincipal].Value)
	if origStr == "" {
		return types.Today()
	}
	orig, err := parseAmountInput(origStr)
	if err != nil || !orig.Equal(owed) {
		return types.Today()
	}
	return openDate
}

// afterLoanWizardSave toasts the creation and reloads every view the new loan
// account and its schedule appear in.
func (a *App) afterLoanWizardSave() tea.Cmd {
	if a.statusbar != nil {
		a.statusbar.SetToast("Loan created.", widget.NotificationInfo)
	}
	return tea.Batch(
		a.loadSidebarData(),
		a.loadDashboardData(),
		a.loadScheduledViewData(),
		a.loadScheduledDueCount(),
		widget.ClearToastCmd(),
	)
}
