package scheduled

import (
	"errors"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/types"
)

// makeLoanAccount creates an active loan account with the given owed magnitude
// (stored negative) and APR (pass a negative apr sentinel to leave it NULL).
func makeLoanAccount(t *testing.T, repo *account.Repository, name, owed, apr string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, account.TypeLoan, "USD",
		types.MustNewMoney(owed).Neg(), types.NewDate(2024, time.January, 1))
	if apr != "" {
		acct.SetInterestRate(types.MustNewMoney(apr))
	}
	if err := repo.Create(acct); err != nil {
		t.Fatalf("create loan account %q: %v", name, err)
	}
	return acct
}

// insertLoanTxn inserts a raw transaction row into an account (same-package
// access to the service's db handle keeps the test independent of the
// transaction service).
func insertLoanTxn(t *testing.T, svc *Service, acctID types.ID, date types.Date, amount string) {
	t.Helper()
	_, err := svc.db.Conn().Exec(
		`INSERT INTO transactions (id, account_id, date, amount, status) VALUES (?, ?, ?, ?, ?)`,
		types.NewID(), acctID, date, types.MustNewMoney(amount), "cleared",
	)
	if err != nil {
		t.Fatalf("insert loan txn: %v", err)
	}
}

// buildTaggedLoanSchedule assembles a loan-shaped schedule on funding whose
// parent draft magnitude is (piPayment + escrow). Pass interestCat = nil to
// omit the interest line (a 0% / principal-only shape).
func buildTaggedLoanSchedule(
	t *testing.T,
	funding, loanAcct *account.Account,
	interestCat, escrowCat *category.Category,
	piPayment, escrow string,
) *Transaction {
	t.Helper()
	pi := types.MustNewMoney(piPayment)
	esc := types.MustNewMoney(escrow)
	// Month-one snapshot amounts are placeholders; ComputeLoanSplits derives
	// P&I from parent magnitude − escrow, not from these. Keep them balanced so
	// the schedule could also round-trip through Create.
	total := pi.Add(esc).Neg()
	st := NewTransactionWithAmount(funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), total)
	st.SetDayOfMonth(1)

	var splits SplitCollection
	if interestCat != nil {
		is := NewCategorizedSplit(st.ID, interestCat.ID, types.MustNewMoney("-1.00"))
		is.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
		splits = append(splits, is)
	}
	ps := NewTransferSplit(st.ID, loanAcct.ID, pi.Neg())
	ps.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	splits = append(splits, ps)
	if !esc.IsZero() {
		es := NewCategorizedSplit(st.ID, escrowCat.ID, esc.Neg())
		es.LoanSection = types.NullableString{String: LoanSectionEscrow, Valid: true}
		splits = append(splits, es)
	}
	st.Splits = splits
	return st
}

func TestComputeLoanSplits(t *testing.T) {
	aug1 := types.NewDate(2026, time.August, 1)

	t.Run("interest/principal split from live balance with escrow pass-through", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, f.escrow, "2401.86", "650.00")

		ls, err := f.svc.ComputeLoanSplits(st, aug1)
		if err != nil {
			t.Fatalf("ComputeLoanSplits: %v", err)
		}
		// interest = round(250000 * 6.5 / 1200, 2) = 1354.17
		if !ls.Interest.Equal(types.MustNewMoney("1354.17")) {
			t.Errorf("interest = %s, want 1354.17", ls.Interest.String())
		}
		// principal = 2401.86 - 1354.17 = 1047.69
		if !ls.Principal.Equal(types.MustNewMoney("1047.69")) {
			t.Errorf("principal = %s, want 1047.69", ls.Principal.String())
		}
		if !ls.EscrowTotal.Equal(types.MustNewMoney("650.00")) {
			t.Errorf("escrow total = %s, want 650.00", ls.EscrowTotal.String())
		}
		// parent draft = -(1354.17 + 1047.69 + 650.00) = -3051.86
		if !ls.ParentAmount.Equal(types.MustNewMoney("-3051.86")) {
			t.Errorf("parent = %s, want -3051.86", ls.ParentAmount.String())
		}
		if ls.Final {
			t.Error("first payment should not be final")
		}
		if len(ls.Splits) != 3 {
			t.Fatalf("splits = %d, want 3", len(ls.Splits))
		}
		// Parent amount must equal the signed sum of the computed lines.
		sum := types.ZeroMoney
		for _, sp := range ls.Splits {
			sum = sum.Add(sp.Amount)
		}
		if !sum.Equal(ls.ParentAmount) {
			t.Errorf("split sum %s != parent %s", sum.String(), ls.ParentAmount.String())
		}
	})

	t.Run("balance drop lowers interest and raises principal", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, f.escrow, "2401.86", "650.00")

		first, err := f.svc.ComputeLoanSplits(st, aug1)
		if err != nil {
			t.Fatalf("first ComputeLoanSplits: %v", err)
		}

		// Simulate the principal payment posting into the loan (+principal).
		insertLoanTxn(t, f.svc, loanAcct.ID, aug1, first.Principal.String())

		second, err := f.svc.ComputeLoanSplits(st, types.NewDate(2026, time.September, 1))
		if err != nil {
			t.Fatalf("second ComputeLoanSplits: %v", err)
		}
		if second.Interest.Cmp(first.Interest) >= 0 {
			t.Errorf("interest did not fall: first %s, second %s", first.Interest.String(), second.Interest.String())
		}
		if second.Principal.Cmp(first.Principal) <= 0 {
			t.Errorf("principal did not rise: first %s, second %s", first.Principal.String(), second.Principal.String())
		}
		// P&I fixed, so the non-final parent draft is unchanged month to month.
		if !second.ParentAmount.Equal(first.ParentAmount) {
			t.Errorf("parent changed: first %s, second %s", first.ParentAmount.String(), second.ParentAmount.String())
		}
	})

	t.Run("final payment clamps principal to owed", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "1000.00", "6.5")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, nil, "2401.86", "0")

		ls, err := f.svc.ComputeLoanSplits(st, aug1)
		if err != nil {
			t.Fatalf("ComputeLoanSplits: %v", err)
		}
		if !ls.Final {
			t.Error("expected final payment")
		}
		if !ls.Principal.Equal(types.MustNewMoney("1000.00")) {
			t.Errorf("principal = %s, want 1000.00 (clamped to owed)", ls.Principal.String())
		}
		// interest = round(1000 * 6.5/1200, 2) = 5.42; parent = -(5.42+1000)
		if !ls.ParentAmount.Equal(types.MustNewMoney("-1005.42")) {
			t.Errorf("parent = %s, want -1005.42", ls.ParentAmount.String())
		}
	})

	t.Run("zero-rate loan books full payment as principal, no interest line", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Car", "500.00", "0")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, nil, nil, "100.00", "0")

		ls, err := f.svc.ComputeLoanSplits(st, aug1)
		if err != nil {
			t.Fatalf("ComputeLoanSplits: %v", err)
		}
		if !ls.Interest.IsZero() {
			t.Errorf("interest = %s, want 0", ls.Interest.String())
		}
		if !ls.Principal.Equal(types.MustNewMoney("100.00")) {
			t.Errorf("principal = %s, want 100.00", ls.Principal.String())
		}
		if len(ls.Splits) != 1 {
			t.Fatalf("splits = %d, want 1 (principal only)", len(ls.Splits))
		}
	})

	t.Run("nearly-paid loan omits a zero interest line", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		// owed 0.01 @ 6.5% -> interest rounds to 0.00.
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "0.01", "6.5")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, nil, "2401.86", "0")

		ls, err := f.svc.ComputeLoanSplits(st, aug1)
		if err != nil {
			t.Fatalf("ComputeLoanSplits: %v", err)
		}
		if !ls.Interest.IsZero() {
			t.Errorf("interest = %s, want 0", ls.Interest.String())
		}
		for _, sp := range ls.Splits {
			if sp.CategoryID == f.interest.ID {
				t.Error("a $0.00 interest line must be omitted from the posted splits")
			}
		}
		if !ls.Final {
			t.Error("a fully-covered small balance is the final payment")
		}
	})

	t.Run("missing interest line while interest > 0 is a typed error", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, nil, nil, "2401.86", "0") // no interest line

		_, err := f.svc.ComputeLoanSplits(st, aug1)
		if !errors.Is(err, ErrLoanMissingInterestLine) {
			t.Errorf("expected ErrLoanMissingInterestLine, got %v", err)
		}
	})

	t.Run("NULL APR is a typed error", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "") // no rate
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, nil, "2401.86", "0")

		_, err := f.svc.ComputeLoanSplits(st, aug1)
		if !errors.Is(err, ErrLoanNoInterestRate) {
			t.Errorf("expected ErrLoanNoInterestRate, got %v", err)
		}
	})

	t.Run("paid-off loan is a typed error", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "0.00", "6.5") // owed 0
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, nil, "2401.86", "0")

		_, err := f.svc.ComputeLoanSplits(st, aug1)
		if !errors.Is(err, ErrLoanPaidOff) {
			t.Errorf("expected ErrLoanPaidOff, got %v", err)
		}
	})

	t.Run("negative amortization is a typed error", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		loanAcct := makeLoanAccount(t, f.accountRepo, "Mtg", "250000.00", "6.5")
		// P&I of 1000 < month-one interest of 1354.17.
		st := buildTaggedLoanSchedule(t, f.funding, loanAcct, f.interest, nil, "1000.00", "0")

		_, err := f.svc.ComputeLoanSplits(st, aug1)
		if !errors.Is(err, loan.ErrNegativeAmortization) {
			t.Errorf("expected ErrNegativeAmortization, got %v", err)
		}
	})
}
