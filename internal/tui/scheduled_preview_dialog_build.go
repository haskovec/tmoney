package tui

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// Construction: the preview dialog for a due schedule, and the three header
// forms it embeds for single-line, multi-line and transfer schedules.

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
