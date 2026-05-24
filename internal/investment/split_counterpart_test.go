package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// TestSplitCounterpart_PaycheckToIRA_CreatesInvestmentTransferCashRow
// is the canonical end-to-end test for the Phase 2 fix: a multi-line
// parent transaction (paycheck) in a regular account with a transfer-
// line split targeting an investment account (401k contribution-style)
// must produce a real investment.Transaction of type TransferCash in
// the investment account — not a malformed regular row.
func TestSplitCounterpart_PaycheckToIRA_CreatesInvestmentTransferCashRow(t *testing.T) {
	database := createTestDB(t)

	// Wire repos.
	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	invRepo := NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)

	// Build services in real wiring order.
	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, nil, txnRepo, caRepo, database)
	txnSvc.SetInvestmentCounterpart(invSvc)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	ira := account.NewAccount("Rollover IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	if err := accountRepo.Create(ira); err != nil {
		t.Fatalf("create IRA: %v", err)
	}

	salary := category.NewCategory("Salary", category.TypeIncome)
	if err := categoryRepo.Create(salary); err != nil {
		t.Fatalf("create salary category: %v", err)
	}

	// Paycheck: gross +1000 salary, −200 401k transfer line to IRA → net +800 in checking.
	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}

	if err := txnSvc.CreateWithSplits(parent, []*transaction.Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	// Pull the transfer_id minted onto the split.
	splits, err := txnSvc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits: %v", err)
	}
	var xfer *transaction.Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil || !xfer.TransferID.Valid {
		t.Fatalf("transfer-line not found or missing transfer_id")
	}

	// The IRA must hold a real investment.Transaction of TransferCash,
	// positive +200, linked by the same transfer_id, pointing back at
	// the checking account.
	invRows, err := invRepo.ListByAccount(ira.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(IRA): %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("expected 1 investment row in IRA, got %d", len(invRows))
	}
	row := invRows[0]
	if row.Type != TransactionTypeTransferCash {
		t.Errorf("row.Type = %q, want %q", row.Type, TransactionTypeTransferCash)
	}
	if !row.TotalAmount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("row.TotalAmount = %s, want 200.00", row.TotalAmount.String())
	}
	if !row.TransferID.Valid || row.TransferID.ID != xfer.TransferID.ID {
		t.Errorf("row.TransferID = %v, want %v", row.TransferID, xfer.TransferID)
	}
	if !row.TransferAccountID.Valid || row.TransferAccountID.ID != checking.ID {
		t.Errorf("row.TransferAccountID = %v, want %v", row.TransferAccountID, checking.ID)
	}

	// And: the IRA's regular-transactions ledger must NOT contain a row —
	// historically the bug created one here.
	regRows, err := txnRepo.ListByAccount(ira.ID)
	if err != nil {
		t.Fatalf("txnRepo.ListByAccount(IRA): %v", err)
	}
	if len(regRows) != 0 {
		t.Errorf("expected 0 regular-table rows in IRA, got %d (counterpart was misrouted)", len(regRows))
	}
}

// TestSplitCounterpart_DeleteParent_CascadesToInvestmentRow verifies the
// delete cascade works end-to-end against the real investment.Service.
func TestSplitCounterpart_DeleteParent_CascadesToInvestmentRow(t *testing.T) {
	database := createTestDB(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	invRepo := NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, nil, txnRepo, caRepo, database)
	txnSvc.SetInvestmentCounterpart(invSvc)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(checking)
	ira := account.NewAccount("IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(ira)

	salary := category.NewCategory("Salary", category.TypeIncome)
	_ = categoryRepo.Create(salary)

	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := txnSvc.CreateWithSplits(parent, []*transaction.Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	if rows, _ := invRepo.ListByAccount(ira.ID, TransactionFilter{}); len(rows) != 1 {
		t.Fatalf("expected 1 investment row pre-delete, got %d", len(rows))
	}

	if err := txnSvc.Delete(parent.ID); err != nil {
		t.Fatalf("Delete(parent): %v", err)
	}

	rows, err := invRepo.ListByAccount(ira.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(IRA) post-delete: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 investment rows after parent delete, got %d (cascade missed)", len(rows))
	}
}

// TestSplitCounterpart_ScheduledPaycheckPosting_LandsInvestmentRow is the
// scheduled-posting half of the Phase 2 acceptance criteria: a paycheck
// schedule with a transfer-line split to an investment account must, on
// PostWithEdits, create the investment-side counterpart via the adapter
// — not a malformed regular row.
func TestSplitCounterpart_ScheduledPaycheckPosting_LandsInvestmentRow(t *testing.T) {
	database := createTestDB(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)
	scheduledRepo := scheduled.NewRepository(database)

	invRepo := NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, nil, txnRepo, caRepo, database)
	txnSvc.SetInvestmentCounterpart(invSvc)
	scheduledSvc := scheduled.NewService(scheduledRepo, txnRepo, txnSvc, database)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(checking)
	ira := account.NewAccount("401k", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(ira)

	salary := category.NewCategory("Salary", category.TypeIncome)
	_ = categoryRepo.Create(salary)

	net, _ := types.NewMoney("800.00")
	gross, _ := types.NewMoney("1000.00")
	retire, _ := types.NewMoney("-200.00")

	st := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly, types.Today(), net)
	st.Splits = scheduled.SplitCollection{
		scheduled.NewCategorizedSplit(st.ID, salary.ID, gross),
		scheduled.NewTransferSplit(st.ID, ira.ID, retire),
	}
	if err := scheduledSvc.Create(st); err != nil {
		t.Fatalf("scheduledSvc.Create: %v", err)
	}

	// Post via PostWithEdits, the post-time preview dialog's code path.
	// Build the parent and splits the way the dialog does for a multi-line
	// schedule with no user edits.
	parent := transaction.NewTransaction(checking.ID, types.Today(), net)
	salaryLine := transaction.NewSplit(parent.ID, salary.ID, gross)
	contribLine := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            retire,
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if _, err := scheduledSvc.PostWithEdits(st.ID, parent, []*transaction.Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("PostWithEdits: %v", err)
	}

	invRows, err := invRepo.ListByAccount(ira.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(IRA): %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("expected 1 investment row from scheduled post, got %d", len(invRows))
	}
	row := invRows[0]
	if row.Type != TransactionTypeTransferCash {
		t.Errorf("row.Type = %q, want %q", row.Type, TransactionTypeTransferCash)
	}
	if !row.TotalAmount.Equal(types.MustNewMoney("200.00")) {
		t.Errorf("row.TotalAmount = %s, want 200.00", row.TotalAmount.String())
	}

	// Confirm IRA's regular ledger is empty (the bug we're fixing put a row
	// here).
	regRows, err := txnRepo.ListByAccount(ira.ID)
	if err != nil {
		t.Fatalf("ListByAccount(IRA, regular): %v", err)
	}
	if len(regRows) != 0 {
		t.Errorf("expected 0 regular rows in IRA after scheduled post, got %d", len(regRows))
	}
}

// TestSplitCounterpart_VoidParent_CascadesToInvestmentRow exercises the
// P2-003 acceptance criterion end-to-end against the real investment
// service: voiding a paycheck-style parent removes the investment-side
// TransferCash counterpart along with the parent's splits.
func TestSplitCounterpart_VoidParent_CascadesToInvestmentRow(t *testing.T) {
	database := createTestDB(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	invRepo := NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, nil, txnRepo, caRepo, database)
	txnSvc.SetInvestmentCounterpart(invSvc)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(checking)
	ira := account.NewAccount("IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(ira)

	salary := category.NewCategory("Salary", category.TypeIncome)
	_ = categoryRepo.Create(salary)

	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := txnSvc.CreateWithSplits(parent, []*transaction.Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	if rows, _ := invRepo.ListByAccount(ira.ID, TransactionFilter{}); len(rows) != 1 {
		t.Fatalf("expected 1 investment row pre-void, got %d", len(rows))
	}

	if err := txnSvc.VoidTransaction(parent.ID); err != nil {
		t.Fatalf("VoidTransaction(parent): %v", err)
	}

	rows, err := invRepo.ListByAccount(ira.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(IRA) post-void: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 investment rows after parent void, got %d (cascade missed)", len(rows))
	}

	got, err := txnSvc.GetByID(parent.ID)
	if err != nil {
		t.Fatalf("GetByID(parent) post-void: %v", err)
	}
	if got.Status != transaction.StatusVoid {
		t.Errorf("parent.Status = %s, want StatusVoid", got.Status)
	}
}

// TestSplitCounterpart_UpdateSplitAmount_PropagatesToInvestmentRow is the
// edit-line half of the P2-003 acceptance criterion: editing a transfer-
// line split's amount must update the linked investment-side row's
// TotalAmount accordingly, with the sign flipped to the destination frame.
func TestSplitCounterpart_UpdateSplitAmount_PropagatesToInvestmentRow(t *testing.T) {
	database := createTestDB(t)

	txnRepo := transaction.NewRepository(database)
	splitRepo := transaction.NewSplitRepository(database)
	transferRepo := transaction.NewTransferRepository(database, txnRepo)
	payeeRepo := payee.NewRepository(database)
	accountRepo := account.NewRepository(database)
	categoryRepo := category.NewRepository(database)

	invRepo := NewRepository(database)
	positionRepo := NewPositionRepository(database)
	lotRepo := NewLotRepository(database)
	transactionLotRepo := NewTransactionLotRepository(database)
	caRepo := NewCorporateActionRepository(database)

	txnSvc := transaction.NewService(txnRepo, splitRepo, transferRepo, payeeRepo, accountRepo, database)
	invSvc := NewService(invRepo, accountRepo, positionRepo, lotRepo, transactionLotRepo, nil, txnRepo, caRepo, database)
	txnSvc.SetInvestmentCounterpart(invSvc)

	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(checking)
	ira := account.NewAccount("IRA", account.TypeInvestment, "USD", types.ZeroMoney, types.Today())
	_ = accountRepo.Create(ira)

	salary := category.NewCategory("Salary", category.TypeIncome)
	_ = categoryRepo.Create(salary)

	parent := transaction.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("800.00"))
	salaryLine := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	contribLine := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		CategoryID:        types.NilID,
		Amount:            types.MustNewMoney("-200.00"),
		TransferAccountID: types.NullableID{ID: ira.ID, Valid: true},
	}
	if err := txnSvc.CreateWithSplits(parent, []*transaction.Split{salaryLine, contribLine}); err != nil {
		t.Fatalf("CreateWithSplits: %v", err)
	}

	// Find the persisted transfer-line split and bump its amount from
	// -200 to -250 (a "raised 401k contribution" edit). The salary line
	// is reduced to keep the splits in balance with the new -250 amount
	// (parent stays at +800 = 950 + -50 - 200 ... but we just want the
	// transfer-line amount edit to flow to the investment row).
	splits, err := txnSvc.GetSplits(parent.ID)
	if err != nil {
		t.Fatalf("GetSplits: %v", err)
	}
	var xfer *transaction.Split
	for _, s := range splits {
		if s.TransferAccountID.Valid {
			xfer = s
		}
	}
	if xfer == nil {
		t.Fatalf("transfer-line not found")
	}

	xfer.Amount = types.MustNewMoney("-250.00")
	if err := txnSvc.UpdateSplit(xfer); err != nil {
		t.Fatalf("UpdateSplit: %v", err)
	}

	// Investment side must now read +250.
	invRows, err := invRepo.ListByAccount(ira.ID, TransactionFilter{})
	if err != nil {
		t.Fatalf("ListByAccount(IRA) post-edit: %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("expected 1 investment row post-edit, got %d", len(invRows))
	}
	if !invRows[0].TotalAmount.Equal(types.MustNewMoney("250.00")) {
		t.Errorf("investment-row TotalAmount = %s, want 250.00 (negated split)", invRows[0].TotalAmount.String())
	}
}
