package tui

import (
	"fmt"
	"strconv"
	"time"

	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// The two dialog builders: an empty New Schedule form, and the Edit Series form
// pre-filled from an existing scheduled transaction.

// buildNewScheduledDialog creates a dialog.Dialog for creating a new scheduled transaction.
func buildNewScheduledDialog(accountOptions, categoryOptions []string) *dialog.Dialog {
	d := dialog.NewDialog("New Scheduled Transaction")
	d.SetWidth(62)

	// Account
	d.AddSelectField("Account", accountOptions, 0)

	// Payee
	d.AddTextField("Payee", "", "Payee name", 0)

	// Category — typeahead combo so the [+ Add new category…] action row
	// surfaces for inline creation (CC-003).
	catField := d.AddComboField("Category", categoryOptions, 0)
	catField.AddNewLabel = "[+ Add new category…]"

	// Amount (empty = variable)
	d.AddTextField("Amount", "", "Empty = variable", 12)

	// Memo
	d.AddTextField("Memo", "", "Optional memo", 0)

	// Frequency
	d.AddSelectField("Frequency", buildFrequencyOptions(), frequencyToIndex(scheduled.FrequencyMonthly))

	// Interval
	f := d.AddTextField("Interval", "1", "Every N periods", 5)
	f.Required = true

	// Start date
	today := time.Now().Format("01/02/2006")
	f = d.AddDateField("Start Date", today)
	f.Required = true

	// Duration
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, 0)

	// End date (used when Duration = Until Date) — optional, may be blank.
	d.AddOptionalDateField("End Date", "")

	// Occurrences (used when Duration = Occurrences)
	d.AddTextField("Occurrences", "", "Number of times", 5)

	// Auto-post checkbox
	d.AddCheckboxField("Auto-post", false)

	// Lead days radio (only meaningful when auto-post is checked)
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, 0)

	// Split transaction — toggles the multi-line split editor on Save.
	d.AddCheckboxField("Split transaction", false)

	d.SetVisible(true)
	return d
}

// buildEditScheduledDialog creates a dialog.Dialog for editing an existing scheduled transaction.
func buildEditScheduledDialog(st *scheduled.Transaction, accountOptions []string, accountIDs []types.ID, categoryOptions []string, categoryIDs []types.ID, payeeNames map[types.ID]string) *dialog.Dialog {
	d := dialog.NewDialog("Edit Scheduled Transaction")
	d.SetWidth(62)

	// Account - find the matching index
	acctIdx := 0
	for i, id := range accountIDs {
		if id == st.AccountID {
			acctIdx = i
			break
		}
	}
	d.AddSelectField("Account", accountOptions, acctIdx)

	// Payee
	payeeName := ""
	if st.HasPayee() {
		if name, ok := payeeNames[st.PayeeID.ID]; ok {
			payeeName = name
		}
	}
	d.AddTextField("Payee", payeeName, "Payee name", 0)

	// Category - find the matching index
	catIdx := 0
	if st.HasCategory() {
		for i, id := range categoryIDs {
			if id == st.CategoryID.ID {
				catIdx = i
				break
			}
		}
	}
	editCatField := d.AddComboField("Category", categoryOptions, catIdx)
	editCatField.AddNewLabel = "[+ Add new category…]"

	// Amount
	amountStr := ""
	if st.HasAmount() {
		amountStr = fmt.Sprintf("%.2f", st.Amount.Money.Float64())
	}
	d.AddTextField("Amount", amountStr, "Empty = variable", 12)

	// Memo
	memoStr := ""
	if st.Memo.Valid {
		memoStr = st.Memo.String
	}
	d.AddTextField("Memo", memoStr, "Optional memo", 0)

	// Frequency
	d.AddSelectField("Frequency", buildFrequencyOptions(), frequencyToIndex(st.Frequency))

	// Interval
	f := d.AddTextField("Interval", strconv.Itoa(st.Interval), "Every N periods", 5)
	f.Required = true

	// Start date
	f = d.AddDateField("Start Date", st.StartDate.Time().Format("01/02/2006"))
	f.Required = true

	// Duration
	durationIdx := durationIndefinite
	endDateStr := ""
	occurrencesStr := ""
	if st.EndDate.Valid {
		durationIdx = durationUntilDate
		endDateStr = st.EndDate.Date.Time().Format("01/02/2006")
	} else if st.Occurrences.Valid {
		durationIdx = durationOccurrences
		occurrencesStr = strconv.FormatInt(st.Occurrences.Int64, 10)
	}
	d.AddRadioField("Duration", []string{"Indefinite", "Until Date", "Occurrences"}, durationIdx)

	// End date — optional, may be blank.
	d.AddOptionalDateField("End Date", endDateStr)

	// Occurrences
	d.AddTextField("Occurrences", occurrencesStr, "Number of times", 5)

	// Auto-post checkbox
	d.AddCheckboxField("Auto-post", st.IsAutoPost())

	// Lead days radio
	d.AddRadioField("Lead time", []string{"On the day", "3 days early", "1 week early"}, leadDaysToIndex(st.PostLeadDays))

	// Split transaction — pre-checked when the schedule already carries
	// child split lines (multi-line template).
	d.AddCheckboxField("Split transaction", len(st.Splits) > 0)

	// "Edit as paycheck →" affordance per MS-029: an alternative entry
	// point that closes this dialog and reopens the schedule in the
	// paycheck wizard with values pre-filled. Visible only when the
	// schedule matches the paycheck heuristic.
	if looksLikePaycheck(st) {
		d.SetButtons([]dialog.DialogButton{
			{Label: "Save", Primary: true},
			{Label: "Cancel"},
			{Label: "Edit as paycheck →", Action: dialog.DialogActionAlternate},
		})
	}

	d.SetVisible(true)
	return d
}
