package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// transferDialogMode distinguishes a New-Transfer dialog from an
// Edit-Transfer dialog.
type transferDialogMode int

const (
	transferDialogModeNew transferDialogMode = iota
	transferDialogModeEdit
)

// accountTypeByID returns the Type for the account with the given ID, or the
// zero value if the account is not in the slice. Unknown accounts dispatch as
// non-investment, falling through to the regular transfer path where the
// existing service-layer guards take over.
func accountTypeByID(accounts []*account.Account, id types.ID) account.Type {
	for _, a := range accounts {
		if a.ID == id {
			return a.Type
		}
	}
	return ""
}

// accountByID returns the account with the given ID from the loaded list, or nil.
func accountByID(accounts []*account.Account, id types.ID) *account.Account {
	for _, a := range accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// transferOpeningDateError returns the inline date-field error when date is
// before the opening date of any of the named accounts, or "".
func transferOpeningDateError(accounts []*account.Account, date types.Date, ids ...types.ID) string {
	for _, id := range ids {
		if acct := accountByID(accounts, id); acct != nil {
			if msg := openingDateFieldError(acct, date); msg != "" {
				return msg
			}
		}
	}
	return ""
}

// transferDialogData holds the loaded data needed for the transfer dialog.
type transferDialogData struct {
	accounts   []*account.Account
	accountIDs []types.ID // parallel to account dropdown options
	categories []*category.Category

	// Edit-mode-only. Zero in new mode.
	//
	// One payload for every shape. There used to be two — a
	// *transaction.TransferPair for bank↔bank and an investmentTransferEdit
	// whenever a leg lived in an investment account — because the two submit
	// paths took different services with different argument orders. With one
	// service there is one payload, and it is the read model itself.
	mode     transferDialogMode
	existing *transfer.Transfer
}

// transferDialogDataMsg is sent when transfer dialog data has been loaded.
type transferDialogDataMsg struct {
	data *transferDialogData
}

// transferDialogSavedMsg is sent when a transfer has been saved.
// savedDate carries the transaction date so the App can use it as the
// session's sticky-date seed for subsequent dialog opens.
//
// savedID is the ID of the transfer leg living in the register the user is
// currently viewing (NilID if neither leg belongs to the current register),
// so the register can move the cursor onto the just-saved row. savedIsInvestment
// reports whether that leg is an investment transaction, which tells the handler
// whether to select it in the investment register or the regular register.
type transferDialogSavedMsg struct {
	savedDate         types.Date
	savedID           types.ID
	savedIsInvestment bool
}

// buildAccountOptions builds parallel display name and ID slices for account selectors.
func buildAccountOptions(accounts []*account.Account) ([]string, []types.ID) {
	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))

	for _, acct := range accounts {
		options = append(options, acct.Name)
		ids = append(ids, acct.ID)
	}

	return options, ids
}

// buildTransferDialog creates a dialog.Dialog for entering a new transfer.
// defaultFromIndex is the index of the currently selected account to
// pre-select as "From". categoryOptions is the "(None)"-led combo list
// produced by buildCategoryOptions; the parallel ID slice is held on the App
// as transferDialogCategoryIDs. Field order is
// From, To, Amount, Date, Memo, Category.
func buildTransferDialog(accountOptions, categoryOptions []string, defaultFromIndex int) *dialog.Dialog {
	d := dialog.NewDialog("New Transfer")

	// From account
	d.AddSelectField("From", accountOptions, defaultFromIndex)

	// To account - default to 0 (first account)
	toIndex := 0
	if defaultFromIndex == 0 && len(accountOptions) > 1 {
		toIndex = 1
	}
	d.AddSelectField("To", accountOptions, toIndex)

	// Amount (positive)
	f := d.AddTextField("Amount", "", "100.00", 12)
	f.Required = true

	// Date field - masked MM/DD/YYYY, defaults to today
	f = d.AddDateField("Date", "")
	f.Required = true

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	// Category — optional label for the transfer, with inline creation. Not
	// applicable to inv→inv transfers (validated at submit).
	catField := d.AddComboField("Category", categoryOptions, 0)
	catField.AddNewLabel = "[+ Add new category…]"

	d.SetVisible(true)
	return d
}

// buildEditTransferDialog builds the edit-mode transfer dialog. Title is
// "Edit Transfer"; the From/To accounts are rendered as a read-only body
// message ("Checking → Savings") since UpdateTransfer cannot move a
// transfer between accounts. Editable fields are Amount (positive), Date,
// Memo, an optional Category, and Status — all pre-filled from the supplied
// values.
//
// The Category combo appears when includeCategory is true — i.e. for every
// transfer with a regular-side leg that can store a category (bank↔bank and
// inv↔reg). It is omitted for inv→inv transfers, where neither leg can hold
// one; there the layout stays Amount, Date, Memo, Status as before.
// categoryIdx pre-selects the combo from the seeding leg's category (0 =
// "(None)").
func buildEditTransferDialog(fromName, toName string, amount types.Money, date types.Date, memo string, status transaction.Status, includeCategory bool, categoryOptions []string, categoryIdx int) *dialog.Dialog {
	d := dialog.NewDialog("Edit Transfer")
	d.SetMessage(fromName + " → " + toName)

	f := d.AddTextField("Amount", amount.String(), "100.00", 12)
	f.Required = true

	f = d.AddDateField("Date", date.Time().Format("01/02/2006"))
	f.Required = true

	d.AddTextField("Memo", memo, "Optional memo", 0)

	if includeCategory {
		catField := d.AddComboField("Category", categoryOptions, categoryIdx)
		catField.AddNewLabel = "[+ Add new category…]"
	}

	statusIdx := 0
	if status == transaction.StatusCleared {
		statusIdx = 1
	}
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, statusIdx)

	d.SetVisible(true)
	return d
}

// editTransferIncludesCategory reports whether the edit-mode Transfer dialog
// for the given loaded data should show a Category combo. Every transfer with
// a regular-side leg (bank↔bank via data.existing, or inv↔reg via
// data.existingInvestment where exactly one leg is investment-typed) can carry
// a category; an inv↔inv transfer (both legs investment) cannot, so the field
// is omitted there. Both the dialog builder and the submit handler consult
// this so field indices agree.
func editTransferIncludesCategory(data *transferDialogData) bool {
	if data == nil || data.existing == nil {
		return false
	}
	// The domain predicate, not a re-derivation from the dialog's account list.
	// The old version inferred both legs' types via accountTypeByID, which
	// returns "" for an account missing from the ACTIVE-only list — so a
	// transfer with a closed leg could get the wrong answer.
	return data.existing.Kind.StoresCategory()
}

// categoryComboIndex resolves the combo SelectedIndex for catID against the
// parallel category ID slice (index 0 is the "(None)" sentinel). Returns 0
// when catID is unset or not found.
func categoryComboIndex(ids []types.ID, catID types.NullableID) int {
	if !catID.Valid {
		return 0
	}
	for i, id := range ids {
		if id == catID.ID {
			return i
		}
	}
	return 0
}

// transferDeps is what the transfer surface needs from outside itself. Two
// constraints shape it, and both are live:
//
//   - Every dep is a func, because switchDatabase re-points App's service
//     fields and closes the previous *db.DB. A closure re-reads the field at
//     call time; a captured pointer would be a use-after-close.
//   - Deps are passed to each call and never stored on the surface, because
//     close() resets the surface to its zero value and would zero them.
//
// Both are pinned by tests: TestTransferDeps_FollowADatabaseSwitch and
// TestGuard_NoSurfaceStructHoldsItsDeps.
type transferDeps struct {
	accounts   func() *account.Service
	categories func() *category.Service
	transfers  func() *transfer.Service
	undo       func() *undo.Manager
}

// transferDeps binds the transfer surface to the services App owns. Every
// accessor may legitimately return nil — an App built by a test has no services
// — so each caller keeps its own nil guard.
func (a *App) transferDeps() transferDeps {
	return transferDeps{
		accounts:   func() *account.Service { return a.accountSvc },
		categories: func() *category.Service { return a.categorySvc },
		transfers:  func() *transfer.Service { return a.transferSvc },
		undo:       func() *undo.Manager { return a.undoManager },
	}
}

// transferAccountNames resolves the From/To account display names for the
// pair carried by a transferDialogData in edit mode. Falls back to "(unknown)"
// when an account isn't present in data.accounts. Handles both the regular-
// pair shape (data.existing) and the inv-involving shape (data.existingInvestment).
func transferAccountNames(data *transferDialogData) (fromName, toName string) {
	fromName = "(unknown)"
	toName = "(unknown)"
	if data == nil {
		return
	}
	if data.existing == nil {
		return
	}
	// The resolved transfer carries its own loaded accounts, so a leg in a
	// CLOSED account still renders its real name rather than "(unknown)" —
	// data.accounts is the active-only list, which is what the old lookup
	// scanned.
	if data.existing.FromAccount != nil {
		fromName = data.existing.FromAccount.Name
	}
	if data.existing.ToAccount != nil {
		toName = data.existing.ToAccount.Name
	}
	return
}

// transferSurface is the Transfer dialog together with the form state that
// belongs to it. Its zero value is closed; close() resets it to that.
type transferSurface struct {
	modalSurface
	data        *transferDialogData
	accountIDs  []types.ID
	categoryIDs []types.ID // parallel to the Category combo options
}

func (s *transferSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// applyData builds either the edit form or the new form over the loaded
// accounts and categories. selectedAccountID pre-selects the "From" account in
// new mode and stickyDate seeds its Date field; edit mode reads neither,
// because the transfer being edited supplies both.
//
// An edit with no existing transfer leaves the dialog unbuilt, so nothing
// paints and the surface stays closed.
func (s *transferSurface) applyData(data *transferDialogData, selectedAccountID types.ID, stickyDate types.Date) {
	s.data = data
	accountOptions, accountIDs := buildAccountOptions(data.accounts)
	s.accountIDs = accountIDs
	// Category combo options are the "(None)"-led, system-excluded list;
	// the parallel ID slice is stashed for the submit handler.
	categoryOptions, categoryIDs := buildCategoryOptions(data.categories)
	s.categoryIDs = categoryIDs

	if data.mode == transferDialogModeEdit {
		t := data.existing
		if t == nil {
			return
		}
		fromName, toName := transferAccountNames(data)
		s.dlg = buildEditTransferDialog(
			fromName, toName, t.Amount, t.Date, t.Memo, t.Status,
			editTransferIncludesCategory(data), categoryOptions,
			categoryComboIndex(categoryIDs, t.CategoryID))
		return
	}

	// Pre-select the currently selected sidebar account as "From"
	defaultFromIndex := 0
	for i, id := range accountIDs {
		if id == selectedAccountID {
			defaultFromIndex = i
			break
		}
	}
	s.dlg = buildTransferDialog(accountOptions, categoryOptions, defaultFromIndex)
	s.dlg.SeedDateField(stickyDate)
}

// categoryFieldIndex returns the dialog-field index of the Category combo for
// the currently open transfer dialog, or -1 when the dialog has no Category
// field. Create mode lays out From, To, Amount, Date, Memo, Category (index 5);
// the edit modes that carry a category lay out
// Amount, Date, Memo, Category, Status (index 3); an inv↔inv edit omits the
// combo entirely, so it reports -1 rather than pointing at the Status radio.
func (s *transferSurface) categoryFieldIndex() int {
	if s.data != nil && s.data.mode == transferDialogModeEdit {
		if !editTransferIncludesCategory(s.data) {
			return -1
		}
		return 3
	}
	return 5
}

// open returns the command that loads the accounts and categories a new
// transfer needs. The dialog is built when the resulting message reaches
// applyData, so there is nothing to mutate before the data lands and the
// receiver goes unread — open is a method because opening is the surface's
// operation, not App's.
func (s *transferSurface) open(deps transferDeps) tea.Cmd {
	return func() tea.Msg {
		data := &transferDialogData{}
		if err := loadTransferAccountsAndCategories(deps, data); err != nil {
			return errMsg{err: err}
		}
		return transferDialogDataMsg{data: data}
	}
}

// openForEdit returns the command that loads the dialog's accounts and
// categories, resolves the whole transfer that legRowID belongs to, and emits a
// transferDialogDataMsg in edit mode so the dialog opens pre-filled.
//
// Either leg's row id works — transfer.Resolve does not care which table the
// named leg lives in — so the bank register and the investment register share
// one entry point. They used to call two aliases of it under different names.
//
// This replaced the two hand-rolled cross-table loaders (94 + 81 lines) that
// each probed one table, guessed the counterpart's ledger from the dialog's
// loaded account list, and hunted the counterpart leg by scanning the target
// account's whole transaction history. Both are now one transfer.Resolve call.
//
// It also fixed a real bug in the process. The old bank-side loader decided
// which ledger the counterpart lived in via accountTypeByID(data.accounts, ...),
// and data.accounts comes from accountSvc.List(true) — ACTIVE ONLY. accountTypeByID
// returns "" for an account it cannot find, which reads as non-investment, so an
// inv↔reg transfer whose investment counterpart had been closed was misrouted to
// GetTransferPair and failed. transfer.Resolve reads account rows directly.
func (s *transferSurface) openForEdit(deps transferDeps, legRowID types.ID) tea.Cmd {
	return func() tea.Msg {
		data := &transferDialogData{mode: transferDialogModeEdit}
		if err := loadTransferAccountsAndCategories(deps, data); err != nil {
			return errMsg{err: err}
		}

		svc := deps.transfers()
		if svc == nil {
			return transferDialogDataMsg{data: data}
		}

		t, err := svc.Resolve(legRowID)
		if err != nil {
			return errMsg{err: err}
		}

		// A transfer LINE inside a multi-line split is owned by the parent
		// transaction's split lifecycle. Refuse it here rather than editing it
		// as a standalone transfer: doing that on the investment side deletes
		// both legs and recreates them under a brand-new transfer_id, which
		// permanently orphans the split line AND mints a second regular-side
		// leg in the bank account.
		if t.Shape == transfer.ShapeSplitLine {
			return errMsg{err: fmt.Errorf(
				"this transfer is a line inside a multi-line split; edit it from the parent transaction's splits (parent %s)",
				t.ParentTransactionID.String(),
			)}
		}
		// Share transfers are owned by the investment share-transfer dialog.
		if t.Movement == transfer.MovementShares {
			return errMsg{err: fmt.Errorf("this is a share transfer; edit it with the Transfer Shares dialog")}
		}

		data.existing = t
		return transferDialogDataMsg{data: data}
	}
}

// loadTransferAccountsAndCategories fills the account and category halves of
// data, which both open paths need identically. It runs on the command's own
// goroutine, so it reaches every service through deps rather than holding one.
func loadTransferAccountsAndCategories(deps transferDeps, data *transferDialogData) error {
	if svc := deps.accounts(); svc != nil {
		accounts, err := svc.List(true)
		if err != nil {
			return err
		}
		data.accounts = accounts
	}
	if svc := deps.categories(); svc != nil {
		categories, err := svc.List()
		if err != nil {
			return err
		}
		data.categories = categories
	}
	_, ids := buildAccountOptions(data.accounts)
	data.accountIDs = ids
	return nil
}

// handleKey gives the dialog the key and reports what it wants done about it.
// An unbuilt surface asks for nothing.
func (s *transferSurface) handleKey(msg tea.KeyPressMsg) dialog.DialogAction {
	if s.dlg == nil {
		return dialog.DialogActionNone
	}
	return s.dlg.HandleKey(msg)
}

// close clears the surface. The zero value is the closed state, so this is the
// whole of it — and it stays that way only because the deps are a call
// parameter rather than a field.
func (s *transferSurface) close() { *s = transferSurface{} }

// submit parses the dialog fields, validates them, and returns the command that
// saves the transfer. A nil command means validation failed: the dialog stays
// open carrying its inline errors.
//
// currentAcct is the account whose register is on screen. The leg living there
// should be selected after the save, so a freshly entered transfer scrolls into
// view just like a plain transaction does. It arrives as a value because the
// returned closure runs on another goroutine, where reading App would race.
func (s *transferSurface) submit(deps transferDeps, currentAcct types.ID) tea.Cmd {
	if s.dlg == nil || s.data == nil {
		return nil
	}

	if s.data.mode == transferDialogModeEdit {
		return s.submitEdit(deps, currentAcct)
	}

	fields := s.dlg.Fields()
	if len(fields) < 5 {
		return nil
	}

	s.dlg.ClearErrors()
	hasErrors := false

	// From account
	fromIdx := fields[0].SelectedIndex
	if fromIdx < 0 || fromIdx >= len(s.accountIDs) {
		fields[0].Error = "Please select a From account"
		hasErrors = true
	}
	fromAccountID := types.NilID
	if fromIdx >= 0 && fromIdx < len(s.accountIDs) {
		fromAccountID = s.accountIDs[fromIdx]
	}

	// To account
	toIdx := fields[1].SelectedIndex
	if toIdx < 0 || toIdx >= len(s.accountIDs) {
		fields[1].Error = "Please select a To account"
		hasErrors = true
	}
	toAccountID := types.NilID
	if toIdx >= 0 && toIdx < len(s.accountIDs) {
		toAccountID = s.accountIDs[toIdx]
	}

	// Validate from != to
	if !fromAccountID.IsNil() && !toAccountID.IsNil() && fromAccountID == toAccountID {
		s.dlg.SetErrorMsg("From and To accounts must be different")
		hasErrors = true
	}

	// Parse amount
	amount, err := parseAmountInput(fields[2].Value)
	if err != nil {
		fields[2].Error = "Invalid amount"
		hasErrors = true
	} else if !amount.IsPositive() {
		fields[2].Error = "Amount must be positive"
		hasErrors = true
	}

	// Parse date
	date, err := parseDateInput(fields[3].Value)
	if err != nil {
		fields[3].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	} else if msg := transferOpeningDateError(s.data.accounts, date, fromAccountID, toAccountID); msg != "" {
		fields[3].Error = msg
		hasErrors = true
	}

	if hasErrors {
		return nil
	}

	// Memo
	memo := strings.TrimSpace(fields[4].Value)

	// Category (optional). Field index 5 (From, To, Amount, Date, Memo,
	// Category); guarded so a legacy 5-field dialog degrades to no category.
	var categoryID types.NullableID
	const catFieldIdx = 5
	if len(fields) > catFieldIdx {
		if idx := fields[catFieldIdx].SelectedIndex; idx > 0 && idx < len(s.categoryIDs) {
			categoryID = types.NullableID{ID: s.categoryIDs[idx], Valid: true}
		}
	}

	// Inline field error for a category the transfer cannot store, so the user
	// sees it on the Category field with the dialog still open rather than as a
	// notification after the dialog closes.
	//
	// This CALLS the domain predicate rather than restating the rule — Kind.
	// StoresCategory is the same function transfer.Service's guard uses, so
	// there is one implementation, not two. The domain still refuses
	// independently (*transfer.CategoryNotSupportedError); this is a fast path
	// to a better-placed message, not the authority.
	if categoryID.Valid && len(fields) > catFieldIdx {
		fromType := accountTypeByID(s.data.accounts, fromAccountID)
		toType := accountTypeByID(s.data.accounts, toAccountID)
		if !transfer.ClassifyKind(fromType, toType).StoresCategory() {
			fields[catFieldIdx].Error = "Categories aren't supported on investment-to-investment transfers"
			return nil
		}
	}

	// Close dialog before async save for responsive UI. Everything the save
	// needs is already a local, so the closure never reads the surface it has
	// just dropped.
	s.close()

	// No dispatch. One service call handles every (From, To) combination: the
	// transfer service derives each leg's sign from its side and each leg's
	// table from its own account type. The 4-arm switch that used to live here —
	// with its four undo command types, three result shapes, and the argument
	// flip DepositFromAccount's (investment, regular) parameter order forced —
	// is gone.
	//
	// The inv↔inv category refusal that used to be re-implemented here is also
	// gone: the domain returns *transfer.CategoryNotSupportedError, and the
	// error surface below maps it back onto the Category field.
	return func() tea.Msg {
		undoMgr := deps.undo()
		if undoMgr == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		svc := deps.transfers()
		if svc == nil {
			return errMsg{err: fmt.Errorf("transfer service not available")}
		}

		cmd := undo.NewCreateTransferCommand(svc, transfer.Spec{
			FromAccountID: fromAccountID,
			ToAccountID:   toAccountID,
			Date:          date,
			Amount:        amount,
			Memo:          memo,
			CategoryID:    categoryID,
		})
		if err := undoMgr.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to create transfer: %w", err)}
		}

		var savedID types.ID
		var savedIsInvestment bool
		if res := cmd.Result(); res != nil {
			if leg, ok := res.LegForAccount(currentAcct); ok {
				savedID = leg.RowID
				savedIsInvestment = leg.Ledger == transfer.LedgerInvestment
			}
		}

		return transferDialogSavedMsg{savedDate: date, savedID: savedID, savedIsInvestment: savedIsInvestment}
	}
}

// transferLegForAccount and cashTransferLegForAccount lived here: one to pick a
// leg out of a *transaction.TransferPair, the other out of an
// *investment.CashTransferResult. Both are gone with those two result types.
// transfer.Result.LegForAccount does the job for every shape, and returns the
// leg's ledger rather than a bool, so the caller does not have to know which
// result shape it was handed.

// submitEdit validates the edit-mode transfer dialog and returns the command
// that applies the edit, or nil when validation failed. Edit-mode field layout
// is Amount(0), Date(1), Memo(2), [Category(3)], Status(3|4). One path for every
// shape: the transfer service addresses the edit by transfer_id and rewrites
// both legs in place, wherever they live.
func (s *transferSurface) submitEdit(deps transferDeps, currentAcct types.ID) tea.Cmd {
	existing := s.data.existing
	if existing == nil {
		return nil
	}

	// Field layout depends on whether a Category combo is present. It is for
	// every transfer with a regular-side leg (bank↔bank, inv↔reg):
	//   Amount(0), Date(1), Memo(2), Category(3), Status(4)
	// inv↔inv omits it:
	//   Amount(0), Date(1), Memo(2), Status(3)
	includeCategory := editTransferIncludesCategory(s.data)
	minFields := 4
	statusIdx := 3
	catIdx := -1
	if includeCategory {
		minFields = 5
		catIdx = 3
		statusIdx = 4
	}

	fields := s.dlg.Fields()
	if len(fields) < minFields {
		return nil
	}

	s.dlg.ClearErrors()
	hasErrors := false

	amount, err := parseAmountInput(fields[0].Value)
	if err != nil {
		fields[0].Error = "Invalid amount"
		hasErrors = true
	} else if !amount.IsPositive() {
		fields[0].Error = "Amount must be positive"
		hasErrors = true
	}

	date, err := parseDateInput(fields[1].Value)
	if err != nil {
		fields[1].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	} else {
		// Inline opening-date feedback for both legs. The domain guards this
		// too; this only places the message on the Date field.
		if msg := openingDateFieldError(existing.FromAccount, date); msg != "" {
			fields[1].Error = msg
			hasErrors = true
		} else if msg := openingDateFieldError(existing.ToAccount, date); msg != "" {
			fields[1].Error = msg
			hasErrors = true
		}
	}

	if hasErrors {
		return nil
	}

	memo := strings.TrimSpace(fields[2].Value)

	// Category selection (only when the combo is present). An unset selection
	// ("(None)") produces an invalid NullableID, which clears the category on
	// both legs.
	var categoryID types.NullableID
	if catIdx >= 0 {
		if idx := fields[catIdx].SelectedIndex; idx > 0 && idx < len(s.categoryIDs) {
			categoryID = types.NullableID{ID: s.categoryIDs[idx], Valid: true}
		}
	}

	status := transaction.StatusUncleared
	if fields[statusIdx].SelectedIndex == 1 {
		status = transaction.StatusCleared
	}

	// One edit path for every shape. dispatchInvestmentEditTransfer and its
	// direction-string derivation are gone: the transfer service addresses an
	// edit by transfer_id and edits both legs in place, so there is nothing left
	// to dispatch on.
	transferID := existing.TransferID

	s.close()

	return func() tea.Msg {
		svc := deps.transfers()
		undoMgr := deps.undo()
		if svc == nil || undoMgr == nil {
			return errMsg{err: fmt.Errorf("transfer service not available")}
		}

		cmd := undo.NewEditTransferCommand(svc, transferID, transfer.Edit{
			Date:       date,
			Amount:     amount,
			Memo:       memo,
			CategoryID: categoryID,
			Status:     status,
		})
		if err := undoMgr.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to update transfer: %w", err)}
		}

		// Keep the cursor on the edited row. An edit can change the date and so
		// re-sort the leg; selecting by ID lands on it wherever it moves. Because
		// the edit is now in place, the leg IDs are the SAME after the write —
		// previously the investment path stashed an ID that UpdateTransferCash
		// had just deleted, so the cursor jumped or vanished.
		var savedID types.ID
		var savedIsInvestment bool
		if res := cmd.Result(); res != nil {
			if leg, ok := res.LegForAccount(currentAcct); ok {
				savedID = leg.RowID
				savedIsInvestment = leg.Ledger == transfer.LedgerInvestment
			}
		}
		return transferDialogSavedMsg{savedDate: date, savedID: savedID, savedIsInvestment: savedIsInvestment}
	}
}

// beginCreateCategory takes the Category combo's typed query off the field,
// hides the dialog and reports the parent names the sub-dialog's combo should
// offer. The dialog is kept alive (hidden) so its field state survives the
// divert; applyCreatedCategory re-shows it with the new category selected.
// It reports false when there is no Category field to divert from, in which
// case nothing was hidden and the caller must not open the sub-dialog.
func (s *transferSurface) beginCreateCategory() (query string, parents []string, ok bool) {
	if s.dlg == nil {
		return "", nil, false
	}
	fields := s.dlg.Fields()
	idx := s.categoryFieldIndex()
	if idx < 0 || idx >= len(fields) {
		return "", nil, false
	}
	catField := fields[idx]
	query = catField.Query
	catField.AddNewTriggered = false
	catField.Query = ""

	parents, _ = s.parentCategoryNames()
	s.dlg.SetVisible(false)
	return query, parents, true
}

// parentCategoryNames is the top-level parent list drawn from the categories
// this surface loaded, for the create-category sub-dialog's parent combo. ok is
// false when the surface has loaded nothing, which is a different answer from
// "loaded, and there are no parents": the router falls back to a live service
// call only for the first one.
func (s *transferSurface) parentCategoryNames() (names []string, ok bool) {
	if s.data == nil {
		return nil, false
	}
	return topLevelParentNames(s.data.categories), true
}

// applyCreatedCategory reloads the Category combo's options with newCat
// selected and re-shows the dialog. Persistence already happened in
// persistCategory; the router passes the fresh category in. The persist is
// asynchronous, so a surface the user has closed in the meantime is left alone.
func (s *transferSurface) applyCreatedCategory(newCat *category.Category, cats []*category.Category) {
	if s.data != nil {
		s.data.categories = cats
	}
	options, ids := buildCategoryOptions(cats)
	s.categoryIDs = ids

	idx := s.categoryFieldIndex()
	if s.dlg == nil || idx < 0 || idx >= len(s.dlg.Fields()) {
		return
	}
	catField := s.dlg.Fields()[idx]
	catField.Options = options
	newIdx := 0
	for i, id := range ids {
		if id == newCat.ID {
			newIdx = i
			break
		}
	}
	catField.SelectedIndex = newIdx
	catField.ComboHighlight = newIdx
	s.dlg.SetVisible(true)
}

// reshow makes the dialog visible again after a create-category divert the user
// cancelled. Nil-safe, because the divert can outlive the surface.
func (s *transferSurface) reshow() {
	if s.dlg != nil {
		s.dlg.SetVisible(true)
	}
}

// -----------------------------------------------------------------------------
// App glue. Everything below is a thin wrapper supplying the services, the
// on-screen register account, and the divert into a sibling surface. No
// behaviour lives here: an action the surface owns must not have a second
// implementation on App.
// -----------------------------------------------------------------------------

// handleTransferDialogKey routes key events to the transfer dialog.
func (a *App) handleTransferDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.transferDialogAction(a.transfer.handleKey(msg))
}

// transferDialogAction dispatches a DialogAction for the transfer dialog, from
// either input path.
//
// AddNew is the one arm that stays on App: it writes createCat, which is
// another surface's state, and a surface must not reach into a sibling. That
// is the whole of what this pilot could not move.
func (a *App) transferDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionSubmit:
		return a, a.transfer.submit(a.transferDeps(), a.currentRegisterAccountID())
	case dialog.DialogActionCancel:
		a.transfer.close()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogForTransfer()
	}

	return a, nil
}

// openCreateCategorySubDialogForTransfer opens the inline create-category
// sub-dialog over the (now hidden) transfer dialog, seeded with the Category
// combo's typed query. Transfers are labeled for spending tracking (credit-card
// payments, loan principal), so the sub-dialog defaults to an Expense type —
// unlike the amount-bearing surfaces, a transfer's amount is always a positive
// magnitude and carries no income/expense signal.
func (a *App) openCreateCategorySubDialogForTransfer() (tea.Model, tea.Cmd) {
	query, parents, ok := a.transfer.beginCreateCategory()
	if !ok {
		return a, nil
	}
	parent, name := splitCategoryQuery(query)
	a.createCat.dlg = buildCreateCategoryDialog(name, parent, parents, category.TypeExpense)
	a.createCat.origin.surface = createCatSourceTransferDialog
	return a, nil
}

// applyCreatedCategoryToTransfer is the per-surface applier for the
// create-category router when the originating surface was the Transfer dialog.
// The surface applies the category to its own field; App closes the sub-dialog,
// which is its own state.
func (a *App) applyCreatedCategoryToTransfer(newCat *category.Category, cats []*category.Category) {
	a.transfer.applyCreatedCategory(newCat, cats)
	a.createCat.dlg = nil
}

// afterTransferSave applies the state a saved transfer leaves behind: the
// session sticky date, the end of any investment edit, the leg to select once
// the reload lands, and the status-bar note.
func (a *App) afterTransferSave(msg transferDialogSavedMsg) tea.Cmd {
	a.rememberSavedDate(msg.savedDate)
	a.investmentEditTxnID = types.NilID
	// Select the saved leg in whichever register the user is viewing so the
	// new transfer scrolls into view, mirroring plain transactions and splits.
	if !msg.savedID.IsNil() {
		if msg.savedIsInvestment {
			a.pendingInvestmentSelectID = msg.savedID
		} else {
			a.pendingRegisterSelectID = msg.savedID
		}
	}
	a.statusbar.AddNotification("Transfer saved", widget.NotificationInfo)
	return a.reloadCurrentView()
}
