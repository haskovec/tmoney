package transaction

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// failingQueryer wraps a real db.Queryer and returns an injected error on the
// Nth Exec (1-based), delegating everything else. It lets a test prove that a
// mid-flow write failure rolls the whole flow back rather than leaving partial
// state — the regression that would silently reappear if a write escaped the
// caller's transaction.
type failingQueryer struct {
	inner  db.Queryer
	failOn int // fail on the failOn-th Exec; 0 disables injection
	execN  int
}

func (f *failingQueryer) Exec(query string, args ...any) (sql.Result, error) {
	f.execN++
	if f.failOn != 0 && f.execN == f.failOn {
		return nil, fmt.Errorf("injected fault on exec #%d", f.execN)
	}
	return f.inner.Exec(query, args...)
}

func (f *failingQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	return f.inner.Query(query, args...)
}

func (f *failingQueryer) QueryRow(query string, args ...any) *sql.Row {
	return f.inner.QueryRow(query, args...)
}

// TestCreateWithSplits_FaultRollsBack forces the second split insert to fail
// during a two-split CreateWithSplits and asserts the whole flow rolls back:
// no parent row and no split rows survive. Before phase 5 the parent + first
// split were left behind on a mid-flight failure.
func TestCreateWithSplits_FaultRollsBack(t *testing.T) {
	svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
	checking := createTestAccount(t, accountRepo, "Checking")

	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("-100.00"))
	line1 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-60.00"))
	line2 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-40.00"))

	err := svc.db.WithTx(func(tx db.Queryer) error {
		// Exec order inside the flow: parent insert (#1), split #1 (#2),
		// split #2 (#3). Fail the second split so the parent and first split
		// must roll back too.
		fw := &failingQueryer{inner: tx, failOn: 3}
		return svc.InTx(fw).CreateWithSplits(parent, []*Split{line1, line2})
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	txns, err := svc.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("ListByAccount() error = %v", err)
	}
	if len(txns) != 0 {
		t.Errorf("expected no parent row after rollback, got %d", len(txns))
	}
	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(splits) != 0 {
		t.Errorf("expected no split rows after rollback, got %d", len(splits))
	}
}

// TestReplaceSplits_FaultRollsBack is the money test: it commits a transaction
// with two splits, then runs ReplaceSplits under a queryer that fails after the
// existing splits have been deleted. It asserts the ORIGINAL splits are fully
// restored — a mid-flight failure must not leave the transaction with half a
// split set (or none). Before phase 5 the delete had already landed
// non-transactionally, destroying the originals.
func TestReplaceSplits_FaultRollsBack(t *testing.T) {
	svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
	checking := createTestAccount(t, accountRepo, "Checking")

	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("-100.00"))
	orig1 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-60.00"))
	orig2 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(parent, []*Split{orig1, orig2}); err != nil {
		t.Fatalf("CreateWithSplits() setup error = %v", err)
	}

	newSplits := []*Split{
		NewSplit(parent.ID, cat.ID, types.MustNewMoney("-70.00")),
		NewSplit(parent.ID, cat.ID, types.MustNewMoney("-30.00")),
	}

	err := svc.db.WithTx(func(tx db.Queryer) error {
		// Exec order inside the execution phase: DeleteByTransaction (#1),
		// then the first new split insert (#2). Fail #2 so the delete of the
		// originals must roll back with it.
		fw := &failingQueryer{inner: tx, failOn: 2}
		return svc.InTx(fw).ReplaceSplits(parent.ID, newSplits)
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("expected 2 original splits intact after rollback, got %d", len(splits))
	}
	var have60, have40 bool
	for _, s := range splits {
		if s.Amount.Equal(types.MustNewMoney("-60.00")) {
			have60 = true
		}
		if s.Amount.Equal(types.MustNewMoney("-40.00")) {
			have40 = true
		}
	}
	if !have60 || !have40 {
		t.Errorf("original split amounts not intact after rollback: have -60=%v -40=%v", have60, have40)
	}
}

// TestCreateWithSplits_HappyPath_RegularTransferLine exercises the runInTx happy
// path with a transfer-line split whose target is a REGULAR account: the parent,
// its splits, and the paired counterpart all commit in one transaction, linked
// by a shared transfer_id.
func TestCreateWithSplits_HappyPath_RegularTransferLine(t *testing.T) {
	svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	transferLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}

	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, transferLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Parent and both splits landed.
	if _, err := svc.GetByID(parent.ID); err != nil {
		t.Fatalf("GetByID(parent) error = %v", err)
	}
	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("expected 2 splits, got %d", len(splits))
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatal("transfer-line split not found")
	}

	// The paired counterpart landed in Savings, linked by the same transfer_id.
	savingsRows, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(savingsRows) != 1 {
		t.Fatalf("expected 1 paired counterpart in Savings, got %d", len(savingsRows))
	}
	paired := savingsRows[0]
	if !paired.TransferID.Valid || paired.TransferID.ID != xfer.TransferID.ID {
		t.Errorf("counterpart transfer_id = %v, want %s", paired.TransferID, xfer.TransferID.ID)
	}
	if !paired.Amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("counterpart amount = %s, want 200.00", paired.Amount.String())
	}
}

// TestRestoreVoidedTransactionWithSplits_HappyPath voids a split transaction
// (which removes its splits and zeroes the row) then restores it via the
// composed undo method, asserting both the row fields and the split set come
// back in one call.
func TestRestoreVoidedTransactionWithSplits_HappyPath(t *testing.T) {
	svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
	checking := createTestAccount(t, accountRepo, "Checking")

	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("-100.00"))
	parent.SetMemo("original")
	orig1 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-60.00"))
	orig2 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(parent, []*Split{orig1, orig2}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Capture the pre-void state exactly as the undo command does.
	before, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	beforeSplits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}

	if err := svc.VoidTransaction(parent.ID); err != nil {
		t.Fatalf("VoidTransaction() error = %v", err)
	}

	if err := svc.RestoreVoidedTransactionWithSplits(parent.ID, before.Amount, before.Memo, before.Status, beforeSplits); err != nil {
		t.Fatalf("RestoreVoidedTransactionWithSplits() error = %v", err)
	}

	restored, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID() after restore error = %v", err)
	}
	if restored.IsVoid() {
		t.Error("expected transaction to be un-voided after restore")
	}
	if !restored.Amount.Equal(types.MustNewMoney("-100.00")) {
		t.Errorf("restored amount = %s, want -100.00", restored.Amount.String())
	}
	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() after restore error = %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("expected 2 restored splits, got %d", len(splits))
	}
}

// TestUpdateWithSplits_FaultRollsBack updates a parent and replaces its splits
// via the composed undo method under a queryer that fails during the split
// rewrite (the second half). It asserts the parent update rolled back too — the
// memo edit is gone AND the original split set survives intact.
func TestUpdateWithSplits_FaultRollsBack(t *testing.T) {
	svc, accountRepo, categoryRepo, _ := createTestTransactionServiceWithCategories(t)
	checking := createTestAccount(t, accountRepo, "Checking")

	cat := category.NewCategory("Groceries", category.TypeExpense)
	if err := categoryRepo.Create(cat); err != nil {
		t.Fatalf("Create category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.NewDate(2024, time.January, 1), types.MustNewMoney("-100.00"))
	parent.SetMemo("original")
	orig1 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-60.00"))
	orig2 := NewSplit(parent.ID, cat.ID, types.MustNewMoney("-40.00"))
	if err := svc.CreateWithSplits(parent, []*Split{orig1, orig2}); err != nil {
		t.Fatalf("CreateWithSplits() setup error = %v", err)
	}

	after, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	after.SetMemo("changed")
	newSplits := []*Split{
		NewSplit(parent.ID, cat.ID, types.MustNewMoney("-70.00")),
		NewSplit(parent.ID, cat.ID, types.MustNewMoney("-30.00")),
	}

	err = svc.db.WithTx(func(tx db.Queryer) error {
		// Exec order: parent Update (#1), then ReplaceSplits'
		// DeleteByTransaction (#2), then the first new split insert (#3). Fail #3
		// so both the parent update and the split delete roll back.
		fw := &failingQueryer{inner: tx, failOn: 3}
		return svc.InTx(fw).UpdateWithSplits(after, newSplits)
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	// Parent update rolled back: memo is still the original.
	got, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID() after rollback error = %v", err)
	}
	if !got.Memo.Valid || got.Memo.String != "original" {
		t.Errorf("expected memo to roll back to \"original\", got %+v", got.Memo)
	}

	// Original splits intact.
	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	if len(splits) != 2 {
		t.Fatalf("expected 2 original splits intact after rollback, got %d", len(splits))
	}
	var have60, have40 bool
	for _, s := range splits {
		if s.Amount.Equal(types.MustNewMoney("-60.00")) {
			have60 = true
		}
		if s.Amount.Equal(types.MustNewMoney("-40.00")) {
			have40 = true
		}
	}
	if !have60 || !have40 {
		t.Errorf("original split amounts not intact after rollback: have -60=%v -40=%v", have60, have40)
	}
}

// The whole-transfer atomicity tests that used to live here — CreateTransfer and
// RecreateTransferPair, happy path and fault-injected — moved with their subject.
// internal/transfer's TestCreate_FaultRollsBackBothLegs runs the same injection
// across ALL FOUR shapes and additionally asserts that no write escaped the
// transaction into either ledger, which the bank-only versions could not.
