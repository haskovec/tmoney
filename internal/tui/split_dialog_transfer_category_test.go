package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin Phase 7's split-dialog carry-through: a seeded categorized
// transfer line (e.g. a loan payment's principal line labeled Loan:Principal)
// keeps its category across an edit round-trip even though the dialog offers no
// picker for it. See specs/transfer-categories.md Phase 7.

func TestNewSplitDialogFromExisting_CategorizedTransferLine_CarriesCategory(t *testing.T) {
	checkingID := types.NewID()
	savingsID := types.NewID()
	parentAccountID := types.NewID()
	foodID := types.NewID()
	billsID := types.NewID()

	existing := []*transaction.Split{
		{BaseModel: types.NewBaseModel(), CategoryID: foodID, Amount: types.MustNewMoney("-40.00")},
		{
			BaseModel:         types.NewBaseModel(),
			CategoryID:        billsID, // a categorized transfer line
			TransferAccountID: types.NullableID{ID: checkingID, Valid: true},
			Amount:            types.MustNewMoney("-60.00"),
		},
	}

	sd := NewSplitDialogFromExisting(types.MustNewMoney("-100.00"),
		[]string{"(None)", "Food"}, []types.ID{types.NilID, foodID}, existing)
	sd.SetTransferTargets([]string{"Checking", "Savings"},
		[]types.ID{checkingID, savingsID}, parentAccountID)

	rows := sd.Rows()
	if len(rows) != 2 || !rows[1].transferMode {
		t.Fatalf("expected row 1 to be a transfer row, got %+v", rows)
	}
	if rows[1].seedTransferCategoryID != billsID {
		t.Errorf("seedTransferCategoryID = %v, want Bills %v", rows[1].seedTransferCategoryID, billsID)
	}

	built, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits: %v", err)
	}
	if len(built) != 2 {
		t.Fatalf("built = %d splits, want 2", len(built))
	}
	if !built[1].TransferAccountID.Valid || built[1].TransferAccountID.ID != checkingID {
		t.Errorf("built[1] transfer target = %+v, want %v", built[1].TransferAccountID, checkingID)
	}
	if built[1].CategoryID != billsID {
		t.Errorf("built[1] category = %v, want Bills %v (carry-through failed)", built[1].CategoryID, billsID)
	}
}

// TestBuildSplits_FreshTransferRow_NoCategory pins that a transfer row created
// inside the dialog (not seeded) carries no category.
func TestBuildSplits_FreshTransferRow_NoCategory(t *testing.T) {
	foodID := types.NewID()
	savingsID := types.NewID()
	parentAccountID := types.NewID()

	sd := NewSplitDialog(types.MustNewMoney("-50.00"),
		[]string{"(None)", "Food"}, []types.ID{types.NilID, foodID})
	sd.SetTransferTargets([]string{"Savings"}, []types.ID{savingsID}, parentAccountID)

	// Flip the single row into transfer mode targeting Savings.
	rows := sd.Rows()
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	sd.rows[0].transferMode = true
	sd.rows[0].accountIndex = 0
	sd.rows[0].amountField.Value = "-50.00"

	built, err := sd.buildSplits()
	if err != nil {
		t.Fatalf("buildSplits: %v", err)
	}
	if !built[0].TransferAccountID.Valid {
		t.Fatalf("built[0] should be a transfer line")
	}
	if !built[0].CategoryID.IsNil() {
		t.Errorf("fresh transfer row category = %v, want nil", built[0].CategoryID)
	}
}

// TestScheduledSplitsFromTransaction_CarriesTransferLineCategory pins the
// Edit-Series carry-through: the split editor's transaction.Split rows convert
// back into scheduled.Split children with the transfer-line category preserved,
// and round-trip through transactionSplitsFromScheduled unchanged.
func TestScheduledSplitsFromTransaction_CarriesTransferLineCategory(t *testing.T) {
	foodID := types.NewID()
	billsID := types.NewID()
	savingsID := types.NewID()

	txnSplits := []*transaction.Split{
		transaction.NewSplit(types.NilID, foodID, types.MustNewMoney("-60.00")),
		{
			BaseModel:         types.NewBaseModel(),
			CategoryID:        billsID,
			TransferAccountID: types.NullableID{ID: savingsID, Valid: true},
			Amount:            types.MustNewMoney("-40.00"),
		},
	}

	children := scheduledSplitsFromTransaction(txnSplits)
	if len(children) != 2 {
		t.Fatalf("children = %d, want 2", len(children))
	}
	// Plain categorized line.
	if !children[0].CategoryID.Valid || children[0].CategoryID.ID != foodID || children[0].TransferAccountID.Valid {
		t.Errorf("children[0] = %+v, want Food categorized line", children[0])
	}
	// Categorized transfer line keeps both fields.
	tl := children[1]
	if !tl.TransferAccountID.Valid || tl.TransferAccountID.ID != savingsID {
		t.Errorf("children[1] transfer target = %+v, want Savings", tl.TransferAccountID)
	}
	if !tl.CategoryID.Valid || tl.CategoryID.ID != billsID {
		t.Errorf("children[1] category = %+v, want Bills (carry-through failed)", tl.CategoryID)
	}

	// Round-trip back to transaction.Split (the seed direction) is lossless.
	st := &scheduled.Transaction{Splits: children}
	seeded := transactionSplitsFromScheduled(st)
	if len(seeded) != 2 || seeded[1].CategoryID != billsID || seeded[1].TransferAccountID.ID != savingsID {
		t.Errorf("round-trip lost the categorized transfer line: %+v", seeded[1])
	}
}

// TestScheduledSplitsFromTransaction_UncategorizedTransferLine confirms a
// transfer line with no category produces a categoryless scheduled child.
func TestScheduledSplitsFromTransaction_UncategorizedTransferLine(t *testing.T) {
	savingsID := types.NewID()
	txnSplits := []*transaction.Split{
		{
			BaseModel:         types.NewBaseModel(),
			CategoryID:        types.NilID,
			TransferAccountID: types.NullableID{ID: savingsID, Valid: true},
			Amount:            types.MustNewMoney("-40.00"),
		},
	}
	children := scheduledSplitsFromTransaction(txnSplits)
	if len(children) != 1 {
		t.Fatalf("children = %d, want 1", len(children))
	}
	if children[0].CategoryID.Valid {
		t.Errorf("uncategorized transfer line produced a category: %+v", children[0].CategoryID)
	}
	if !children[0].TransferAccountID.Valid {
		t.Errorf("transfer target dropped: %+v", children[0])
	}
}
