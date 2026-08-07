package transaction

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin Phase 7: a categorized transfer line (a "categorized
// transfer", e.g. a loan payment's principal line labeled Loan:Principal)
// mirrors its category onto the paired counter-transaction and preserves it
// through create / edit / move / ReplaceSplits, without disturbing the
// delete/void cascades. See specs/transfer-categories.md Phase 7.

// makeCategorizedTransferLineSplit builds a transfer-line split that also
// carries a category, shaped as the TUI split dialog emits it (TransferID
// unset — the service mints or preserves it).
func makeCategorizedTransferLineSplit(parentID, targetAcctID, categoryID types.ID, amount types.Money) *Split {
	s := makeTransferLineSplit(parentID, targetAcctID, amount)
	s.CategoryID = categoryID
	return s
}

type catXferFixture struct {
	svc           *Service
	accountRepo   *account.Repository
	categoryRepo  *category.Repository
	parentID      types.ID
	savingsID     types.ID
	foodID        types.ID
	billsID       types.ID
	rentID        types.ID
	transferID    types.ID
	counterpartID types.ID
}

// setupCatBankXfer builds a -100 split parent in Checking: a -60 Food line plus
// a -40 transfer line into Savings labeled Bills, whose regular-table
// counterpart mirrors the Bills category.
func setupCatBankXfer(t *testing.T) catXferFixture {
	t.Helper()
	svc, accountRepo, _, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	mkCat := func(name string) types.ID {
		c := category.NewCategory(name, category.TypeExpense)
		if err := categoryRepo.Create(c); err != nil {
			t.Fatalf("create category %s: %v", name, err)
		}
		return c.ID
	}
	foodID := mkCat("Food")
	billsID := mkCat("Bills")
	rentID := mkCat("Rent")

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	catLine := NewSplit(parent.ID, foodID, types.MustNewMoney("-60.00"))
	xfer := makeCategorizedTransferLineSplit(parent.ID, savings.ID, billsID, types.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(parent, []*Split{catLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	line := transferLineOf(t, svc, parent.ID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("expected a minted transfer_id on the transfer line")
	}
	if line.CategoryID != billsID {
		t.Fatalf("transfer line lost its category: got %v, want %v", line.CategoryID, billsID)
	}
	cp, err := svc.findPairedByTransferID(line.TransferID.ID)
	if err != nil || cp == nil {
		t.Fatalf("expected a bank counterpart, err=%v cp=%v", err, cp)
	}
	if !cp.HasCategory() || cp.CategoryID.ID != billsID {
		t.Fatalf("counterpart category = %v (has=%v), want Bills %v", cp.CategoryID, cp.HasCategory(), billsID)
	}
	return catXferFixture{
		svc:           svc,
		accountRepo:   accountRepo,
		categoryRepo:  categoryRepo,
		parentID:      parent.ID,
		savingsID:     savings.ID,
		foodID:        foodID,
		billsID:       billsID,
		rentID:        rentID,
		transferID:    line.TransferID.ID,
		counterpartID: cp.ID,
	}
}

// TestCreateWithSplits_CategorizedTransferLine_MirrorsToBankCounterpart is the
// core create-time mirror (asserted inside setupCatBankXfer); this keeps a
// named regression around it.
func TestCreateWithSplits_CategorizedTransferLine_MirrorsToBankCounterpart(t *testing.T) {
	setupCatBankXfer(t)
}

func TestUpdateSplit_TransferLineCategoryChange_MirrorsToCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)

	line := transferLineOf(t, f.svc, f.parentID)
	line.CategoryID = f.rentID // Bills -> Rent, amount unchanged
	if err := f.svc.UpdateSplit(line); err != nil {
		t.Fatalf("UpdateSplit: %v", err)
	}

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost: err=%v cp=%v", err, cp)
	}
	if cp.ID != f.counterpartID {
		t.Errorf("counterpart identity churned on category edit")
	}
	if !cp.HasCategory() || cp.CategoryID.ID != f.rentID {
		t.Errorf("counterpart category = %v, want Rent %v", cp.CategoryID, f.rentID)
	}
}

func TestUpdateSplit_TransferLineCategoryCleared_MirrorsToCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)

	line := transferLineOf(t, f.svc, f.parentID)
	line.CategoryID = types.NilID // clear the category
	if err := f.svc.UpdateSplit(line); err != nil {
		t.Fatalf("UpdateSplit: %v", err)
	}

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost: err=%v cp=%v", err, cp)
	}
	if cp.HasCategory() {
		t.Errorf("counterpart category should be cleared, got %v", cp.CategoryID)
	}
}

func TestMoveTransferLine_CarriesCategoryToNewCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)
	vacation := createTestAccount(t, f.accountRepo, "Vacation")

	line := transferLineOf(t, f.svc, f.parentID)
	line.TransferAccountID = types.NullableID{ID: vacation.ID, Valid: true}
	if err := f.svc.UpdateSplit(line); err != nil {
		t.Fatalf("UpdateSplit (move target): %v", err)
	}

	// Old counterpart in Savings gone.
	if oldCP, _ := f.svc.findPairedByTransferID(f.transferID); oldCP != nil {
		t.Errorf("old counterpart should be gone: %s", oldCP.ID)
	}
	// New counterpart in Vacation carries the category.
	moved := transferLineOf(t, f.svc, f.parentID)
	if moved == nil || !moved.TransferID.Valid {
		t.Fatalf("moved transfer line missing transfer_id")
	}
	newCP, err := f.svc.findPairedByTransferID(moved.TransferID.ID)
	if err != nil || newCP == nil {
		t.Fatalf("no counterpart in new target: err=%v", err)
	}
	if newCP.AccountID != vacation.ID {
		t.Errorf("counterpart account = %s, want vacation %s", newCP.AccountID, vacation.ID)
	}
	if !newCP.HasCategory() || newCP.CategoryID.ID != f.billsID {
		t.Errorf("moved counterpart category = %v, want Bills %v", newCP.CategoryID, f.billsID)
	}
}

func TestReplaceSplits_CategorizedTransferLine_RetainedKeepsCategory(t *testing.T) {
	f := setupCatBankXfer(t)

	// TUI-shaped rewrite: identical transfer line (transfer_id unset, same
	// amount + category). Counterpart must be untouched and still categorized.
	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeCategorizedTransferLineSplit(f.parentID, f.savingsID, f.billsID, types.MustNewMoney("-40.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || line.TransferID.ID != f.transferID {
		t.Errorf("transfer_id churned: got %v, want %s", line.TransferID, f.transferID)
	}
	if line.CategoryID != f.billsID {
		t.Errorf("split line category churned: got %v, want Bills", line.CategoryID)
	}
	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost: err=%v", err)
	}
	if cp.ID != f.counterpartID {
		t.Errorf("counterpart identity churned")
	}
	if !cp.HasCategory() || cp.CategoryID.ID != f.billsID {
		t.Errorf("counterpart category = %v, want Bills", cp.CategoryID)
	}
}

func TestReplaceSplits_TransferLineCategoryOnlyChange_MirrorsCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)

	// Amount unchanged, category Bills -> Rent. The counterpart is retained
	// (identity preserved) but its category must be re-synced.
	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeCategorizedTransferLineSplit(f.parentID, f.savingsID, f.rentID, types.MustNewMoney("-40.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost: err=%v", err)
	}
	if cp.ID != f.counterpartID {
		t.Errorf("counterpart identity churned on category-only change")
	}
	if !cp.HasCategory() || cp.CategoryID.ID != f.rentID {
		t.Errorf("counterpart category = %v, want Rent (category-only re-sync failed)", cp.CategoryID)
	}
}

func TestReplaceSplits_AddedCategorizedTransferLine_MintsCategorizedCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)

	// Drop the transfer line entirely, then add a fresh categorized one back.
	if err := f.svc.ReplaceSplits(f.parentID, []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-100.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (drop): %v", err)
	}
	if err := f.svc.ReplaceSplits(f.parentID, []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeCategorizedTransferLineSplit(f.parentID, f.savingsID, f.rentID, types.MustNewMoney("-40.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (add): %v", err)
	}

	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("added transfer line has no minted transfer_id")
	}
	cp, err := f.svc.findPairedByTransferID(line.TransferID.ID)
	if err != nil || cp == nil {
		t.Fatalf("no counterpart minted for added transfer line: err=%v", err)
	}
	if !cp.HasCategory() || cp.CategoryID.ID != f.rentID {
		t.Errorf("minted counterpart category = %v, want Rent", cp.CategoryID)
	}
}

func TestReplaceSplits_ReconciledCounterpartBlocksCategoryOnlyChange(t *testing.T) {
	f := setupCatBankXfer(t)

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart missing: %v", err)
	}
	cp.SetStatus(StatusReconciled)
	if err := f.svc.txnRepo.Update(cp); err != nil {
		t.Fatalf("reconcile counterpart: %v", err)
	}

	// Category-only change (amount unchanged) must be blocked by the reconciled
	// counterpart, with no partial mutation.
	err = f.svc.ReplaceSplits(f.parentID, []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeCategorizedTransferLineSplit(f.parentID, f.savingsID, f.rentID, types.MustNewMoney("-40.00")),
	})
	var recErr *IsReconciledError
	if !errors.As(err, &recErr) {
		t.Fatalf("expected IsReconciledError, got %v", err)
	}
	// Splits and counterpart unchanged.
	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || line.CategoryID != f.billsID {
		t.Errorf("split line mutated despite block: %+v", line)
	}
	cpAfter, _ := f.svc.findPairedByTransferID(f.transferID)
	if cpAfter == nil || cpAfter.CategoryID.ID != f.billsID || cpAfter.Status != StatusReconciled {
		t.Errorf("counterpart mutated despite block: %+v", cpAfter)
	}
}

func TestDeleteSplit_CategorizedTransferLine_DeletesCounterpart(t *testing.T) {
	f := setupCatBankXfer(t)

	line := transferLineOf(t, f.svc, f.parentID)
	if err := f.svc.DeleteSplit(line.ID); err != nil {
		t.Fatalf("DeleteSplit: %v", err)
	}
	if cp, _ := f.svc.findPairedByTransferID(f.transferID); cp != nil {
		t.Errorf("counterpart should be deleted, still present: %s", cp.ID)
	}
	if n := accountTxnCount(t, f.svc, f.savingsID); n != 0 {
		t.Errorf("savings has %d transactions, want 0", n)
	}
}

// TestCreateWithSplits_CategorizedInvestmentTransferLine_LineHoldsCategory
// verifies an investment-target categorized transfer line keeps its category
// on the split line while the investment adapter counterpart (which has no
// category column) carries none.
func TestCreateWithSplits_CategorizedInvestmentTransferLine_LineHoldsCategory(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("create salary: %v", err)
	}
	contrib := category.NewCategory("Retirement", category.TypeExpense)
	if err := categoryRepo.Create(contrib); err != nil {
		t.Fatalf("create retirement: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	xfer := makeCategorizedTransferLineSplit(parent.ID, ira.ID, contrib.ID, types.MustNewMoney("-200.00"))
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	line := transferLineOf(t, svc, parent.ID)
	if line == nil || line.CategoryID != contrib.ID {
		t.Fatalf("investment transfer line lost its category: %+v", line)
	}
	// Adapter holds exactly one counterpart and it stores no category (the fake
	// row has no category field — matching the investment_transactions schema).
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment counterpart, got %d", len(adapter.rows))
	}
	// The regular table holds no counterpart for this transfer_id.
	if cp, _ := svc.findPairedByTransferID(line.TransferID.ID); cp != nil {
		t.Errorf("investment transfer should have no regular-table counterpart, got %s", cp.ID)
	}
}

type catInvXferFixture struct {
	svc          *Service
	adapter      *fakeInvCounterpart
	parentID     types.ID
	iraID        types.ID
	salaryID     types.ID
	retirementID types.ID
	altCatID     types.ID
	transferID   types.ID
}

// setupCatInvXfer builds an +800 split parent in Checking: a +1000 Salary line
// plus a -200 transfer line into an investment IRA labeled Retirement. The
// counterpart lives on the investment adapter, which stores no category.
func setupCatInvXfer(t *testing.T) catInvXferFixture {
	t.Helper()
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("create salary: %v", err)
	}
	retirement := category.NewCategory("Retirement", category.TypeExpense)
	if err := categoryRepo.Create(retirement); err != nil {
		t.Fatalf("create retirement: %v", err)
	}
	altCat := category.NewCategory("Retirement Roth", category.TypeExpense)
	if err := categoryRepo.Create(altCat); err != nil {
		t.Fatalf("create alt: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	xfer := makeCategorizedTransferLineSplit(parent.ID, ira.ID, retirement.ID, types.MustNewMoney("-200.00"))
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}
	line := transferLineOf(t, svc, parent.ID)
	if line == nil || !line.TransferID.Valid || line.CategoryID != retirement.ID {
		t.Fatalf("expected a categorized investment transfer line, got %+v", line)
	}
	if adapter.findRowByTransferID(line.TransferID.ID) == nil {
		t.Fatalf("expected an investment-side counterpart")
	}
	return catInvXferFixture{
		svc:          svc,
		adapter:      adapter,
		parentID:     parent.ID,
		iraID:        ira.ID,
		salaryID:     salary.ID,
		retirementID: retirement.ID,
		altCatID:     altCat.ID,
		transferID:   line.TransferID.ID,
	}
}

// TestReplaceSplits_CategoryOnlyChange_InvestmentReconciledCounterpart_Allowed
// pins the review fix: a category-only change on an investment-target transfer
// line must NOT be blocked by a reconciled investment counterpart, because that
// counterpart stores no category and is never written by the change.
func TestReplaceSplits_CategoryOnlyChange_InvestmentReconciledCounterpart_Allowed(t *testing.T) {
	f := setupCatInvXfer(t)
	row := f.adapter.findRowByTransferID(f.transferID)
	row.reconciled = true

	newSplits := []*Split{
		NewSplit(f.parentID, f.salaryID, types.MustNewMoney("1000.00")),
		makeCategorizedTransferLineSplit(f.parentID, f.iraID, f.altCatID, types.MustNewMoney("-200.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("category-only change on investment target should not be blocked by a reconciled counterpart: %v", err)
	}

	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || line.CategoryID != f.altCatID {
		t.Errorf("split line category = %+v, want alt", line)
	}
	// Investment counterpart untouched and still reconciled.
	after := f.adapter.findRowByTransferID(f.transferID)
	if after == nil || !after.reconciled || !after.amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("investment counterpart should be untouched: %+v", after)
	}
}

// TestUpdateSplit_CategoryOnlyChange_InvestmentReconciledCounterpart_Allowed is
// the UpdateSplit twin of the above.
func TestUpdateSplit_CategoryOnlyChange_InvestmentReconciledCounterpart_Allowed(t *testing.T) {
	f := setupCatInvXfer(t)
	row := f.adapter.findRowByTransferID(f.transferID)
	row.reconciled = true

	line := transferLineOf(t, f.svc, f.parentID)
	line.CategoryID = f.altCatID // category-only, amount unchanged
	if err := f.svc.UpdateSplit(line); err != nil {
		t.Fatalf("category-only UpdateSplit should not be blocked by a reconciled investment counterpart: %v", err)
	}
	got := transferLineOf(t, f.svc, f.parentID)
	if got == nil || got.CategoryID != f.altCatID {
		t.Errorf("split line category = %+v, want alt", got)
	}
	if !row.reconciled || !row.amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("investment counterpart should be untouched: %+v", row)
	}
}

// TestUpdateSplit_CategoryChange_ReconciledRegularCounterpart_NoPartialWrite
// pins the review fix that UpdateSplit pre-flights the counterpart before
// persisting the split row, so a reconciled regular counterpart blocks the edit
// with no partial mutation of the split line.
func TestUpdateSplit_CategoryChange_ReconciledRegularCounterpart_NoPartialWrite(t *testing.T) {
	f := setupCatBankXfer(t)

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart missing: %v", err)
	}
	cp.SetStatus(StatusReconciled)
	if err := f.svc.txnRepo.Update(cp); err != nil {
		t.Fatalf("reconcile counterpart: %v", err)
	}

	line := transferLineOf(t, f.svc, f.parentID)
	line.CategoryID = f.rentID // Bills -> Rent
	err = f.svc.UpdateSplit(line)
	var recErr *IsReconciledError
	if !errors.As(err, &recErr) {
		t.Fatalf("expected IsReconciledError, got %v", err)
	}
	// The split row must be unchanged (pre-flight ran before splitRepo.Update).
	got := transferLineOf(t, f.svc, f.parentID)
	if got == nil || got.CategoryID != f.billsID {
		t.Errorf("split line mutated despite block (partial write): got %+v, want Bills", got)
	}
}
