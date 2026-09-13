package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
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
//
// It delegates to the header dialog, and that aliasing is load-bearing: the
// create-category divert hides the header, which must take the whole preview
// off screen. Nil-safe so the modal registry can walk an unbuilt surface — see
// the note on dialog.Dialog.IsVisible.
func (p *SchedulePreviewDialog) IsVisible() bool {
	return p != nil && p.headerDialog.IsVisible()
}

// Render draws the preview. For a multi-line preview the header dialog and the
// embedded split editor stack vertically; Tab transitions focus between the two
// surfaces (header → split editor, Shift+Tab from split editor → header).
//
// This was inline in renderLayout. It lives here so the preview is one modal
// surface to the registry rather than the only entry needing bespoke paint
// code. handleSchedulePreviewMouse maps a click against this same composite,
// so hit-testing cannot drift from paint.
func (p *SchedulePreviewDialog) Render(styles widget.Styles) string {
	if p == nil {
		return ""
	}
	out := p.headerDialog.Render(styles)
	if p.IsMultiLine() {
		if sd := p.SplitDialog(); sd != nil {
			out = lipgloss.JoinVertical(lipgloss.Left, out, sd.Render(styles))
		}
	}
	return out
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
