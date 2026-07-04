package scheduled

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/alpacahq/alpacadecimal"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// createBalancedLoanSchedule builds a strictly loan-shaped schedule whose
// month-one snapshot is internally balanced (Σ lines == parent), so it passes
// generic multi-line validation and persists via svc.Create. interestCat = nil
// / escrow = "0" omit those lines.
func createBalancedLoanSchedule(
	t *testing.T,
	svc *Service,
	funding, loanAcct *account.Account,
	interestCat, escrowCat *category.Category,
	owed, apr, pi, escrow string,
	nextDate types.Date,
) *Transaction {
	t.Helper()
	interest, principal, _, err := loan.SplitPayment(
		types.MustNewMoney(owed), types.MustNewMoney(apr), types.MustNewMoney(pi))
	if err != nil {
		t.Fatalf("month-one SplitPayment: %v", err)
	}
	esc := types.MustNewMoney(escrow)

	var splits SplitCollection
	total := types.ZeroMoney
	if interestCat != nil && interest.IsPositive() {
		is := NewCategorizedSplit(types.NewID(), interestCat.ID, interest.Neg())
		is.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
		splits = append(splits, is)
		total = total.Add(interest.Neg())
	}
	ps := NewTransferSplit(types.NewID(), loanAcct.ID, principal.Neg())
	ps.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	splits = append(splits, ps)
	total = total.Add(principal.Neg())
	if !esc.IsZero() {
		es := NewCategorizedSplit(types.NewID(), escrowCat.ID, esc.Neg())
		es.LoanSection = types.NullableString{String: LoanSectionEscrow, Valid: true}
		splits = append(splits, es)
		total = total.Add(esc.Neg())
	}

	st := NewTransactionWithAmount(funding.ID, FrequencyMonthly, nextDate, total)
	st.SetDayOfMonth(int(nextDate.Time().Day()))
	for _, sp := range splits {
		sp.ScheduledTransactionID = st.ID
	}
	st.Splits = splits
	if err := svc.Create(st); err != nil {
		t.Fatalf("create loan schedule: %v", err)
	}
	return st
}

// splitByCategory returns the amount of the (single) split on txnID with the
// given category; zero if none exists.
func splitByCategory(t *testing.T, svc *Service, txnID, catID types.ID) types.Money {
	t.Helper()
	var m types.Money
	err := svc.db.Conn().QueryRow(
		`SELECT amount FROM transaction_splits
		 WHERE CAST(transaction_id AS VARCHAR) = ? AND CAST(category_id AS VARCHAR) = ?`,
		txnID.String(), catID.String()).Scan(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return types.ZeroMoney
	}
	if err != nil {
		t.Fatalf("query split by category: %v", err)
	}
	return m
}

func TestLoanPost_RecomputesFromLiveBalance(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, f.escrow,
		"250000.00", "6.5", "2401.86", "650.00", types.NewDate(2026, time.August, 1))

	txn, err := f.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post month 1: %v", err)
	}

	// Month-one interest line is exactly -1354.17.
	if got := splitByCategory(t, f.svc, txn.ID, f.interest.ID); !got.Equal(types.MustNewMoney("-1354.17")) {
		t.Errorf("interest split = %s, want -1354.17", got.String())
	}
	// Loan balance moved toward zero by the principal (1047.69).
	bal, err := f.accountRepo.Balance(loanAcct.ID)
	if err != nil {
		t.Fatalf("loan balance: %v", err)
	}
	if !bal.Equal(types.MustNewMoney("-248952.31")) {
		t.Errorf("loan balance after month 1 = %s, want -248952.31", bal.String())
	}
}

func TestLoanPost_PrincipalCategoryFlowsToBothLegs(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	// Any persisted non-system category stands in for Loan:Principal here.
	principalCatID := f.escrow.ID

	interest, principal, _, err := loan.SplitPayment(
		types.MustNewMoney("250000.00"), types.MustNewMoney("6.5"), types.MustNewMoney("2401.86"))
	if err != nil {
		t.Fatalf("SplitPayment: %v", err)
	}
	is := NewCategorizedSplit(types.NewID(), f.interest.ID, interest.Neg())
	is.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
	ps := NewTransferSplit(types.NewID(), loanAcct.ID, principal.Neg())
	ps.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	ps.CategoryID = types.NullableID{ID: principalCatID, Valid: true} // categorized transfer
	st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly,
		types.NewDate(2026, time.August, 1), interest.Neg().Add(principal.Neg()))
	st.SetDayOfMonth(1)
	is.ScheduledTransactionID = st.ID
	ps.ScheduledTransactionID = st.ID
	st.Splits = SplitCollection{is, ps}
	if err := f.svc.Create(st); err != nil {
		t.Fatalf("create categorized-principal loan schedule: %v", err)
	}

	txn, err := f.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}

	// The posted principal split (the funding-account transfer line) carries the
	// category — recompute-at-post copied it from the template.
	var splitCat string
	if err := f.svc.db.Conn().QueryRow(
		`SELECT CAST(category_id AS VARCHAR) FROM transaction_splits
		 WHERE CAST(transaction_id AS VARCHAR) = ? AND transfer_account_id IS NOT NULL`,
		txn.ID.String()).Scan(&splitCat); err != nil {
		t.Fatalf("query posted principal split: %v", err)
	}
	if splitCat != principalCatID.String() {
		t.Errorf("posted principal split category = %s, want %s", splitCat, principalCatID.String())
	}

	// The minted loan-account counterpart carries the same category (Phase 7
	// counterpart mirroring), so the payment's both legs are labeled.
	var count int
	if err := f.svc.db.Conn().QueryRow(
		`SELECT COUNT(*) FROM transactions
		 WHERE CAST(account_id AS VARCHAR) = ? AND CAST(category_id AS VARCHAR) = ?`,
		loanAcct.ID.String(), principalCatID.String()).Scan(&count); err != nil {
		t.Fatalf("query loan-account counterpart: %v", err)
	}
	if count != 1 {
		t.Errorf("loan-account counterpart rows carrying the category = %d, want 1", count)
	}
}

func TestLoanPost_MultiMonthInterestFalls(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, f.escrow,
		"250000.00", "6.5", "2401.86", "650.00", types.NewDate(2026, time.August, 1))

	m1, err := f.svc.PostWithDate(st.ID, types.NewDate(2026, time.August, 1), nil)
	if err != nil {
		t.Fatalf("post month 1: %v", err)
	}
	m2, err := f.svc.PostWithDate(st.ID, types.NewDate(2026, time.September, 1), nil)
	if err != nil {
		t.Fatalf("post month 2: %v", err)
	}

	int1 := splitByCategory(t, f.svc, m1.ID, f.interest.ID)
	int2 := splitByCategory(t, f.svc, m2.ID, f.interest.ID)
	// Interest lines are negative; a smaller balance means a smaller magnitude,
	// i.e. int2 is closer to zero (int2 > int1 as signed values).
	if int2.Cmp(int1) <= 0 {
		t.Errorf("month-2 interest %s should be closer to zero than month-1 %s", int2.String(), int1.String())
	}
	// Both months draft the same fixed total (non-final): -3051.86 each.
	if !m1.Amount.Equal(types.MustNewMoney("-3051.86")) || !m2.Amount.Equal(types.MustNewMoney("-3051.86")) {
		t.Errorf("parent drafts = %s / %s, want -3051.86 each", m1.Amount.String(), m2.Amount.String())
	}
}

func TestLoanPost_ExtraPrincipalLowersNextInterest(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, f.escrow,
		"250000.00", "6.5", "2401.86", "650.00", types.NewDate(2026, time.August, 1))

	baseline, err := f.svc.ComputeLoanSplits(st, types.NewDate(2026, time.September, 1))
	if err != nil {
		t.Fatalf("baseline compute: %v", err)
	}

	// A one-off extra-principal transfer into the loan, dated before the Sept
	// occurrence, should reduce the recomputed interest for that occurrence.
	insertLoanTxn(t, f.svc, loanAcct.ID, types.NewDate(2026, time.August, 15), "50000.00")

	after, err := f.svc.ComputeLoanSplits(st, types.NewDate(2026, time.September, 1))
	if err != nil {
		t.Fatalf("after-extra compute: %v", err)
	}
	if after.Interest.Cmp(baseline.Interest) >= 0 {
		t.Errorf("extra principal did not lower interest: baseline %s, after %s",
			baseline.Interest.String(), after.Interest.String())
	}
}

func TestLoanPost_FinalPaymentClampsAndCompletes(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "1000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, nil,
		"1000.00", "6.5", "2401.86", "0", types.NewDate(2026, time.August, 1))

	txn, err := f.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("post final: %v", err)
	}
	// Draft clamps to interest(5.42) + principal(1000) = -1005.42.
	if !txn.Amount.Equal(types.MustNewMoney("-1005.42")) {
		t.Errorf("final draft = %s, want -1005.42", txn.Amount.String())
	}
	// Loan is now at zero.
	bal, _ := f.accountRepo.Balance(loanAcct.ID)
	if !bal.IsZero() {
		t.Errorf("loan balance after final = %s, want 0", bal.String())
	}
	// Schedule marked completed.
	got, err := f.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("reload schedule: %v", err)
	}
	if !got.IsCompleted() {
		t.Error("schedule should be completed after final payment")
	}
}

func TestLoanPost_PaidOffRefusalMarksCompleted(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "0.00", "6.5") // already paid off
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, nil,
		"1.00", "6.5", "2401.86", "0", types.NewDate(2026, time.August, 1))

	_, err := f.svc.Post(st.ID, nil)
	if !errors.Is(err, ErrLoanPaidOff) {
		t.Fatalf("expected ErrLoanPaidOff, got %v", err)
	}
	got, err := f.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.IsCompleted() {
		t.Error("a paid-off refusal must mark the schedule completed")
	}
}

func TestLoanPostWithEdits_PayoffCompletes(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "1000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, nil,
		"1000.00", "6.5", "2401.86", "0", types.NewDate(2026, time.August, 1))

	// Simulate the preview submitting an edited final payment that pays the loan
	// off (principal transfer of the full balance).
	parent := transaction.NewTransaction(f.funding.ID, types.NewDate(2026, time.August, 1), types.MustNewMoney("-1005.42"))
	interestSplit := transaction.NewSplit(types.ID{}, f.interest.ID, types.MustNewMoney("-5.42"))
	principalSplit := &transaction.Split{
		BaseModel:         types.NewBaseModel(),
		Amount:            types.MustNewMoney("-1000.00"),
		TransferAccountID: types.NullableID{ID: loanAcct.ID, Valid: true},
	}
	_, err := f.svc.PostWithEdits(st.ID, parent, []*transaction.Split{interestSplit, principalSplit})
	if err != nil {
		t.Fatalf("PostWithEdits: %v", err)
	}
	got, err := f.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.IsCompleted() {
		t.Error("PostWithEdits that pays the loan off must complete the schedule")
	}
}

func TestLoanAutoPost_SkipReasonContinuesBatch(t *testing.T) {
	f := newLoanShapeFixture(t)
	paidOffLoan := makeLoanAccount(t, f.accountRepo, "PaidMtg", "0.00", "6.5")

	// A due, auto-post, paid-off loan schedule.
	loanSched := createBalancedLoanSchedule(t, f.svc, f.funding, paidOffLoan, f.interest, nil,
		"1.00", "6.5", "2401.86", "0", types.NewDate(2026, time.June, 1))
	loanSched.SetAutoPost(true)
	if err := f.svc.Update(loanSched); err != nil {
		t.Fatalf("enable auto-post on loan schedule: %v", err)
	}

	// A due, auto-post, ordinary single-line schedule that must still post.
	normal := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.June, 1), types.MustNewMoney("-100.00"))
	normal.SetDayOfMonth(1)
	normal.SetAutoPost(true)
	if err := f.svc.Create(normal); err != nil {
		t.Fatalf("create normal schedule: %v", err)
	}

	summary, err := f.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost must not abort the batch: %v", err)
	}

	// The ordinary schedule posted at least once.
	if summary.PostedCount == 0 {
		t.Error("expected the ordinary schedule to auto-post despite the paid-off loan")
	}
	// The loan schedule was skipped with a reason and marked completed.
	var sawLoanSkip bool
	for _, r := range summary.Results {
		if r.ScheduledTransactionID == loanSched.ID {
			sawLoanSkip = r.Skipped && r.SkipReason == "loan is paid off"
		}
	}
	if !sawLoanSkip {
		t.Error("expected the paid-off loan schedule to be skipped with reason 'loan is paid off'")
	}
	got, _ := f.svc.GetByID(loanSched.ID)
	if !got.IsCompleted() {
		t.Error("paid-off loan schedule should be marked completed by auto-post")
	}
}

func TestLoanAutoPost_MultiOverdueCompounds(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	// Due several months back so multiple occurrences post in one pass.
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, f.escrow,
		"250000.00", "6.5", "2401.86", "650.00", types.NewDate(2026, time.April, 1))
	st.SetAutoPost(true)
	if err := f.svc.Update(st); err != nil {
		t.Fatalf("enable auto-post: %v", err)
	}

	summary, err := f.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}
	if summary.PostedCount < 2 {
		t.Fatalf("expected multiple overdue occurrences, posted %d", summary.PostedCount)
	}
	// After N compounding payments the loan balance dropped by MORE than N
	// months of the first month's principal — because interest fell (and so
	// principal rose) as the balance shrank. If the split were stale, each
	// month would pay the same 1047.69 and pay-down would equal N × 1047.69.
	bal, _ := f.accountRepo.Balance(loanAcct.ID)
	paidDown := types.MustNewMoney("-250000.00").Sub(bal).Neg() // positive magnitude paid down
	firstPrincipal := types.MustNewMoney("1047.69")
	staleTotal := firstPrincipal.Mul(alpacadecimal.NewFromInt(int64(summary.PostedCount)))
	if paidDown.Cmp(staleTotal) <= 0 {
		t.Errorf("compounded pay-down %s should exceed %d × first-month principal (%s)",
			paidDown.String(), summary.PostedCount, staleTotal.String())
	}
}

func TestLoanPost_ZeroRateNoInterestLine(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Car", "1200.00", "0")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, nil, nil,
		"1200.00", "0", "100.00", "0", types.NewDate(2026, time.August, 1))

	txn, err := f.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("post 0%% loan: %v", err)
	}
	// Full payment books as principal; no interest split exists.
	if got := splitByCategory(t, f.svc, txn.ID, f.interest.ID); !got.IsZero() {
		t.Errorf("unexpected interest split %s on a 0%% loan", got.String())
	}
	bal, _ := f.accountRepo.Balance(loanAcct.ID)
	if !bal.Equal(types.MustNewMoney("-1100.00")) {
		t.Errorf("loan balance = %s, want -1100.00", bal.String())
	}
}

func TestLoanPost_NullAPRTypedError(t *testing.T) {
	f := newLoanShapeFixture(t)
	loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
	st := createBalancedLoanSchedule(t, f.svc, f.funding, loanAcct, f.interest, nil,
		"250000.00", "6.5", "2401.86", "0", types.NewDate(2026, time.August, 1))

	// Clear the APR after creation (simulating an account edit).
	loanAcct.ClearInterestRate()
	if err := f.accountRepo.Update(loanAcct); err != nil {
		t.Fatalf("clear APR: %v", err)
	}

	_, err := f.svc.Post(st.ID, nil)
	if !errors.Is(err, ErrLoanNoInterestRate) {
		t.Errorf("expected ErrLoanNoInterestRate, got %v", err)
	}
}

func TestGenericMultiLineUnaffected(t *testing.T) {
	f := newLoanShapeFixture(t)
	// A paycheck-shaped (no loan_section) multi-line schedule must post verbatim.
	net := types.MustNewMoney("900.00")
	st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), net)
	st.SetDayOfMonth(1)
	gross := types.MustNewMoney("1000.00")
	tax := types.MustNewMoney("-100.00")
	st.Splits = SplitCollection{
		NewCategorizedSplit(st.ID, f.interest.ID, gross),
		NewCategorizedSplit(st.ID, f.escrow.ID, tax),
	}
	if err := f.svc.Create(st); err != nil {
		t.Fatalf("create generic multi-line: %v", err)
	}
	txn, err := f.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("post generic multi-line: %v", err)
	}
	if !txn.Amount.Equal(net) {
		t.Errorf("generic parent draft = %s, want %s (verbatim)", txn.Amount.String(), net.String())
	}
	if got := splitByCategory(t, f.svc, txn.ID, f.interest.ID); !got.Equal(gross) {
		t.Errorf("generic split posted %s, want verbatim %s", got.String(), gross.String())
	}
}
