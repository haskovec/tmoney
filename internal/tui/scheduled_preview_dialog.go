package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// dialog.Field indices for the SchedulePreviewDialog's header dialog.
//
// Single-line preview fields:
//
//	Date(0) Payee(1) Category(2) Amount(3) Memo(4) Status(5)
//
// Multi-line preview header fields (lines live in the embedded
// SplitDialog; the header carries no scalar category or amount):
//
//	Date(0) Payee(1) Memo(2) Status(3)
const (
	previewFieldDate          = 0
	previewFieldPayee         = 1
	previewSingleFieldCat     = 2
	previewSingleFieldAmount  = 3
	previewSingleFieldMemo    = 4
	previewSingleFieldStatus  = 5
	previewMultiFieldMemo     = 2
	previewMultiFieldStatus   = 3
	previewStatusUnclearedIdx = 0

	// Transfer preview fields. From/To render as a read-only body message
	// ("Checking → Visa"); Date / Amount / Category / Memo / Status are
	// editable. The Category combo enables a one-off relabel of this
	// occurrence without touching the template.
	previewXferFieldDate     = 0
	previewXferFieldAmount   = 1
	previewXferFieldCategory = 2
	previewXferFieldMemo     = 3
	previewXferFieldStatus   = 4
)

// SchedulePreviewDialog is the Quicken-style "post one occurrence" dialog
// that opens when the user presses Enter on a due scheduled transaction.
// It is pre-filled with the schedule's template values; edits made here
// flow into the real transaction created at save time but do not modify
// the template (one-off semantics — see
// specs/multiline-splits-and-paycheck.md, Post-Time Preview dialog.Dialog).
//
// The dialog has two shapes:
//   - For a single-line schedule it owns one dialog.Dialog with date / payee /
//     category / amount / memo / status fields, mirroring the regular
//     transaction edit dialog.
//   - For a multi-line schedule it owns a header dialog.Dialog (date / payee /
//     memo / status) plus an embedded SplitDialog seeded from the
//     template's children. The header carries no scalar category or
//     amount since the lines own those.
//
// MS-018 introduces the scaffolding only — the wiring that opens this
// dialog from the Scheduled view (MS-019) and the save/cancel handlers
// that create the real transaction and advance the schedule (MS-020)
// land in subsequent slices.
type SchedulePreviewDialog struct {
	// template is the scheduled transaction whose next occurrence is
	// being previewed. It is read-only from the preview's perspective —
	// edits do not flow back to the stored template.
	template *scheduled.Transaction

	// headerDialog carries the editable parent-transaction fields. For
	// a single-line schedule this includes Category and Amount; for a
	// multi-line schedule those live in splitDialog and the header is
	// only date / payee / memo / status.
	headerDialog *dialog.Dialog

	// splitDialog is non-nil for multi-line schedules and edits the
	// line items.
	splitDialog *SplitDialog

	// categoryIDs parallels the headerDialog's category combo options,
	// so the submit handler can map the selected index back to a
	// category ID. Nil for multi-line previews (the header has no
	// scalar category field).
	categoryIDs []types.ID

	// payees is the snapshot of payees passed to the constructor. The
	// submit handler looks up the existing payee by name (case-
	// insensitive); a new name is created via the payee service.
	payees []*payee.Payee

	// splitFocus tracks which surface receives key events on multi-line
	// previews — false means the header dialog, true means the embedded
	// split editor. Tab past the header's last focusable position
	// transitions focus into the split editor; Shift+Tab from the split
	// editor's first focus transitions back to the header. Always false
	// on single-line previews.
	splitFocus bool

	// loanShaped is true when the previewed schedule is loan-shaped: its
	// interest/principal split was seeded from the loan's live balance
	// (ComputeLoanSplits) rather than copied verbatim from the template, and
	// it gets the reseed-on-date rule and payoff toast. Always multi-line.
	loanShaped bool

	// loanSeedDate is the occurrence date the current loan seed was computed
	// for. A Date-field edit to a different date reseeds the split (until
	// loanSeedFrozen is set); see the reseed rule in specs/loan-wizard.md.
	loanSeedDate types.Date

	// loanSeedFrozen becomes true once the user edits any line amount in a
	// loan-shaped preview. After that, user values win and Date edits no
	// longer reseed the computed split.
	loanSeedFrozen bool

	// loanSeededRows snapshots a signature of each split row (category /
	// transfer target / amount / memo) right after each loan (re)seed, so any
	// subsequent line edit — not just an amount change — is detected and
	// freezes reseeding (a category/memo edit is user intent the reseed would
	// otherwise silently discard when it rebuilds the editor).
	loanSeededRows []string

	// accounts / categoryOptions are stashed at construction so a
	// date-change reseed can rebuild the embedded split editor exactly as
	// the constructor did (transfer targets + category resolution).
	accounts        []*account.Account
	categoryOptions []string
}

// NewSchedulePreviewDialog builds the preview dialog for one due
// occurrence of a scheduled transaction.
//
// The dialog is seeded entirely from the template:
//   - The Date field is pre-filled with template.NextDate (one-off date
//     edits never shift the schedule's cadence; see MS-021).
//   - For a single-line schedule, Category, Amount, Payee, and Memo
//     come straight from the template's scalar fields.
//   - For a multi-line schedule, the embedded SplitDialog is seeded
//     from template.Splits via transactionSplitsFromScheduled; the
//     transfer-target picker excludes the schedule's own account so
//     self-transfers stay impossible.
//
// payees / categoryOptions / categoryIDs / accounts are passed in by
// the caller to keep this function pure (it never touches services).
//
// loanSplits is non-nil only for loan-shaped schedules: the caller
// (loadSchedulePreviewData) recomputes the month's interest/principal split
// from the loan's live balance and passes it in so the embedded split editor
// is seeded with the computed values instead of the stored template. When it
// is non-nil the preview is marked loan-shaped and gets the reseed-on-date
// rule and the payoff toast.
func NewSchedulePreviewDialog(
	template *scheduled.Transaction,
	accounts []*account.Account,
	payees []*payee.Payee,
	categoryOptions []string,
	categoryIDs []types.ID,
	loanSplits *scheduled.LoanSplits,
) *SchedulePreviewDialog {
	if template == nil {
		return nil
	}

	p := &SchedulePreviewDialog{
		template:        template,
		payees:          payees,
		accounts:        accounts,
		categoryOptions: categoryOptions,
		categoryIDs:     categoryIDs,
	}

	payeeName := ""
	if template.HasPayee() {
		for _, py := range payees {
			if py == nil {
				continue
			}
			if py.ID == template.PayeeID.ID {
				payeeName = py.Name
				break
			}
		}
	}

	memo := ""
	if template.Memo.Valid {
		memo = template.Memo.String
	}

	dateStr := template.NextDate.Time().Format("01/02/2006")

	if template.IsTransfer() {
		fromName := accountNameByID(accounts, template.AccountID)
		toName := accountNameByID(accounts, template.TransferAccountID.ID)
		amountStr := ""
		if template.HasAmount() {
			amountStr = template.Amount.Money.Abs().String()
		}
		catIdx := 0
		if template.HasCategory() {
			for i, id := range categoryIDs {
				if id == template.CategoryID.ID {
					catIdx = i
					break
				}
			}
		}
		p.categoryIDs = categoryIDs
		p.headerDialog = buildPreviewHeaderTransfer(fromName, toName, dateStr, amountStr, memo, categoryOptions, catIdx)
		return p
	}

	if len(template.Splits) > 0 {
		p.headerDialog = buildPreviewHeaderMulti(dateStr, payeeName, memo)

		// A loan-shaped schedule seeds its interest/principal split from the
		// live-balance recompute the caller supplied, not the stored template
		// (which is only a month-one snapshot). Everything else — a paycheck
		// or a hand-built multi-line schedule — posts its template lines.
		parentAmount := template.Amount.Money
		seedSplits := transactionSplitsFromScheduled(template)
		// A posted occurrence carries no paycheck_section.
		// transactionSplitsFromScheduled forwards the tag so the Edit Series
		// round trip preserves it, but no posting path writes
		// transaction_splits.paycheck_section (see migration 028's rationale),
		// so drop it here to keep the preview, auto-post and `scheduled post`
		// in agreement.
		for _, sp := range seedSplits {
			sp.PaycheckSection = types.NullableString{}
		}
		if loanSplits != nil {
			p.loanShaped = true
			p.loanSeedDate = template.NextDate
			parentAmount = loanSplits.ParentAmount
			seedSplits = loanSplits.Splits
		}

		p.splitDialog = NewSplitDialogFromExisting(parentAmount, categoryOptions, categoryIDs, seedSplits)
		// Match the header dialog's width so the two stacked panels line up.
		p.splitDialog.width = 62

		accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)
		p.splitDialog.SetTransferTargets(accountOptions, accountIDs, template.AccountID)

		if p.loanShaped {
			p.loanSeededRows = p.currentLineSignatures()
		}
		return p
	}

	catIdx := 0
	if template.HasCategory() {
		for i, id := range categoryIDs {
			if id == template.CategoryID.ID {
				catIdx = i
				break
			}
		}
	}

	amountStr := ""
	if template.HasAmount() {
		amountStr = template.Amount.Money.String()
	}

	p.categoryIDs = categoryIDs
	p.headerDialog = buildPreviewHeaderSingle(dateStr, payeeName, memo, amountStr, categoryOptions, catIdx)
	return p
}

// buildPreviewHeaderSingle builds the dialog for a single-line preview.
// dialog.Field layout mirrors the regular transaction edit dialog so the user
// sees a familiar shape.
func buildPreviewHeaderSingle(dateStr, payeeName, memo, amountStr string, categoryOptions []string, catIdx int) *dialog.Dialog {
	d := dialog.NewDialog("Post Scheduled Transaction")
	d.SetWidth(62)

	f := d.AddDateField("Date", dateStr)
	f.Required = true

	d.AddTextField("Payee", payeeName, "Payee name", 0)
	catField := d.AddComboField("Category", categoryOptions, catIdx)
	catField.AddNewLabel = "[+ Add new category…]"

	af := d.AddTextField("Amount", amountStr, "-50.00", 12)
	af.Required = true

	d.AddTextField("Memo", memo, "Optional memo", 0)
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, previewStatusUnclearedIdx)

	d.SetVisible(true)
	return d
}

// buildPreviewHeaderMulti builds the header dialog for a multi-line
// preview. The lines (and their imbalance indicator) live in the
// embedded SplitDialog; the header only carries scalar fields that
// apply to the parent transaction as a whole.
func buildPreviewHeaderMulti(dateStr, payeeName, memo string) *dialog.Dialog {
	d := dialog.NewDialog("Post Scheduled Transaction")
	d.SetWidth(62)

	f := d.AddDateField("Date", dateStr)
	f.Required = true

	d.AddTextField("Payee", payeeName, "Payee name", 0)
	d.AddTextField("Memo", memo, "Optional memo", 0)
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, previewStatusUnclearedIdx)

	// The multi-line preview's single Save/Cancel bar lives on the
	// embedded split panel below this header, so the header carries no
	// buttons of its own.
	d.SetButtons(nil)

	d.SetVisible(true)
	return d
}

// submitSchedulePreviewTransfer posts one occurrence of a transfer schedule
// using the edited date / amount / memo / status, creating a clean linked
// transfer pair. Edits are one-off — the template is untouched.
func (a *App) submitSchedulePreviewTransfer(template *scheduled.Transaction, header *dialog.Dialog) (tea.Model, tea.Cmd) {
	header.ClearErrors()
	fields := header.Fields()
	hasErrors := false

	date, err := parseDateInput(fields[previewXferFieldDate].Value)
	if err != nil {
		fields[previewXferFieldDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	var magnitude types.Money
	if m, perr := parseAmountInput(strings.TrimSpace(fields[previewXferFieldAmount].Value)); perr != nil {
		fields[previewXferFieldAmount].Error = "Invalid amount"
		hasErrors = true
	} else {
		magnitude = m.Abs()
	}

	memo := strings.TrimSpace(fields[previewXferFieldMemo].Value)
	cleared := fields[previewXferFieldStatus].SelectedIndex == 1

	// One-off category for this occurrence. Index 0 is the "(None)" sentinel,
	// which clears the label on both posted legs without touching the template.
	categoryID := types.NullableID{}
	catIdx := fields[previewXferFieldCategory].SelectedIndex
	if catIdx > 0 && catIdx < len(a.schedPreviewDialog.categoryIDs) {
		categoryID = types.NullableID{ID: a.schedPreviewDialog.categoryIDs[catIdx], Valid: true}
	}

	if hasErrors {
		return a, nil
	}

	templateID := template.ID
	a.closeSchedulePreviewDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}
		cmd := undo.NewPostScheduledTransferCommand(
			a.scheduledTxnSvc,
			a.transferSvc,
			templateID,
			date,
			magnitude,
			memo,
			cleared,
			categoryID,
		)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to post scheduled transfer: %w", err)}
		}
		return scheduledPostedMsg{}
	}
}

// buildPreviewHeaderTransfer builds the header for a single-line transfer
// preview. From/To are read-only (re-orienting a transfer is an Edit-Series
// action); Date / Amount / Category / Memo / Status are editable for this one
// occurrence. The Category combo is seeded from the template's category
// (catIdx, 0 = "(None)") and supports inline creation, so the label can be set,
// changed, or cleared for this occurrence alone.
func buildPreviewHeaderTransfer(fromName, toName, dateStr, amountStr, memo string, categoryOptions []string, catIdx int) *dialog.Dialog {
	d := dialog.NewDialog("Post Scheduled Transfer")
	d.SetWidth(62)
	d.SetMessage(fromName + " → " + toName)

	f := d.AddDateField("Date", dateStr)
	f.Required = true

	af := d.AddTextField("Amount", amountStr, "100.00", 12)
	af.Required = true

	catField := d.AddComboField("Category", categoryOptions, catIdx)
	catField.AddNewLabel = "[+ Add new category…]"

	d.AddTextField("Memo", memo, "Optional memo", 0)
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, previewStatusUnclearedIdx)

	d.SetVisible(true)
	return d
}

// IsTransfer reports whether this preview is for a single-line transfer
// schedule.
func (p *SchedulePreviewDialog) IsTransfer() bool {
	return p.template != nil && p.template.IsTransfer()
}

// categoryFieldIndex returns the header field index of the Category combo for
// this preview shape, or -1 when the preview has no scalar category field
// (multi-line, whose lines own their categories). A transfer preview and a
// single-line preview both carry a Category combo but at layout-specific
// indices, so the inline create-category divert resolves the field through
// this helper rather than a hardcoded constant.
func (p *SchedulePreviewDialog) categoryFieldIndex() int {
	switch {
	case p == nil:
		return -1
	case p.IsTransfer():
		return previewXferFieldCategory
	case p.IsMultiLine():
		return -1
	default:
		return previewSingleFieldCat
	}
}

// Template returns the underlying scheduled transaction this preview
// was opened against.
func (p *SchedulePreviewDialog) Template() *scheduled.Transaction {
	return p.template
}

// HeaderDialog returns the dialog carrying the parent-transaction
// fields. Always non-nil.
func (p *SchedulePreviewDialog) HeaderDialog() *dialog.Dialog {
	return p.headerDialog
}

// SplitDialog returns the embedded split editor for multi-line
// previews, or nil for single-line previews.
func (p *SchedulePreviewDialog) SplitDialog() *SplitDialog {
	return p.splitDialog
}

// IsMultiLine reports whether this preview is for a multi-line
// scheduled transaction.
func (p *SchedulePreviewDialog) IsMultiLine() bool {
	return p.splitDialog != nil
}

// IsVisible reports whether the preview dialog should currently render.
func (p *SchedulePreviewDialog) IsVisible() bool {
	return p.headerDialog != nil && p.headerDialog.IsVisible()
}

// FocusOnSplits reports whether key events are currently routed to the
// embedded split editor (multi-line previews only). Always false on
// single-line previews.
func (p *SchedulePreviewDialog) FocusOnSplits() bool {
	return p.splitFocus
}

// IsLoanShaped reports whether this preview is for a loan-shaped schedule —
// one whose interest/principal split is recomputed from the loan's live
// balance rather than copied from the template.
func (p *SchedulePreviewDialog) IsLoanShaped() bool {
	return p.loanShaped
}

// currentLineSignatures snapshots a signature of each split-editor row —
// category selection, transfer mode/target, amount, and memo. Used to detect
// any user line edit (not just an amount change), which permanently freezes
// loan reseeding (the reseed rebuilds the whole editor and would otherwise
// discard a category/memo edit). See the reseed rule in specs/loan-wizard.md.
func (p *SchedulePreviewDialog) currentLineSignatures() []string {
	if p.splitDialog == nil {
		return nil
	}
	out := make([]string, len(p.splitDialog.rows))
	for i := range p.splitDialog.rows {
		r := &p.splitDialog.rows[i]
		out[i] = fmt.Sprintf("%d|%t|%d|%s|%s",
			r.categoryIndex, r.transferMode, r.accountIndex,
			r.amountField.Value, r.memoField.Value)
	}
	return out
}

// userEditedLines reports whether any split-editor row differs from the last
// loan seed — a line amount, category, transfer target, or memo edit, or an
// added/removed row. Any such edit freezes reseeding (user values win).
func (p *SchedulePreviewDialog) userEditedLines() bool {
	cur := p.currentLineSignatures()
	if len(cur) != len(p.loanSeededRows) {
		return true
	}
	for i := range cur {
		if cur[i] != p.loanSeededRows[i] {
			return true
		}
	}
	return false
}

// reseedLoanSplits rebuilds the embedded split editor from a freshly computed
// LoanSplits (a date-change reseed). Only called for loan-shaped previews the
// user has not yet edited. It rebuilds exactly as the constructor did so the
// transfer targets and category resolution stay correct, and updates the
// split editor's total so the imbalance check tracks the new parent amount.
func (p *SchedulePreviewDialog) reseedLoanSplits(ls *scheduled.LoanSplits, date types.Date) {
	p.splitDialog = NewSplitDialogFromExisting(ls.ParentAmount, p.categoryOptions, p.categoryIDs, ls.Splits)
	p.splitDialog.width = 62
	accountOptions, accountIDs := buildSplitTransferAccountOptions(p.accounts)
	p.splitDialog.SetTransferTargets(accountOptions, accountIDs, p.template.AccountID)
	p.loanSeedDate = date
	p.loanSeededRows = p.currentLineSignatures()
}

// schedulePreviewDataMsg carries the dependencies needed to construct a
// SchedulePreviewDialog (template + lookups for payees/accounts/categories).
// It is dispatched asynchronously by loadSchedulePreviewData so the data
// load doesn't block the key handler.
type schedulePreviewDataMsg struct {
	template        *scheduled.Transaction
	accounts        []*account.Account
	payees          []*payee.Payee
	categoryOptions []string
	categoryIDs     []types.ID
	// loanSplits is non-nil only for a loan-shaped schedule whose live-balance
	// recompute succeeded — the preview seeds its lines from it.
	loanSplits *scheduled.LoanSplits
}

// schedulePreviewLoanBlockedMsg is emitted instead of schedulePreviewDataMsg
// when a loan-shaped schedule cannot be previewed because its live-balance
// recompute failed. paidOff is set for an already-paid-off loan (owed ≤ 0):
// opening the preview is a manual post attempt, so the loader has already
// refused-and-completed the schedule (mirroring the CLI post path) and the
// handler shows the payoff toast. Otherwise err carries the typed reason
// (missing interest line, missing APR, negative amortization) surfaced as an
// alert toast; the preview does not open with stale template values.
type schedulePreviewLoanBlockedMsg struct {
	paidOff bool
	err     error
}

// loadSchedulePreviewData loads accounts/payees/categories for the
// currently selected scheduled transaction and emits a
// schedulePreviewDataMsg. Per MS-019 this replaces the legacy
// immediate-post path on Enter — the message handler then constructs
// the preview dialog from the template values.
//
// If the cursor is out of range or the required state is missing the
// returned command is nil and Enter is a no-op.
func (a *App) loadSchedulePreviewData() tea.Cmd {
	if a.scheduled == nil || a.scheduledTable == nil {
		return nil
	}
	cursor := a.scheduledTable.Cursor()
	if cursor < 0 || cursor >= len(a.scheduled.allTxns) {
		return nil
	}
	template := a.scheduled.allTxns[cursor]
	if template == nil {
		return nil
	}

	return func() tea.Msg {
		var accounts []*account.Account
		if a.accountSvc != nil {
			acs, err := a.accountSvc.List(true)
			if err != nil {
				return errMsg{err: err}
			}
			accounts = acs
		}

		var payees []*payee.Payee
		if a.payeeSvc != nil {
			ps, err := a.payeeSvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			payees = ps
		}

		var categories []*category.Category
		if a.categorySvc != nil {
			cs, err := a.categorySvc.List()
			if err != nil {
				return errMsg{err: err}
			}
			categories = cs
		}

		// Offer Value Adjustment when the schedule posts to an asset
		// account, so a value-adjustment line (e.g. depreciation)
		// survives an edit-at-post rather than reverting to (None). A
		// transfer schedule never gets it: Value Adjustment is a system
		// category and a transfer may be labeled with non-system categories
		// only.
		includeVA := accountIsAssetByID(accounts, template.AccountID) && !template.IsTransfer()
		categoryOptions, categoryIDs := buildCategoryOptionsFor(categories, includeVA)

		// Loan-shaped schedules seed the preview from a live-balance recompute
		// (interest/principal split as of the next payment date) instead of the
		// stored month-one snapshot. A recompute failure means the payment
		// cannot be previewed with correct numbers, so the loader blocks the
		// open with a reason rather than seeding stale template values.
		var loanSplits *scheduled.LoanSplits
		if a.scheduledTxnSvc != nil && a.scheduledTxnSvc.IsLoanShaped(template) {
			ls, err := a.scheduledTxnSvc.ComputeLoanSplits(template, template.NextDate)
			switch {
			case err == nil:
				loanSplits = ls
			case errors.Is(err, scheduled.ErrLoanPaidOff):
				// Opening the preview is the TUI's only manual-post door; a
				// paid-off loan is a terminal state, so refuse and complete
				// the schedule on the spot (Post refuses + marks completed via
				// the same path the CLI uses) rather than stranding a
				// never-postable due schedule. Post returns ErrLoanPaidOff on
				// the successful refuse-and-complete; a *different* error (e.g.
				// a closed funding account tripping the pre-check) means the
				// schedule was NOT completed, so surface that instead of a
				// misleading "paid off" toast.
				if _, perr := a.scheduledTxnSvc.Post(template.ID, nil); perr != nil &&
					!errors.Is(perr, scheduled.ErrLoanPaidOff) {
					return schedulePreviewLoanBlockedMsg{err: perr}
				}
				return schedulePreviewLoanBlockedMsg{paidOff: true}
			default:
				return schedulePreviewLoanBlockedMsg{err: err}
			}
		}

		return schedulePreviewDataMsg{
			template:        template,
			accounts:        accounts,
			payees:          payees,
			categoryOptions: categoryOptions,
			categoryIDs:     categoryIDs,
			loanSplits:      loanSplits,
		}
	}
}

// closeSchedulePreviewDialog clears preview dialog state.
func (a *App) closeSchedulePreviewDialog() {
	a.schedPreviewDialog = nil
}

// handleSchedulePreviewMouse routes a left-click to the schedule preview
// dialog. For a single-line preview the header dialog is centered alone
// and handled directly. For a multi-line preview the header is stacked
// over the embedded split panel and centered together, so the click is
// mapped to whichever panel it lands in using the same overlay-centering
// math the view uses to render them.
func (a *App) handleSchedulePreviewMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	p := a.schedPreviewDialog
	if p == nil {
		return a, nil
	}
	header := p.HeaderDialog()
	if header == nil {
		return a, nil
	}

	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return a, nil
	}

	if !p.IsMultiLine() {
		switch header.HandleMouse(msg, a.width, a.height) {
		case dialog.DialogActionSubmit:
			return a.submitSchedulePreviewDialog()
		case dialog.DialogActionCancel:
			a.closeSchedulePreviewDialog()
		case dialog.DialogActionAddNew:
			return a.openCreateCategorySubDialogFromSchedPreview()
		}
		return a, nil
	}

	// Reconstruct the composited overlay exactly as app_view builds it so
	// the click maps to identical coordinates.
	headerStr := header.Render(a.styles)
	splitStr := p.SplitDialog().Render(a.styles)
	overlay := lipgloss.JoinVertical(lipgloss.Left, headerStr, splitStr)
	startCol, startRow := widget.OverlayTopLeft(overlay, a.width, a.height)
	headerLines := strings.Count(headerStr, "\n") + 1

	m := msg.Mouse()
	relY := m.Y - startRow
	if relY < 0 {
		return a, nil
	}

	// Content-local offsets within a panel: border (1) + h-pad (2) on X,
	// border (1) + v-pad (1) on Y.
	if relY < headerLines {
		if header.HandleMouseLocal(m.X-startCol-3, relY-2) == dialog.DialogActionCancel {
			a.closeSchedulePreviewDialog()
			return a, nil
		}
		p.splitFocus = false
		return a, nil
	}

	sd := p.SplitDialog()
	switch sd.HandleMouseLocal(m.X-startCol-3, relY-headerLines-2) {
	case dialog.DialogActionSubmit:
		return a.submitSchedulePreviewDialog()
	case dialog.DialogActionCancel:
		a.closeSchedulePreviewDialog()
		return a, nil
	}
	// A mouse-added split row counts as a user edit — freeze loan reseeding.
	a.freezeLoanSeedIfEdited()
	p.splitFocus = true
	return a, nil
}

// handleSchedulePreviewDialogKey routes keys to the preview dialog. Esc
// cancels (the dialog closes and no transaction is created); Enter on
// the Save button submits — the real transaction is created with any
// user edits, and the schedule advances by one cadence using the
// template's original next_date.
//
// For multi-line previews, keys route to either the header dialog or
// the embedded split editor based on the preview's splitFocus toggle.
// Tab past the header's last focusable element transitions focus into
// the split editor; Shift+Tab from the split editor's first focus
// transitions back to the header. The split editor's MS-013 imbalance
// indicator and disabled-Save behavior apply: a Save attempt with
// imbalanced lines is rejected with the validation error on the
// header.
func (a *App) handleSchedulePreviewDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}

	if a.schedPreviewDialog.IsMultiLine() {
		return a.handleSchedulePreviewMultiLineKey(msg)
	}

	action := a.schedPreviewDialog.HeaderDialog().HandleKey(msg)
	switch action {
	case dialog.DialogActionCancel:
		a.closeSchedulePreviewDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitSchedulePreviewDialog()
	case dialog.DialogActionAddNew:
		return a.openCreateCategorySubDialogFromSchedPreview()
	}
	return a, nil
}

// openCreateCategorySubDialogFromSchedPreview hides the schedule preview
// header dialog and opens the inline create-category sub-dialog seeded with
// the typed query from the Category combo. The preview's field state is
// preserved by keeping the dialog alive (just hidden) for the duration of
// the divert; restoration on cancel and post-create wiring happens through
// the createCatDialog handlers.
func (a *App) openCreateCategorySubDialogFromSchedPreview() (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil {
		return a, nil
	}
	fields := header.Fields()
	catIdx := a.schedPreviewDialog.categoryFieldIndex()
	if catIdx < 0 || catIdx >= len(fields) {
		return a, nil
	}
	catField := fields[catIdx]
	query := catField.Query
	// Consume the trigger and clear the typed query — the create-category
	// dialog now owns it. This way, when we restore the preview, its
	// Category combo doesn't carry stale typed text.
	catField.AddNewTriggered = false
	catField.Query = ""

	// createCatSource must be set before parentsForCreateCatDialog so the
	// helper picks the right source for the parents list.
	a.createCatSource = createCatSourceSchedPreview
	parents := a.parentsForCreateCatDialog()
	parent, name := splitCategoryQuery(query)
	// A transfer's always-positive amount carries no income/expense signal, so
	// its create-category divert defaults to Expense; the single-line preview
	// infers the type from the typed amount.
	defaultType := category.TypeExpense
	if !a.schedPreviewDialog.IsTransfer() && len(fields) > previewSingleFieldAmount {
		defaultType = inferCategoryTypeFromAmount(fields[previewSingleFieldAmount].Value)
	}
	a.createCatDialog = buildCreateCategoryDialog(name, parent, parents, defaultType)
	header.SetVisible(false)
	return a, nil
}

// applyCreatedCategoryToSchedPreview is the per-surface applier called by
// the createCategoryRequestMsg router when the originating surface was the
// (single-line) schedule preview dialog. It reloads the dialog's category
// list with newCat pre-selected on the Category combo, advances focus to
// Amount, re-shows the preview, and clears the create-category sub-dialog.
// Persistence happened in persistCategory; the router passes in the
// freshly-created category.
func (a *App) applyCreatedCategoryToSchedPreview(newCat *category.Category, cats []*category.Category) {
	if a.schedPreviewDialog == nil {
		a.createCatDialog = nil
		return
	}
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil {
		a.createCatDialog = nil
		return
	}
	catIdx := a.schedPreviewDialog.categoryFieldIndex()
	if catIdx < 0 || catIdx >= len(header.Fields()) {
		a.createCatDialog = nil
		return
	}

	// A transfer preview offers only non-system categories; a single-line
	// preview may additionally surface Value Adjustment (asset-account
	// previews), so preserve that build-time decision across the rebuild.
	var options []string
	var ids []types.ID
	if a.schedPreviewDialog.IsTransfer() {
		options, ids = buildCategoryOptions(cats)
	} else {
		includeVA := slices.Contains(header.Fields()[catIdx].Options, category.ValueAdjustmentCategoryName)
		options, ids = buildCategoryOptionsFor(cats, includeVA)
	}
	a.schedPreviewDialog.categoryIDs = ids

	catField := header.Fields()[catIdx]
	catField.Options = options
	newIdx := 0
	for i, id := range ids {
		if id == newCat.ID {
			newIdx = i
			break
		}
	}
	catField.SelectedIndex = newIdx
	// Focus advances to the field after Category so the user can keep typing.
	header.SetFocusIndex(catIdx + 1)
	header.SetVisible(true)
	a.createCatDialog = nil
}

// handleSchedulePreviewMultiLineKey routes keys for a multi-line
// preview to either the header dialog or the embedded split editor.
// The two surfaces have independent focus models — the preview's
// splitFocus toggles between them on Tab from the header's last
// focusable element and Shift+Tab from the split editor's first focus.
func (a *App) handleSchedulePreviewMultiLineKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := a.schedPreviewDialog
	header := p.HeaderDialog()
	splits := p.SplitDialog()

	keyStr := msg.String()

	// Esc always cancels regardless of which surface has focus.
	if keyStr == "esc" {
		a.closeSchedulePreviewDialog()
		return a, nil
	}

	if !p.splitFocus {
		// Tab past the header's last focusable element transitions
		// into the split editor instead of wrapping back to field 0.
		if keyStr == "tab" && header.FocusIndex() == header.FocusableCount()-1 {
			p.splitFocus = true
			splits.focus = splitFocusRows
			splits.rowIndex = 0
			splits.fieldFocus = splitFieldCategory
			return a, nil
		}
		action := header.HandleKey(msg)
		// A Date edit reseeds a loan-shaped preview's split from the balance
		// as of the new date, until the user edits a line amount.
		a.maybeReseedLoanPreview()
		switch action {
		case dialog.DialogActionCancel:
			a.closeSchedulePreviewDialog()
			return a, nil
		case dialog.DialogActionSubmit:
			return a.submitSchedulePreviewDialog()
		}
		return a, nil
	}

	// Shift+Tab from the split editor's first focus transitions back
	// to the header at field 0.
	if keyStr == "shift+tab" && splits.focus == splitFocusRows && splits.rowIndex == 0 && splits.fieldFocus == splitFieldCategory {
		p.splitFocus = false
		header.SetFocusIndex(0)
		return a, nil
	}

	action := splits.HandleKey(msg)
	// A line-amount edit freezes loan reseeding (user values win).
	a.freezeLoanSeedIfEdited()
	switch action {
	case dialog.DialogActionCancel:
		a.closeSchedulePreviewDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitSchedulePreviewDialog()
	}
	return a, nil
}

// maybeReseedLoanPreview recomputes a loan-shaped preview's interest/principal
// split when the Date field has been edited to a new occurrence date — until
// the user edits a line amount, after which user values win and Date edits no
// longer reseed (the reseed rule in specs/loan-wizard.md). A recompute failure
// (e.g. the loan is already paid off at the new date) leaves the current seed
// in place.
func (a *App) maybeReseedLoanPreview() {
	p := a.schedPreviewDialog
	if p == nil || !p.loanShaped || p.loanSeedFrozen || a.scheduledTxnSvc == nil {
		return
	}
	if p.userEditedLines() {
		p.loanSeedFrozen = true
		return
	}
	header := p.HeaderDialog()
	if header == nil {
		return
	}
	fields := header.Fields()
	if len(fields) <= previewFieldDate {
		return
	}
	newDate, err := parseDateInput(fields[previewFieldDate].Value)
	if err != nil || newDate.Equal(p.loanSeedDate) {
		return
	}
	ls, err := a.scheduledTxnSvc.ComputeLoanSplits(p.template, newDate)
	if err != nil {
		return
	}
	p.reseedLoanSplits(ls, newDate)
}

// freezeLoanSeedIfEdited permanently freezes loan reseeding once the user has
// edited any line amount (or added/removed a row) in a loan-shaped preview.
func (a *App) freezeLoanSeedIfEdited() {
	p := a.schedPreviewDialog
	if p == nil || !p.loanShaped || p.loanSeedFrozen {
		return
	}
	if p.userEditedLines() {
		p.loanSeedFrozen = true
	}
}

// submitSchedulePreviewDialog parses the preview dialog fields, builds
// the parent transaction (and, for multi-line previews, the split rows
// frozen at template values until MS-021 lands per-instance line edits)
// with any user edits applied, and dispatches a
// PostScheduledTransactionWithEditsCommand through the undo manager.
// The schedule advances by one cadence using the template's original
// next_date as the basis, so a date edit in the preview never shifts
// the schedule.
//
// Validation errors keep the dialog open and surface a per-field
// error. On success the dialog closes synchronously and a
// scheduledPostedMsg fires after the async save completes.
func (a *App) submitSchedulePreviewDialog() (tea.Model, tea.Cmd) {
	if a.schedPreviewDialog == nil {
		return a, nil
	}
	template := a.schedPreviewDialog.Template()
	header := a.schedPreviewDialog.HeaderDialog()
	if header == nil || template == nil {
		return a, nil
	}

	if a.schedPreviewDialog.IsTransfer() {
		return a.submitSchedulePreviewTransfer(template, header)
	}

	header.ClearErrors()
	hasErrors := false
	fields := header.Fields()

	date, err := parseDateInput(fields[previewFieldDate].Value)
	if err != nil {
		fields[previewFieldDate].Error = "Invalid date (MM/DD/YYYY)"
		hasErrors = true
	}

	payeeName := strings.TrimSpace(fields[previewFieldPayee].Value)

	var (
		amount      types.Money
		categoryID  types.ID
		memo        string
		statusIdx   int
		multiSplits []*transaction.Split
	)

	if a.schedPreviewDialog.IsMultiLine() {
		memo = strings.TrimSpace(fields[previewMultiFieldMemo].Value)
		statusIdx = fields[previewMultiFieldStatus].SelectedIndex

		sd := a.schedPreviewDialog.SplitDialog()

		switch {
		case a.schedPreviewDialog.loanShaped && !a.schedPreviewDialog.loanSeedFrozen &&
			!hasErrors && a.scheduledTxnSvc != nil:
			// A loan-shaped preview the user has not hand-edited is recomputed
			// authoritatively at the posting date here, not trusted from the
			// (possibly stale or reseed-refused) split editor. This closes the
			// date/seed desync: posting a date at which the loan is paid off (or
			// otherwise fails to compute) is refused with a clear error instead
			// of silently posting the old date's interest/principal.
			ls, cerr := a.scheduledTxnSvc.ComputeLoanSplits(template, date)
			if cerr != nil {
				header.SetErrorMsg("Cannot post this loan payment: " + cerr.Error())
				return a, nil
			}
			multiSplits = ls.Splits
			amount = ls.ParentAmount
		default:
			// Generic multi-line, or a loan preview the user edited (frozen):
			// post the split editor's rows verbatim (user values win). The
			// lines are validated to sum to the editor total.
			if sd != nil {
				built, err := sd.buildSplits()
				if err != nil {
					header.SetErrorMsg(err.Error())
					return a, nil
				}
				multiSplits = built
			}
			amount = template.Amount.Money
			if a.schedPreviewDialog.loanShaped && sd != nil {
				amount = sd.totalAmount
			}
		}
	} else {
		catIdx := fields[previewSingleFieldCat].SelectedIndex
		ids := a.schedPreviewDialog.categoryIDs
		if catIdx > 0 && catIdx < len(ids) {
			categoryID = ids[catIdx]
		}

		amountStr := strings.TrimSpace(fields[previewSingleFieldAmount].Value)
		m, err := parseAmountInput(amountStr)
		if err != nil {
			fields[previewSingleFieldAmount].Error = "Invalid amount"
			hasErrors = true
		} else {
			amount = m
		}
		memo = strings.TrimSpace(fields[previewSingleFieldMemo].Value)
		statusIdx = fields[previewSingleFieldStatus].SelectedIndex
	}

	if hasErrors {
		return a, nil
	}

	// Build the parent transaction the user's edits applied. The
	// account always tracks the template — the preview does not allow
	// changing the destination account.
	parent := transaction.NewTransaction(template.AccountID, date, amount)
	if !categoryID.IsNil() {
		parent.SetCategory(categoryID)
	}
	if memo != "" {
		parent.SetMemo(memo)
	}
	if statusIdx == 1 {
		parent.Clear()
	}

	// Resolve payee ID once we're inside the async command so a new
	// name can be created via the payee service. Capture the snapshot
	// of payees by-name so we can short-circuit the common case where
	// the user didn't edit the name.
	payeeLookup := make(map[string]types.ID, len(a.schedPreviewDialog.payees))
	for _, p := range a.schedPreviewDialog.payees {
		if p == nil {
			continue
		}
		payeeLookup[strings.ToLower(p.Name)] = p.ID
	}

	templateID := template.ID
	loanShaped := a.schedPreviewDialog.loanShaped
	a.closeSchedulePreviewDialog()

	return a, func() tea.Msg {
		if a.undoManager == nil {
			return errMsg{err: fmt.Errorf("undo manager not available")}
		}

		if payeeName != "" {
			if id, ok := payeeLookup[strings.ToLower(payeeName)]; ok {
				parent.SetPayee(id)
			} else if a.payeeSvc != nil {
				py, _, err := a.payeeSvc.GetOrCreate(payeeName)
				if err != nil {
					return errMsg{err: fmt.Errorf("failed to resolve payee: %w", err)}
				}
				parent.SetPayee(py.ID)
			}
		}

		cmd := undo.NewPostScheduledTransactionWithEditsCommand(
			a.scheduledTxnSvc,
			a.transactionSvc,
			templateID,
			parent,
			multiSplits,
		)
		if err := a.undoManager.Execute(cmd); err != nil {
			return errMsg{err: fmt.Errorf("failed to post scheduled transaction: %w", err)}
		}

		// PostWithEdits runs finalizeLoanPayoff, which marks a loan-shaped
		// schedule completed once the loan balance reaches ≥ 0 (a normal
		// final payment, or a penny-tweaked edit that overshoots). Re-read
		// the schedule to surface that as a payoff toast; the handler runs
		// on the main loop, so it does the SetToast (never this closure).
		paidOff := false
		if loanShaped && a.scheduledTxnSvc != nil {
			if st, gerr := a.scheduledTxnSvc.GetByID(templateID); gerr == nil && st != nil {
				paidOff = st.IsCompleted()
			}
		}
		return scheduledPostedMsg{loanPaidOff: paidOff}
	}
}
