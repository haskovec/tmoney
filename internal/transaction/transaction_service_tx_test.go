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

// TestTransactionService_CreateTransfer_FaultRollsBack forces the second Exec
// (the to-leg insert) to fail and asserts the flow errors and leaves no rows
// for either leg — no orphaned from-leg.
func TestTransactionService_CreateTransfer_FaultRollsBack(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	from := createTestAccount(t, accountRepo, "Checking")
	to := createTestAccount(t, accountRepo, "Savings")

	amount, _ := types.NewMoney("100.00")
	date := types.NewDate(2020, time.January, 1)

	err := svc.db.WithTx(func(tx db.Queryer) error {
		// Fail the second Exec: the from-leg insert lands, the to-leg insert
		// errors, so the whole tx must roll back.
		fw := &failingQueryer{inner: tx, failOn: 2}
		_, e := svc.InTx(fw).CreateTransfer(from.ID, to.ID, date, amount, "", types.NullableID{})
		return e
	})
	if err == nil {
		t.Fatal("expected injected fault error, got nil")
	}

	fromTxns, err := svc.ListByAccount(from.ID)
	if err != nil {
		t.Fatalf("ListByAccount(from) error = %v", err)
	}
	toTxns, err := svc.ListByAccount(to.ID)
	if err != nil {
		t.Fatalf("ListByAccount(to) error = %v", err)
	}
	if len(fromTxns) != 0 || len(toTxns) != 0 {
		t.Fatalf("expected no rows after rollback, got from=%d to=%d", len(fromTxns), len(toTxns))
	}
}

// TestTransactionService_CreateTransfer_HappyPath exercises the new runInTx
// path with no injected fault: the unbound service opens and commits its own
// transaction, and both legs land linked by a shared transfer_id.
func TestTransactionService_CreateTransfer_HappyPath(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t)
	from := createTestAccount(t, accountRepo, "Checking")
	to := createTestAccount(t, accountRepo, "Savings")

	amount, _ := types.NewMoney("100.00")
	date := types.NewDate(2020, time.January, 1)

	pair, err := svc.CreateTransfer(from.ID, to.ID, date, amount, "", types.NullableID{})
	if err != nil {
		t.Fatalf("CreateTransfer() error = %v", err)
	}

	if !pair.FromTransaction.TransferID.Valid {
		t.Fatal("expected from leg to carry a transfer_id")
	}

	got, err := svc.GetTransferPair(pair.FromTransaction.TransferID.ID)
	if err != nil {
		t.Fatalf("GetTransferPair() error = %v", err)
	}
	if got.FromTransaction == nil || got.ToTransaction == nil {
		t.Fatal("expected both legs present after commit")
	}
	if got.FromTransaction.TransferID.ID != got.ToTransaction.TransferID.ID {
		t.Fatalf("legs not linked: from transfer_id=%s to transfer_id=%s",
			got.FromTransaction.TransferID.ID, got.ToTransaction.TransferID.ID)
	}
	if !got.FromTransaction.Amount.Neg().Equal(got.ToTransaction.Amount) {
		t.Fatalf("legs not equal-and-opposite: from=%s to=%s",
			got.FromTransaction.Amount, got.ToTransaction.Amount)
	}
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
