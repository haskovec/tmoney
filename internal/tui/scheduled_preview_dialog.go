package tui

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// Field indices for the SchedulePreviewDialog's header dialog.
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
)

// SchedulePreviewDialog is the Quicken-style "post one occurrence" dialog
// that opens when the user presses Enter on a due scheduled transaction.
// It is pre-filled with the schedule's template values; edits made here
// flow into the real transaction created at save time but do not modify
// the template (one-off semantics — see
// specs/multiline-splits-and-paycheck.md, Post-Time Preview Dialog).
//
// The dialog has two shapes:
//   - For a single-line schedule it owns one Dialog with date / payee /
//     category / amount / memo / status fields, mirroring the regular
//     transaction edit dialog.
//   - For a multi-line schedule it owns a header Dialog (date / payee /
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
	headerDialog *Dialog

	// splitDialog is non-nil for multi-line schedules and edits the
	// line items.
	splitDialog *SplitDialog
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
func NewSchedulePreviewDialog(
	template *scheduled.Transaction,
	accounts []*account.Account,
	payees []*payee.Payee,
	categoryOptions []string,
	categoryIDs []types.ID,
) *SchedulePreviewDialog {
	if template == nil {
		return nil
	}

	p := &SchedulePreviewDialog{template: template}

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

	if len(template.Splits) > 0 {
		p.headerDialog = buildPreviewHeaderMulti(dateStr, payeeName, memo)

		parentAmount := template.Amount.Money
		seedSplits := transactionSplitsFromScheduled(template)
		p.splitDialog = NewSplitDialogFromExisting(parentAmount, categoryOptions, categoryIDs, seedSplits)

		accountOptions, accountIDs := buildSplitTransferAccountOptions(accounts)
		p.splitDialog.SetTransferTargets(accountOptions, accountIDs, template.AccountID)
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

	p.headerDialog = buildPreviewHeaderSingle(dateStr, payeeName, memo, amountStr, categoryOptions, catIdx)
	return p
}

// buildPreviewHeaderSingle builds the dialog for a single-line preview.
// Field layout mirrors the regular transaction edit dialog so the user
// sees a familiar shape.
func buildPreviewHeaderSingle(dateStr, payeeName, memo, amountStr string, categoryOptions []string, catIdx int) *Dialog {
	d := NewDialog("Post Scheduled Transaction")
	d.SetWidth(62)

	f := d.AddDateField("Date", dateStr)
	f.Required = true

	d.AddTextField("Payee", payeeName, "Payee name", 0)
	d.AddComboField("Category", categoryOptions, catIdx)

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
func buildPreviewHeaderMulti(dateStr, payeeName, memo string) *Dialog {
	d := NewDialog("Post Scheduled Transaction")
	d.SetWidth(62)

	f := d.AddDateField("Date", dateStr)
	f.Required = true

	d.AddTextField("Payee", payeeName, "Payee name", 0)
	d.AddTextField("Memo", memo, "Optional memo", 0)
	d.AddRadioField("Status", []string{"Uncleared", "Cleared"}, previewStatusUnclearedIdx)

	d.SetVisible(true)
	return d
}

// Template returns the underlying scheduled transaction this preview
// was opened against.
func (p *SchedulePreviewDialog) Template() *scheduled.Transaction {
	return p.template
}

// HeaderDialog returns the dialog carrying the parent-transaction
// fields. Always non-nil.
func (p *SchedulePreviewDialog) HeaderDialog() *Dialog {
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
