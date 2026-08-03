package transaction

import (
	"fmt"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/types"
)

// fakeInvCounterpart is a stub InvestmentCounterpartPort used by
// tests in the transaction package. The transaction package cannot
// import investment (cycle), so we record calls in-memory and check
// them via the captured fields below.
type fakeInvCounterpart struct {
	rows map[types.ID]*fakeInvRow // keyed by row ID

	createErr error
	updateErr error
	deleteErr error
}

type fakeInvRow struct {
	id           types.ID
	invAcctID    types.ID
	otherAcctID  types.ID
	date         types.Date
	amount       types.Money
	memo         string
	transferID   types.ID
	reconciled   bool
	createCalled int
}

func newFakeInvCounterpart() *fakeInvCounterpart {
	return &fakeInvCounterpart{rows: map[types.ID]*fakeInvRow{}}
}

func (f *fakeInvCounterpart) CreateCounterpart(
	_ db.Queryer,
	invAcctID, otherAcctID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	transferID types.ID,
) (types.ID, error) {
	if f.createErr != nil {
		return types.ID{}, f.createErr
	}
	id := types.NewID()
	f.rows[id] = &fakeInvRow{
		id:           id,
		invAcctID:    invAcctID,
		otherAcctID:  otherAcctID,
		date:         date,
		amount:       amount,
		memo:         memo,
		transferID:   transferID,
		createCalled: 1,
	}
	return id, nil
}

func (f *fakeInvCounterpart) FindCounterpart(_ db.Queryer, transferID types.ID) (types.ID, bool, bool, error) {
	for _, r := range f.rows {
		if r.transferID == transferID {
			return r.id, r.reconciled, true, nil
		}
	}
	return types.ID{}, false, false, nil
}

func (f *fakeInvCounterpart) DeleteCounterpart(_ db.Queryer, rowID types.ID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.rows, rowID)
	return nil
}

func (f *fakeInvCounterpart) UpdateCounterpartAmount(_ db.Queryer, rowID types.ID, newAmount types.Money) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	r, ok := f.rows[rowID]
	if !ok {
		return fmt.Errorf("row %s not in fake", rowID.String())
	}
	r.amount = newAmount
	return nil
}

// findRowByTransferID returns the fake row linked to the transfer_id,
// or nil if none exists. Test helper.
func (f *fakeInvCounterpart) findRowByTransferID(transferID types.ID) *fakeInvRow {
	for _, r := range f.rows {
		if r.transferID == transferID {
			return r
		}
	}
	return nil
}

// createTestServiceWithAdapter builds a transaction.Service with an
// account.Repository and a fake investment counterpart adapter wired in.
func createTestServiceWithAdapter(t *testing.T) (*Service, *account.Repository, *fakeInvCounterpart, *category.Repository) {
	t.Helper()
	database := createTestDB(t)
	txnRepo := NewRepository(database)
	splitRepo := NewSplitRepository(database)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	adapter := newFakeInvCounterpart()
	svc := NewService(txnRepo, splitRepo, payeeRepo, accountRepo, adapter, database)
	return svc, accountRepo, adapter, categoryRepo
}

// TestCreateWithSplits_InvestmentTargetSplit_DispatchesToAdapter verifies
// the core P2 fix: a transfer-line split whose target is an investment
// account routes the counterpart through the adapter instead of creating
// a malformed regular transaction in the investment account.
func TestCreateWithSplits_InvestmentTargetSplit_DispatchesToAdapter(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "Rollover IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	// Paycheck-style parent: net +800 in checking, with -200 transfer-
	// line split to the IRA (401k-contribution style).
	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}

	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment-side counterpart, got %d", len(adapter.rows))
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found in splits")
	}

	row := adapter.findRowByTransferID(xfer.TransferID.ID)
	if row == nil {
		t.Fatalf("adapter row not found for transfer_id %s", xfer.TransferID.ID.String())
	}
	if row.invAcctID != ira.ID {
		t.Errorf("counterpart invAcctID = %s, want %s", row.invAcctID.String(), ira.ID.String())
	}
	if row.otherAcctID != checking.ID {
		t.Errorf("counterpart otherAcctID = %s, want %s", row.otherAcctID.String(), checking.ID.String())
	}
	if !row.amount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("counterpart amount = %s, want 200.00 (negated split)", row.amount.String())
	}
	if row.transferID != xfer.TransferID.ID {
		t.Errorf("counterpart transferID = %s, want %s", row.transferID.String(), xfer.TransferID.ID.String())
	}

	// No regular-side counterpart was created in the IRA. ListByAccount
	// on the IRA should yield no rows on the regular transactions table.
	iraRegRows, err := svc.ListByAccount(ira.ID)
	if err != nil {
		t.Fatalf("ListByAccount(IRA) error = %v", err)
	}
	if len(iraRegRows) != 0 {
		t.Errorf("expected 0 regular-table rows in IRA, got %d (counterpart was misrouted)", len(iraRegRows))
	}
}

// TestCreateWithSplits_HSATargetSplit_DispatchesToAdapter — HSA accounts
// are investment-type per IsInvestmentType() and must also route through
// the adapter.
func TestCreateWithSplits_HSATargetSplit_DispatchesToAdapter(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	hsa := createTestAccountOfType(t, accountRepo, "HSA", account.TypeHSA)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("950.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-50.00"),
		TransferAccountID: types.NullableID{ID: hsa.ID, Valid: true},
	}

	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment-side counterpart for HSA target, got %d", len(adapter.rows))
	}
}

// TestCreateWithSplits_InvestmentTargetSplit_NoAdapterWired_ReturnsError
// guards against silently creating malformed data when the adapter is
// missing.
func TestCreateWithSplits_InvestmentTargetSplit_NoAdapterWired_ReturnsError(t *testing.T) {
	svc, accountRepo := createTestTransactionService(t) // no adapter wired
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-100.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-100.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}

	err := svc.CreateWithSplits(parent, []*Split{contribLine})
	if err == nil {
		t.Fatal("CreateWithSplits() expected error when adapter is missing, got nil")
	}
	// Parent must not have been persisted.
	list, lerr := svc.ListByAccount(checking.ID)
	if lerr != nil {
		t.Fatalf("ListByAccount() error = %v", lerr)
	}
	if len(list) != 0 {
		t.Errorf("expected no transactions after rejected split, got %d", len(list))
	}
}

// TestCreateWithSplits_InvestmentCounterpartRollback verifies that when
// a later split fails, an earlier investment-side counterpart is also
// removed (no orphan investment.Transaction row).
func TestCreateWithSplits_InvestmentCounterpartRollback(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("100.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("300.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	// Second transfer line with an invalid target to trigger a late failure
	// — point at a non-existent account ID.
	bogusID := types.NewID()
	bogusLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("0.00"),
		TransferAccountID: types.NullableID{ID: bogusID, Valid: true},
	}

	err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine, bogusLine})
	if err == nil {
		t.Fatal("CreateWithSplits() expected error on bogus target account, got nil")
	}

	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after rollback, got %d (leak)", len(adapter.rows))
	}
}

// TestUpdateSplit_InvestmentTarget_AmountEditPropagates verifies that
// an amount edit on a transfer-line split whose target is an investment
// account propagates to the investment-side row.
func TestUpdateSplit_InvestmentTarget_AmountEditPropagates(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found")
	}

	xfer.Amount = types.MustNewMoney("-250.00")
	if err := svc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit() error = %v", err)
	}

	row := adapter.findRowByTransferID(xfer.TransferID.ID)
	if row == nil {
		t.Fatalf("investment counterpart not found after edit")
	}
	if !row.amount.Equal(types.MustNewMoney("250.00")) {
		t.Errorf("counterpart amount after edit = %s, want 250.00", row.amount.String())
	}
}

// TestUpdateSplit_MoveFromBankToInvestment_RoutesCorrectly verifies the
// cross-table target-change cascade: old paired (regular) row is
// deleted, new paired (investment) row is created via the adapter.
func TestUpdateSplit_MoveFromBankToInvestment_RoutesCorrectly(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Confirm pre-state: 1 paired row in Savings (regular table), no
	// investment-side counterparts.
	savingsBefore, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) before move: %v", err)
	}
	if len(savingsBefore) != 1 {
		t.Fatalf("expected 1 paired row in Savings before move, got %d", len(savingsBefore))
	}
	if len(adapter.rows) != 0 {
		t.Fatalf("expected 0 investment counterparts before move, got %d", len(adapter.rows))
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found")
	}

	// Move target from Savings to IRA.
	xfer.TransferAccountID = types.NullableID{ID: ira.ID, Valid: true}
	if err := svc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit() to move target: %v", err)
	}

	// Post-state: 0 paired rows in Savings; 1 investment counterpart on IRA.
	savingsAfter, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) after move: %v", err)
	}
	if len(savingsAfter) != 0 {
		t.Errorf("expected 0 paired rows in Savings after move, got %d", len(savingsAfter))
	}
	if len(adapter.rows) != 1 {
		t.Errorf("expected 1 investment counterpart after move, got %d", len(adapter.rows))
	}
	for _, r := range adapter.rows {
		if r.invAcctID != ira.ID {
			t.Errorf("new counterpart on wrong account: got %s, want %s", r.invAcctID.String(), ira.ID.String())
		}
		if !r.amount.Equal(types.MustNewMoney("200.00")) {
			t.Errorf("new counterpart amount = %s, want 200.00", r.amount.String())
		}
	}
}

// TestUpdateSplit_MoveFromInvestmentToBank_RoutesCorrectly verifies the
// reverse direction: old investment-side row is removed, new bank-side
// regular row is created.
func TestUpdateSplit_MoveFromInvestmentToBank_RoutesCorrectly(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment counterpart pre-move, got %d", len(adapter.rows))
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found")
	}

	// Move target from IRA to Savings.
	xfer.TransferAccountID = types.NullableID{ID: savings.ID, Valid: true}
	if err := svc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit() to move target: %v", err)
	}

	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after move-away, got %d (leak)", len(adapter.rows))
	}
	savingsRows, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) after move: %v", err)
	}
	if len(savingsRows) != 1 {
		t.Errorf("expected 1 paired row in Savings after move-in, got %d", len(savingsRows))
	}
}

// TestDelete_OfParentWithInvestmentSplit_CascadesToInvestmentRow verifies
// the delete cascade reaches the investment-side counterpart.
func TestDelete_OfParentWithInvestmentSplit_CascadesToInvestmentRow(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment counterpart pre-delete, got %d", len(adapter.rows))
	}

	if err := svc.Delete(parent.ID); err != nil {
		t.Fatalf("Delete(parent) error = %v", err)
	}

	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after parent delete, got %d (cascade missed)", len(adapter.rows))
	}
}

// TestDeleteSplit_InvestmentTarget_RemovesInvestmentRow verifies the
// single-split delete cascade.
func TestDeleteSplit_InvestmentTarget_RemovesInvestmentRow(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	splits, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits() error = %v", err)
	}
	var xfer *Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found")
	}

	if err := svc.DeleteSplit(xfer.ID); err != nil {
		t.Fatalf("DeleteSplit() error = %v", err)
	}

	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after DeleteSplit, got %d", len(adapter.rows))
	}
}

// TestCreateTransferLineCounterpart_RegularStillUsesRegularRepo guards
// against the dispatch accidentally routing bank-target splits through
// the adapter — we want the regular path to be unchanged for bank
// targets.
func TestCreateTransferLineCounterpart_RegularStillUsesRegularRepo(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
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

	if len(adapter.rows) != 0 {
		t.Errorf("adapter unexpectedly called for bank-target split (got %d rows)", len(adapter.rows))
	}
	savingsRows, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) error = %v", err)
	}
	if len(savingsRows) != 1 {
		t.Errorf("expected 1 paired regular row in Savings, got %d", len(savingsRows))
	}
}

// TestVoidTransaction_OfParentWithInvestmentSplit_CascadesToInvestmentRow
// verifies that voiding a multi-line parent with an investment-side
// transfer-line removes the investment counterpart. Without the cascade
// fix, the investment row was orphaned after void.
func TestVoidTransaction_OfParentWithInvestmentSplit_CascadesToInvestmentRow(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment counterpart pre-void, got %d", len(adapter.rows))
	}

	if err := svc.VoidTransaction(parent.ID); err != nil {
		t.Fatalf("VoidTransaction(parent) error = %v", err)
	}

	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after parent void, got %d (cascade missed)", len(adapter.rows))
	}

	got, err := svc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID(parent) error = %v", err)
	}
	if got.Status != StatusVoid {
		t.Errorf("parent.Status = %s, want StatusVoid", got.Status)
	}
	if !got.Amount.IsZero() {
		t.Errorf("parent.Amount = %s, want 0", got.Amount.String())
	}
	remaining, err := svc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits(parent) error = %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 splits after void, got %d", len(remaining))
	}
}

// TestVoidTransaction_OfParentWithMixedCounterparts_CascadesBoth verifies
// that voiding a paycheck-style parent with one bank-side and one
// investment-side transfer-line cleans up BOTH counterparts coherently.
func TestVoidTransaction_OfParentWithMixedCounterparts_CascadesBoth(t *testing.T) {
	svc, accountRepo, adapter, categoryRepo := createTestServiceWithAdapter(t)
	checking := createTestAccount(t, accountRepo, "Checking")
	savings := createTestAccount(t, accountRepo, "Savings")
	ira := createTestAccountOfType(t, accountRepo, "IRA", account.TypeInvestment)

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("Create salary category: %v", err)
	}

	parent := NewTransaction(checking.ID, types.Today(), types.MustNewMoney("700.00"))
	salaryLine := NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	savingsLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-100.00"),
		TransferAccountID: types.NullableID{ID: savings.ID, Valid: true},
	}
	contribLine := &Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := svc.CreateWithSplits(parent, []*Split{salaryLine, savingsLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits() error = %v", err)
	}

	// Pre-state: 1 paired regular row in Savings, 1 adapter row for IRA.
	savingsBefore, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) pre-void: %v", err)
	}
	if len(savingsBefore) != 1 {
		t.Fatalf("expected 1 paired row in Savings pre-void, got %d", len(savingsBefore))
	}
	if len(adapter.rows) != 1 {
		t.Fatalf("expected 1 investment counterpart pre-void, got %d", len(adapter.rows))
	}

	if err := svc.VoidTransaction(parent.ID); err != nil {
		t.Fatalf("VoidTransaction(parent) error = %v", err)
	}

	savingsAfter, err := svc.ListByAccount(savings.ID)
	if err != nil {
		t.Fatalf("ListByAccount(savings) post-void: %v", err)
	}
	if len(savingsAfter) != 0 {
		t.Errorf("expected 0 paired rows in Savings after void, got %d (bank cascade missed)", len(savingsAfter))
	}
	if len(adapter.rows) != 0 {
		t.Errorf("expected 0 investment counterparts after void, got %d (investment cascade missed)", len(adapter.rows))
	}
}
