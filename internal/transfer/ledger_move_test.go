package transfer

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// seedCashOnlyHSA fills the invested-HSA fixture with every cash kind the
// move accepts: a deposit, a withdrawal, interest, a fee, an inbound whole
// transfer from Checking, an outbound whole transfer to Brokerage, and an
// inbound paycheck split line from Checking. It returns the ids the
// assertions need.
type cashOnlySeed struct {
	depositID, withdrawalID, interestID, feeID types.ID
	inTransferID, outTransferID, splitTransferID types.ID
}

func (h *harness) seedCashOnlyHSA() cashOnlySeed {
	h.t.Helper()
	var s cashOnlySeed
	must := func(txn *investment.Transaction, err error) types.ID {
		h.t.Helper()
		if err != nil {
			h.t.Fatalf("seed hsa: %v", err)
		}
		return txn.ID
	}
	s.depositID = must(h.invSvc.Deposit(h.hsa.ID, testDate(), types.MustNewMoney("1000.00"), "opening deposit"))
	s.withdrawalID = must(h.invSvc.Withdrawal(h.hsa.ID, testDate(), types.MustNewMoney("180.25"), "clinic visit"))
	s.interestID = must(h.invSvc.Interest(h.hsa.ID, testDate(), types.MustNewMoney("0.12"), ""))
	s.feeID = must(h.invSvc.Fee(h.hsa.ID, testDate(), types.MustNewMoney("2.00"), "monthly fee"))
	s.inTransferID, _, _ = h.seed(h.checking, h.hsa, "250.00", "payroll", types.NullableID{})
	s.outTransferID, _, _ = h.seed(h.hsa, h.brokerage, "600.00", "sweep to invest", types.NullableID{})

	salary := h.newCategory("Salary", category.TypeIncome)
	parent := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("950.00"))
	earnings := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1000.00"))
	line := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		TransactionID:     parent.ID,
		Amount:            types.MustNewMoney("-50.00"),
		TransferAccountID: types.NullableID{ID: h.hsa.ID, Valid: true},
	}
	if err := h.txnSvc.CreateWithSplits(parent, []*transaction.Split{earnings, line}); err != nil {
		h.t.Fatalf("seed hsa: paycheck split: %v", err)
	}
	splits, err := h.txnSvc.GetSplits(parent.ID)
	if err != nil {
		h.t.Fatalf("seed hsa: GetSplits: %v", err)
	}
	for _, sp := range splits {
		if sp.TransferID.Valid {
			s.splitTransferID = sp.TransferID.ID
		}
	}
	if s.splitTransferID.IsNil() {
		h.t.Fatal("seed hsa: paycheck line minted no transfer id")
	}
	return s
}

func (h *harness) invCount(acctID types.ID) int {
	h.t.Helper()
	rows, err := h.invRepo.ListByAccount(acctID, investment.TransactionFilter{})
	if err != nil {
		h.t.Fatalf("list investment rows: %v", err)
	}
	return len(rows)
}

func (h *harness) regCount(acctID types.ID) int {
	h.t.Helper()
	n, err := h.txnRepo.CountByAccount(acctID)
	if err != nil {
		h.t.Fatalf("count register rows: %v", err)
	}
	return n
}

func (h *harness) accountType(acctID types.ID) account.Type {
	h.t.Helper()
	a, err := h.accountRepo.GetByID(acctID)
	if err != nil {
		h.t.Fatalf("load account: %v", err)
	}
	return a.Type
}

func TestPlanAccountTypeChange_SameLedgerIsEmpty(t *testing.T) {
	h := newHarness(t)
	h.seedCashOnlyHSA()

	plan, err := h.svc.PlanAccountTypeChange(h.hsa.ID, account.TypeInvestment)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.CrossesLedger() || plan.MovesRows() || plan.Total != 0 {
		t.Errorf("hsa_investment → investment should not cross ledgers: %+v", plan)
	}

	plan, err = h.svc.PlanAccountTypeChange(h.checking.ID, account.TypeSavings)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.CrossesLedger() {
		t.Errorf("checking → savings should not cross ledgers: %+v", plan)
	}
}

func TestPlanAccountTypeChange_CashOnlyCounts(t *testing.T) {
	h := newHarness(t)
	h.seedCashOnlyHSA()

	plan, err := h.svc.PlanAccountTypeChange(h.hsa.ID, account.TypeHSA)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !plan.CrossesLedger() || plan.FromLedger != LedgerInvestment || plan.ToLedger != LedgerRegular {
		t.Fatalf("plan ledgers = %s → %s, want investment → regular", plan.FromLedger, plan.ToLedger)
	}
	want := map[investment.TransactionType]int{
		investment.TransactionTypeDeposit:      1,
		investment.TransactionTypeWithdrawal:   1,
		investment.TransactionTypeInterest:     1,
		investment.TransactionTypeFee:          1,
		investment.TransactionTypeTransferCash: 3, // in, out, paycheck line
	}
	if !sameCounts(plan.RowsByType, want) || plan.Total != 7 {
		t.Errorf("RowsByType = %v (total %d), want %v (7)", plan.RowsByType, plan.Total, want)
	}
	if got := plan.SortedTypes(); len(got) != 5 || got[0] != investment.TransactionTypeDeposit {
		t.Errorf("SortedTypes = %v", got)
	}
}

func TestChangeAccountType_MovesCashRowsAndKeepsLinks(t *testing.T) {
	h := newHarness(t)
	seed := h.seedCashOnlyHSA()
	// Cash: +1000 −180.25 +0.12 −2 +250 −600 +50 = 517.87
	before, err := h.accountRepo.BalanceAsOf(h.checking.ID, testDate())
	if err != nil {
		t.Fatal(err)
	}

	plan, err := h.svc.PlanAccountTypeChange(h.hsa.ID, account.TypeHSA)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := h.svc.ChangeAccountType(plan); err != nil {
		t.Fatalf("change: %v", err)
	}

	if got := h.accountType(h.hsa.ID); got != account.TypeHSA {
		t.Errorf("type = %s, want hsa", got)
	}
	if n := h.invCount(h.hsa.ID); n != 0 {
		t.Errorf("investment rows left behind: %d", n)
	}
	if n := h.regCount(h.hsa.ID); n != 7 {
		t.Errorf("register rows = %d, want 7", n)
	}

	// Field mapping on the withdrawal: same id, signed amount, memo, uncleared.
	w, err := h.txnRepo.GetByID(seed.withdrawalID)
	if err != nil {
		t.Fatalf("moved withdrawal not found by its original id: %v", err)
	}
	if !w.Amount.Equal(types.MustNewMoney("-180.25")) || w.Memo.String != "clinic visit" || w.Status != transaction.StatusUncleared {
		t.Errorf("moved withdrawal = amount %s memo %q status %s", w.Amount, w.Memo.String, w.Status)
	}
	if w.CategoryID.Valid || w.PayeeID.Valid || w.TransferID.Valid {
		t.Errorf("moved withdrawal should carry no category, payee or transfer: %+v", w)
	}

	// Balance follows the rows into the register.
	bal, err := h.accountRepo.BalanceAsOf(h.hsa.ID, testDate())
	if err != nil {
		t.Fatal(err)
	}
	if !bal.Equal(types.MustNewMoney("517.87")) {
		t.Errorf("register balance = %s, want 517.87", bal)
	}
	// And the other side of every transfer is untouched.
	after, err := h.accountRepo.BalanceAsOf(h.checking.ID, testDate())
	if err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Errorf("checking balance moved: %s → %s", before, after)
	}

	// The whole transfers now read with the new kinds and both legs intact.
	in, err := h.svc.Get(seed.inTransferID)
	if err != nil {
		t.Fatalf("read inbound transfer: %v", err)
	}
	if in.Kind != KindRegToReg || in.To.AccountID != h.hsa.ID {
		t.Errorf("checking → hsa reads as %s (to %s)", in.Kind, in.To.AccountID)
	}
	out, err := h.svc.Get(seed.outTransferID)
	if err != nil {
		t.Fatalf("read outbound transfer: %v", err)
	}
	if out.Kind != KindRegToInv || out.From.AccountID != h.hsa.ID {
		t.Errorf("hsa → brokerage reads as %s (from %s)", out.Kind, out.From.AccountID)
	}
	if n := h.invCount(h.brokerage.ID); n != 1 {
		t.Errorf("brokerage leg count = %d, want 1", n)
	}

	// The paycheck line's counterpart is now the register row the
	// transaction service looks for by transfer id.
	legs, err := h.txnRepo.ListByTransferID(seed.splitTransferID)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 1 || legs[0].AccountID != h.hsa.ID || !legs[0].Amount.Equal(types.MustNewMoney("50.00")) {
		t.Errorf("paycheck counterpart = acct %s (hsa %s) amount %s", legs[0].AccountID, h.hsa.ID, legs[0].Amount)
	}
}

func TestChangeAccountType_RefusesSecurityRows(t *testing.T) {
	h := newHarness(t)
	h.seedCashOnlyHSA()
	sec := security.NewSecurity("ACME", "Acme Index Fund", security.TypeETF)
	if err := h.securityRepo.Create(sec); err != nil {
		t.Fatal(err)
	}
	total := types.MustNewMoney("250.00")
	if _, err := h.invSvc.Buy(h.hsa.ID, sec.ID, testDate(), types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("buy: %v", err)
	}
	invBefore := h.invCount(h.hsa.ID)

	_, err := h.svc.PlanAccountTypeChange(h.hsa.ID, account.TypeHSA)
	var refused *LedgerMoveRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("plan error = %v, want LedgerMoveRefusedError", err)
	}
	if refused.Rows != 1 {
		t.Errorf("refused rows = %d, want 1 (the buy)", refused.Rows)
	}

	// A plan forged by hand is re-checked inside the transaction.
	forged := &LedgerMovePlan{AccountID: h.hsa.ID, From: account.TypeHSAInvestment, To: account.TypeHSA,
		FromLedger: LedgerInvestment, ToLedger: LedgerRegular, RowsByType: map[investment.TransactionType]int{}}
	if err := h.svc.ChangeAccountType(forged); !errors.As(err, &refused) {
		t.Fatalf("forged plan error = %v, want LedgerMoveRefusedError", err)
	}
	if h.invCount(h.hsa.ID) != invBefore || h.regCount(h.hsa.ID) != 0 || h.accountType(h.hsa.ID) != account.TypeHSAInvestment {
		t.Error("a refused change wrote something")
	}
}

func TestChangeAccountType_RefusesRegularWithRows(t *testing.T) {
	h := newHarness(t)
	h.seedRegToReg("100.00", types.NullableID{})

	_, err := h.svc.PlanAccountTypeChange(h.checking.ID, account.TypeInvestment)
	var refused *LedgerMoveRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("plan error = %v, want LedgerMoveRefusedError", err)
	}
	if h.accountType(h.checking.ID) != account.TypeChecking {
		t.Error("refused plan changed the type")
	}

	// An empty regular account may cross into the investment ledger.
	empty := h.newAccount("Empty", account.TypeChecking, "0.00", testDate())
	plan, err := h.svc.PlanAccountTypeChange(empty.ID, account.TypeHSAInvestment)
	if err != nil {
		t.Fatalf("plan empty: %v", err)
	}
	if !plan.CrossesLedger() || plan.MovesRows() {
		t.Errorf("empty plan = %+v", plan)
	}
	if err := h.svc.ChangeAccountType(plan); err != nil {
		t.Fatalf("change empty: %v", err)
	}
	if h.accountType(empty.ID) != account.TypeHSAInvestment {
		t.Error("empty account did not change type")
	}
}

func TestChangeAccountType_StalePlanIsRefused(t *testing.T) {
	h := newHarness(t)
	h.seedCashOnlyHSA()

	plan, err := h.svc.PlanAccountTypeChange(h.hsa.ID, account.TypeHSA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.invSvc.Deposit(h.hsa.ID, testDate(), types.MustNewMoney("5.00"), "late"); err != nil {
		t.Fatal(err)
	}

	err = h.svc.ChangeAccountType(plan)
	var stale *StalePlanError
	if !errors.As(err, &stale) {
		t.Fatalf("error = %v, want StalePlanError", err)
	}
	if h.invCount(h.hsa.ID) != 8 || h.regCount(h.hsa.ID) != 0 {
		t.Error("stale plan wrote something")
	}
}

// TestAccountServiceUpdate_GuardsLedgerChange pins the domain guard the move
// relies on: a plain Update cannot strand rows in the other ledger.
func TestAccountServiceUpdate_GuardsLedgerChange(t *testing.T) {
	h := newHarness(t)
	h.seedCashOnlyHSA()
	acctSvc := account.NewService(h.accountRepo, h.db)

	a, _ := h.accountRepo.GetByID(h.hsa.ID)
	a.Type = account.TypeHSA
	err := acctSvc.Update(a)
	var guard *account.LedgerChangeError
	if !errors.As(err, &guard) {
		t.Fatalf("Update error = %v, want LedgerChangeError", err)
	}
	if guard.Rows != 7 {
		t.Errorf("guard rows = %d, want 7", guard.Rows)
	}

	// Same-ledger change passes.
	a.Type = account.TypeInvestment
	if err := acctSvc.Update(a); err != nil {
		t.Errorf("same-ledger Update: %v", err)
	}
}
