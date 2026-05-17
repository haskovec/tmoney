package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// TestSchedulePreviewDialog_OpensWithTemplateValues covers MS-018: the
// preview dialog scaffolding must open pre-filled with the template's
// values for both single-line and multi-line scheduled transactions.
// Edits to the dialog will eventually flow into the real transaction
// (MS-020); for now the scaffolding only needs to seed the fields
// correctly.
func TestSchedulePreviewDialog_OpensWithTemplateValues(t *testing.T) {
	t.Run("single-line preview seeds scalar fields from the template", func(t *testing.T) {
		accountID := types.NewID()
		payeeID := types.NewID()
		categoryID := types.NewID()
		nextDate := types.NewDate(2026, 4, 15)
		amount, _ := types.NewMoney("-1500.00")

		template := scheduled.NewTransaction(accountID, scheduled.FrequencyMonthly, nextDate)
		template.NextDate = nextDate
		template.Amount = types.NullableMoney{Money: amount, Valid: true}
		template.SetPayee(payeeID)
		template.SetCategory(categoryID)
		template.SetMemo("Monthly rent")

		payees := []*payee.Payee{
			{BaseModel: types.BaseModel{ID: payeeID}, Name: "Landlord"},
		}
		categoryOptions := []string{"(None)", "Other", "Rent"}
		categoryIDs := []types.ID{types.NilID, types.NewID(), categoryID}
		accounts := []*account.Account{
			{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
		}

		preview := NewSchedulePreviewDialog(template, accounts, payees, categoryOptions, categoryIDs)
		if preview == nil {
			t.Fatal("NewSchedulePreviewDialog returned nil for a valid template")
		}
		if preview.IsMultiLine() {
			t.Fatal("preview should be single-line when template has no splits")
		}
		if preview.SplitDialog() != nil {
			t.Fatal("single-line preview should not embed a split dialog")
		}
		if !preview.IsVisible() {
			t.Error("preview dialog should be visible after construction")
		}
		if preview.Template() != template {
			t.Error("Template accessor should return the source template")
		}

		hd := preview.HeaderDialog()
		if hd == nil {
			t.Fatal("HeaderDialog should be non-nil")
		}
		fields := hd.Fields()
		// Single-line preview should expose six fields:
		// Date, Payee, Category, Amount, Memo, Status.
		if len(fields) != 6 {
			t.Fatalf("expected 6 fields on single-line preview, got %d", len(fields))
		}

		// Date pre-fills with next_date in MM/DD/YYYY form.
		if got, want := fields[previewFieldDate].Value, "04/15/2026"; got != want {
			t.Errorf("date field = %q, want %q", got, want)
		}
		// Payee resolves from the payees slice.
		if got, want := fields[previewFieldPayee].Value, "Landlord"; got != want {
			t.Errorf("payee field = %q, want %q", got, want)
		}
		// Category combo points at the template's category.
		if got, want := fields[previewSingleFieldCat].SelectedIndex, 2; got != want {
			t.Errorf("category selectedIndex = %d, want %d", got, want)
		}
		// Amount renders the template amount.
		if got, want := fields[previewSingleFieldAmount].Value, amount.String(); got != want {
			t.Errorf("amount field = %q, want %q", got, want)
		}
		// Memo pre-fills from the template.
		if got, want := fields[previewSingleFieldMemo].Value, "Monthly rent"; got != want {
			t.Errorf("memo field = %q, want %q", got, want)
		}
		// Status defaults to Uncleared (a fresh post is always uncleared
		// until the user toggles it).
		if got, want := fields[previewSingleFieldStatus].SelectedIndex, previewStatusUnclearedIdx; got != want {
			t.Errorf("status selectedIndex = %d, want %d", got, want)
		}
	})

	t.Run("multi-line preview seeds header and split editor from the template", func(t *testing.T) {
		accountID := types.NewID()
		retirementAccountID := types.NewID()
		payeeID := types.NewID()
		incomeCatID := types.NewID()
		taxCatID := types.NewID()
		nextDate := types.NewDate(2026, 1, 23)
		netAmount, _ := types.NewMoney("4090.00")
		grossAmount, _ := types.NewMoney("5000.00")
		taxAmount, _ := types.NewMoney("-410.00")
		retireAmount, _ := types.NewMoney("-500.00")

		template := scheduled.NewTransaction(accountID, scheduled.FrequencyBiweekly, nextDate)
		template.NextDate = nextDate
		template.Amount = types.NullableMoney{Money: netAmount, Valid: true}
		template.SetPayee(payeeID)
		template.SetMemo("Paycheck")

		template.Splits = scheduled.SplitCollection{
			scheduled.NewCategorizedSplit(template.ID, incomeCatID, grossAmount),
			scheduled.NewCategorizedSplit(template.ID, taxCatID, taxAmount),
			scheduled.NewTransferSplit(template.ID, retirementAccountID, retireAmount),
		}

		payees := []*payee.Payee{
			{BaseModel: types.BaseModel{ID: payeeID}, Name: "Employer Inc"},
		}
		categoryOptions := []string{"(None)", "Salary", "Federal Tax"}
		categoryIDs := []types.ID{types.NilID, incomeCatID, taxCatID}
		accounts := []*account.Account{
			{BaseModel: types.BaseModel{ID: accountID}, Name: "Checking", Active: true, Type: account.TypeChecking},
			{BaseModel: types.BaseModel{ID: retirementAccountID}, Name: "401k", Active: true, Type: account.TypeInvestment},
		}

		preview := NewSchedulePreviewDialog(template, accounts, payees, categoryOptions, categoryIDs)
		if preview == nil {
			t.Fatal("NewSchedulePreviewDialog returned nil for a multi-line template")
		}
		if !preview.IsMultiLine() {
			t.Fatal("preview should report multi-line for a template with splits")
		}
		if !preview.IsVisible() {
			t.Error("preview dialog should be visible after construction")
		}

		hd := preview.HeaderDialog()
		if hd == nil {
			t.Fatal("HeaderDialog should be non-nil")
		}
		fields := hd.Fields()
		// Multi-line preview header carries date, payee, memo, status —
		// no scalar category or amount (those live in the split editor).
		if len(fields) != 4 {
			t.Fatalf("expected 4 header fields on multi-line preview, got %d", len(fields))
		}
		if got, want := fields[previewFieldDate].Value, "01/23/2026"; got != want {
			t.Errorf("date field = %q, want %q", got, want)
		}
		if got, want := fields[previewFieldPayee].Value, "Employer Inc"; got != want {
			t.Errorf("payee field = %q, want %q", got, want)
		}
		if got, want := fields[previewMultiFieldMemo].Value, "Paycheck"; got != want {
			t.Errorf("memo field = %q, want %q", got, want)
		}
		if got, want := fields[previewMultiFieldStatus].SelectedIndex, previewStatusUnclearedIdx; got != want {
			t.Errorf("status selectedIndex = %d, want %d", got, want)
		}

		sd := preview.SplitDialog()
		if sd == nil {
			t.Fatal("multi-line preview should embed a SplitDialog")
		}
		if !sd.totalAmount.Equal(netAmount) {
			t.Errorf("split totalAmount = %s, want %s",
				sd.totalAmount.String(), netAmount.String())
		}
		// Imbalance starts at zero because the template's lines net to
		// the parent's amount.
		if !sd.IsSaveEnabled() {
			t.Errorf("preview should open balanced (template guarantees signed sum); remaining=%s",
				sd.remaining().String())
		}

		rows := sd.Rows()
		if len(rows) != 3 {
			t.Fatalf("expected 3 split rows, got %d", len(rows))
		}

		// Row 0 — categorized income line.
		if got, want := rows[0].amountField.Value, grossAmount.String(); got != want {
			t.Errorf("row[0] amount = %q, want %q", got, want)
		}
		if rows[0].transferMode {
			t.Errorf("row[0] should be in category mode (income line)")
		}
		if got, want := rows[0].categoryIndex, 1; got != want {
			t.Errorf("row[0] categoryIndex = %d, want %d", got, want)
		}

		// Row 1 — categorized tax line (negative amount).
		if got, want := rows[1].amountField.Value, taxAmount.String(); got != want {
			t.Errorf("row[1] amount = %q, want %q", got, want)
		}
		if got, want := rows[1].categoryIndex, 2; got != want {
			t.Errorf("row[1] categoryIndex = %d, want %d", got, want)
		}

		// Row 2 — transfer line to the retirement account. The template
		// child carried TransferAccountID; the split dialog seeded the
		// row in category mode at index 0 (the (None) slot) because
		// NewSplitDialogFromExisting doesn't currently introspect
		// transfer-shape children — but it must at minimum preserve the
		// amount so the imbalance indicator works.
		if got, want := rows[2].amountField.Value, retireAmount.String(); got != want {
			t.Errorf("row[2] amount = %q, want %q", got, want)
		}

		// SetTransferTargets should have filtered out the parent
		// account (the schedule's own account) so users can't
		// self-transfer.
		for _, id := range sd.transferAccountIDs {
			if id == accountID {
				t.Errorf("transfer target picker still contains the schedule's own account")
			}
		}
		if len(sd.transferAccountIDs) == 0 {
			t.Error("transfer target picker should contain at least one account (the 401k)")
		}
	})

	t.Run("nil template yields nil preview", func(t *testing.T) {
		if got := NewSchedulePreviewDialog(nil, nil, nil, nil, nil); got != nil {
			t.Errorf("NewSchedulePreviewDialog(nil, …) = %v, want nil", got)
		}
	})
}
