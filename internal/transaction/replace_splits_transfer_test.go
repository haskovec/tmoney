package transaction

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the Phase-2 fix: ReplaceSplits must keep a transfer line's
// paired counter-transaction consistent when a split set is rewritten, rather
// than blindly dropping every row (which tripped the transfer_id pairing CHECK
// and orphaned the counterpart). See specs/transfer-categories.md and the
// implementation plan's Phase 2.

// makeTransferLineSplit builds a transfer-line split shaped exactly as the TUI
// split dialog's buildSplits emits it: TransferAccountID set, TransferID unset
// (the service is expected to mint or preserve it).
func makeTransferLineSplit(parentID, targetAcctID types.ID, amount types.Money) *Split {
	return &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parentID,
		CategoryID:        types.NilID,
		Amount:            amount,
		TransferAccountID: types.NullableID{ID: targetAcctID, Valid: true},
	}
}

// transferLineOf returns the single transfer-line split on a transaction (nil
// if none).
func transferLineOf(t *testing.T, svc *Service, parentID types.ID) *Split {
	t.Helper()
	splits, err := svc.GetSplits(parentID)
	if err != nil {
		t.Fatalf("GetSplits: %v", err)
	}
	var found *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			if found != nil {
				t.Fatalf("expected one transfer line, found multiple")
			}
			found = s
		}
	}
	return found
}

// accountTxnCount reports how many transactions live in an account — used to
// prove no orphaned counterparts survive.
func accountTxnCount(t *testing.T, svc *Service, acctID types.ID) int {
	t.Helper()
	txns, err := svc.ListByAccount(acctID)
	if err != nil {
		t.Fatalf("ListByAccount: %v", err)
	}
	return len(txns)
}

type bankXferFixture struct {
	svc           *Service
	accountRepo   *account.Repository
	categoryRepo  *category.Repository
	parentID      types.ID
	savingsID     types.ID
	foodID        types.ID
	transferID    types.ID
	counterpartID types.ID
}

// setupBankXfer builds a -100 split parent in Checking: a -60 Food line plus a
// -40 transfer line into Savings, whose counterpart lives on the regular table.
func setupBankXfer(t *testing.T) bankXferFixture {
	t.Helper()
	svc, accountRepo, _, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	food := category.NewCategory("Food", category.TypeExpense)
	if err := categoryRepo.Create(food); err != nil {
		t.Fatalf("create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	catLine := NewSplit(parent.ID, food.ID, types.MustNewMoney("-60.00"))
	xfer := makeTransferLineSplit(parent.ID, savings.ID, types.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(parent, []*Split{catLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	line := transferLineOf(t, svc, parent.ID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("expected a minted transfer_id on the transfer line")
	}
	cp, err := svc.findPairedByTransferID(line.TransferID.ID)
	if err != nil || cp == nil {
		t.Fatalf("expected a bank counterpart, err=%v cp=%v", err, cp)
	}
	if !cp.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Fatalf("counterpart amount = %s, want 40.00", cp.Amount.String())
	}
	return bankXferFixture{
		svc:           svc,
		accountRepo:   accountRepo,
		categoryRepo:  categoryRepo,
		parentID:      parent.ID,
		savingsID:     savings.ID,
		foodID:        food.ID,
		transferID:    line.TransferID.ID,
		counterpartID: cp.ID,
	}
}

func TestReplaceSplits_BankTransferLine_TUIShapedRetained(t *testing.T) {
	f := setupBankXfer(t)

	// TUI-shaped rewrite: transfer row with TransferID unset, same amounts.
	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeTransferLineSplit(f.parentID, f.savingsID, types.MustNewMoney("-40.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits (regression: used to fail the pairing CHECK): %v", err)
	}

	splits, _ := f.svc.GetSplits(f.parentID)
	if len(splits) != 2 {
		t.Fatalf("want 2 splits after replace, got %d", len(splits))
	}
	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || line.TransferID.ID != f.transferID {
		t.Errorf("transfer_id churned: got %v, want %s", line.TransferID, f.transferID)
	}
	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost after replace: err=%v cp=%v", err, cp)
	}
	if cp.ID != f.counterpartID {
		t.Errorf("counterpart identity churned: got %s, want %s", cp.ID, f.counterpartID)
	}
	if !cp.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("counterpart amount = %s, want 40.00", cp.Amount.String())
	}
	if n := accountTxnCount(t, f.svc, f.savingsID); n != 1 {
		t.Errorf("savings has %d transactions (orphan?), want 1", n)
	}
}

func TestReplaceSplits_BankTransferLine_AmountEditCascades(t *testing.T) {
	f := setupBankXfer(t)

	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-50.00")),
		makeTransferLineSplit(f.parentID, f.savingsID, types.MustNewMoney("-50.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil || cp == nil {
		t.Fatalf("counterpart lost: err=%v cp=%v", err, cp)
	}
	if cp.ID != f.counterpartID {
		t.Errorf("counterpart identity churned on amount edit")
	}
	if !cp.Amount.Equal(types.MustNewMoney("50.00")) {
		t.Errorf("counterpart amount = %s, want 50.00 (cascade failed)", cp.Amount.String())
	}
}

func TestReplaceSplits_BankTransferLine_DroppedDeletesCounterpart(t *testing.T) {
	f := setupBankXfer(t)

	// Collapse to a single categorized line summing to the parent amount.
	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-100.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	if got := transferLineOf(t, f.svc, f.parentID); got != nil {
		t.Errorf("transfer line should be gone, got %+v", got)
	}
	cp, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil {
		t.Fatalf("findPairedByTransferID: %v", err)
	}
	if cp != nil {
		t.Errorf("counterpart should be deleted, still present: %s", cp.ID)
	}
	if n := accountTxnCount(t, f.svc, f.savingsID); n != 0 {
		t.Errorf("savings has %d transactions, want 0", n)
	}
}

func TestReplaceSplits_BankTransferLine_AddedMintsCounterpart(t *testing.T) {
	f := setupBankXfer(t)

	// First drop the transfer line entirely.
	if err := f.svc.ReplaceSplits(f.parentID, []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-100.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (drop): %v", err)
	}

	// Now add a fresh transfer line back.
	if err := f.svc.ReplaceSplits(f.parentID, []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeTransferLineSplit(f.parentID, f.savingsID, types.MustNewMoney("-40.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (add): %v", err)
	}

	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("added transfer line has no minted transfer_id")
	}
	if line.TransferID.ID == f.transferID {
		t.Errorf("re-added transfer line reused the deleted transfer_id")
	}
	cp, err := f.svc.findPairedByTransferID(line.TransferID.ID)
	if err != nil || cp == nil {
		t.Fatalf("no counterpart minted for added transfer line: err=%v", err)
	}
	if !cp.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("new counterpart amount = %s, want 40.00", cp.Amount.String())
	}
	if n := accountTxnCount(t, f.svc, f.savingsID); n != 1 {
		t.Errorf("savings has %d transactions, want 1", n)
	}
}

func TestReplaceSplits_BankTransferLine_TargetChangeMovesCounterpart(t *testing.T) {
	f := setupBankXfer(t)
	vacation := createTestAccount(t, f.accountRepo, "Vacation")

	newSplits := []*Split{
		NewSplit(f.parentID, f.foodID, types.MustNewMoney("-60.00")),
		makeTransferLineSplit(f.parentID, vacation.ID, types.MustNewMoney("-40.00")),
	}
	if err := f.svc.ReplaceSplits(f.parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	// Old counterpart (in Savings) gone.
	oldCP, err := f.svc.findPairedByTransferID(f.transferID)
	if err != nil {
		t.Fatalf("findPairedByTransferID: %v", err)
	}
	if oldCP != nil {
		t.Errorf("old savings counterpart should be gone, still present: %s", oldCP.ID)
	}
	if n := accountTxnCount(t, f.svc, f.savingsID); n != 0 {
		t.Errorf("savings has %d transactions, want 0", n)
	}

	// New counterpart in Vacation.
	line := transferLineOf(t, f.svc, f.parentID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("moved transfer line missing transfer_id")
	}
	newCP, err := f.svc.findPairedByTransferID(line.TransferID.ID)
	if err != nil || newCP == nil {
		t.Fatalf("no counterpart in new target: err=%v", err)
	}
	if newCP.AccountID != vacation.ID {
		t.Errorf("counterpart account = %s, want vacation %s", newCP.AccountID, vacation.ID)
	}
	if !newCP.Amount.Equal(types.MustNewMoney("40.00")) {
		t.Errorf("moved counterpart amount = %s, want 40.00", newCP.Amount.String())
	}
}

// setupInvXfer builds an +800 split parent in Checking: a +1000 Salary income
// line plus a -200 transfer line into an investment IRA, whose counterpart is
// routed through the investment adapter.
func setupInvXfer(t *testing.T) (svc *Service, adapter *fakeInvCounterpart, parentID, iraID, salaryID, transferID types.ID) {
	t.Helper()
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)
	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	xfer := makeTransferLineSplit(parent.ID, ira.ID, types.MustNewMoney("-200.00"))
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, xfer}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}
	line := transferLineOf(t, svc, parent.ID)
	if line == nil || !line.TransferID.Valid {
		t.Fatalf("expected minted transfer_id")
	}
	row := adapter.findRowByTransferID(line.TransferID.ID)
	if row == nil {
		t.Fatalf("expected an investment-side counterpart in the adapter")
	}
	if !row.amount.Equal(types.MustNewMoney("200.00")) {
		t.Fatalf("adapter row amount = %s, want 200.00", row.amount.String())
	}
	return svc, adapter, parent.ID, ira.ID, salary.ID, line.TransferID.ID
}

func TestReplaceSplits_InvestmentTransferLine_RetainedAmountEdit(t *testing.T) {
	svc, adapter, parentID, iraID, salaryID, transferID := setupInvXfer(t)

	newSplits := []*Split{
		NewSplit(parentID, salaryID, types.MustNewMoney("1100.00")),
		makeTransferLineSplit(parentID, iraID, types.MustNewMoney("-300.00")),
	}
	if err := svc.ReplaceSplits(parentID, newSplits); err != nil {
		t.Fatalf("ReplaceSplits: %v", err)
	}

	if len(adapter.rows) != 1 {
		t.Fatalf("adapter should hold exactly 1 counterpart, got %d", len(adapter.rows))
	}
	row := adapter.findRowByTransferID(transferID)
	if row == nil {
		t.Fatalf("adapter counterpart transfer_id churned")
	}
	if !row.amount.Equal(types.MustNewMoney("300.00")) {
		t.Errorf("adapter row amount = %s, want 300.00", row.amount.String())
	}
}

func TestReplaceSplits_InvestmentTransferLine_DroppedAndAdded(t *testing.T) {
	svc, adapter, parentID, iraID, salaryID, _ := setupInvXfer(t)

	// Drop the transfer line.
	if err := svc.ReplaceSplits(parentID, []*Split{
		NewSplit(parentID, salaryID, types.MustNewMoney("800.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (drop): %v", err)
	}
	if len(adapter.rows) != 0 {
		t.Errorf("adapter counterpart should be deleted, %d remain", len(adapter.rows))
	}

	// Add it back.
	if err := svc.ReplaceSplits(parentID, []*Split{
		NewSplit(parentID, salaryID, types.MustNewMoney("1000.00")),
		makeTransferLineSplit(parentID, iraID, types.MustNewMoney("-200.00")),
	}); err != nil {
		t.Fatalf("ReplaceSplits (add): %v", err)
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("adapter should hold 1 counterpart after re-add, got %d", len(adapter.rows))
	}
	line := transferLineOf(t, svc, parentID)
	row := adapter.findRowByTransferID(line.TransferID.ID)
	if row == nil || !row.amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("re-added adapter counterpart wrong: %+v", row)
	}
}

func TestReplaceSplits_ReconciledBankCounterpartBlocks(t *testing.T) {
	reconcileCounterpart := func(t *testing.T, f bankXferFixture) {
		t.Helper()
		cp, err := f.svc.findPairedByTransferID(f.transferID)
		if err != nil || cp == nil {
			t.Fatalf("counterpart missing: %v", err)
		}
		cp.SetStatus(StatusReconciled)
		if err := f.svc.txnRepo.Update(cp); err != nil {
			t.Fatalf("reconcile counterpart: %v", err)
		}
	}

	assertUnchanged := func(t *testing.T, f bankXferFixture) {
		t.Helper()
		splits, _ := f.svc.GetSplits(f.parentID)
		if len(splits) != 2 {
			t.Errorf("splits mutated despite block: got %d, want 2", len(splits))
		}
		line := transferLineOf(t, f.svc, f.parentID)
		if line == nil || !line.Amount.Equal(types.MustNewMoney("-40.00")) {
			t.Errorf("transfer line mutated despite block: %+v", line)
		}
		cp, _ := f.svc.findPairedByTransferID(f.transferID)
		if cp == nil || !cp.Amount.Equal(types.MustNewMoney("40.00")) {
			t.Errorf("counterpart mutated despite block: %+v", cp)
		}
	}

	t.Run("amount change is blocked", func(t *testing.T) {
		f := setupBankXfer(t)
		reconcileCounterpart(t, f)

		err := f.svc.ReplaceSplits(f.parentID, []*Split{
			NewSplit(f.parentID, f.foodID, types.MustNewMoney("-50.00")),
			makeTransferLineSplit(f.parentID, f.savingsID, types.MustNewMoney("-50.00")),
		})
		var recErr *IsReconciledError
		if !errors.As(err, &recErr) {
			t.Fatalf("expected IsReconciledError, got %v", err)
		}
		assertUnchanged(t, f)
	})

	t.Run("drop is blocked", func(t *testing.T) {
		f := setupBankXfer(t)
		reconcileCounterpart(t, f)

		err := f.svc.ReplaceSplits(f.parentID, []*Split{
			NewSplit(f.parentID, f.foodID, types.MustNewMoney("-100.00")),
		})
		var recErr *IsReconciledError
		if !errors.As(err, &recErr) {
			t.Fatalf("expected IsReconciledError, got %v", err)
		}
		assertUnchanged(t, f)
	})

	t.Run("unrelated edit is allowed when the transfer line is untouched", func(t *testing.T) {
		f := setupBankXfer(t)
		reconcileCounterpart(t, f)

		// Re-categorize the non-transfer line but leave the transfer line
		// identical — no counterpart mutation, so the reconciled counterpart
		// must not block it.
		drink := category.NewCategory("Drink", category.TypeExpense)
		if err := f.categoryRepo.Create(drink); err != nil {
			t.Fatalf("create category: %v", err)
		}
		if err := f.svc.ReplaceSplits(f.parentID, []*Split{
			NewSplit(f.parentID, drink.ID, types.MustNewMoney("-60.00")),
			makeTransferLineSplit(f.parentID, f.savingsID, types.MustNewMoney("-40.00")),
		}); err != nil {
			t.Fatalf("ReplaceSplits should succeed (transfer line untouched): %v", err)
		}
		cp, _ := f.svc.findPairedByTransferID(f.transferID)
		if cp == nil || cp.Status != StatusReconciled || !cp.Amount.Equal(types.MustNewMoney("40.00")) {
			t.Errorf("counterpart should be untouched and still reconciled: %+v", cp)
		}
	})
}

func TestReplaceSplits_ReconciledInvestmentCounterpartBlocks(t *testing.T) {
	svc, adapter, parentID, iraID, salaryID, transferID := setupInvXfer(t)
	row := adapter.findRowByTransferID(transferID)
	row.reconciled = true

	err := svc.ReplaceSplits(parentID, []*Split{
		NewSplit(parentID, salaryID, types.MustNewMoney("1100.00")),
		makeTransferLineSplit(parentID, iraID, types.MustNewMoney("-300.00")),
	})
	var recErr *IsReconciledError
	if !errors.As(err, &recErr) {
		t.Fatalf("expected IsReconciledError, got %v", err)
	}
	// Adapter row untouched.
	if !row.amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("adapter row mutated despite block: %s", row.amount.String())
	}
}
