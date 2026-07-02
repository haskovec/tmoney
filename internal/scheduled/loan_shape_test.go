package scheduled

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/types"
)

// loanShapeFixture builds a funding (checking) account, an active loan account
// (with an APR), and a Loan:Interest expense category for loan-shape tests.
type loanShapeFixture struct {
	svc         *Service
	accountRepo *account.Repository
	funding     *account.Account
	loan        *account.Account
	interest    *category.Category
	escrow      *category.Category
}

func newLoanShapeFixture(t *testing.T) loanShapeFixture {
	t.Helper()
	svc, accountRepo, _, categoryRepo := createTestScheduledTransactionService(t)

	funding := createTestAccountForScheduled(t, accountRepo, "Checking")

	loan := account.NewAccount("Mortgage", account.TypeLoan, "USD",
		types.MustNewMoney("-250000.00"), types.NewDate(2024, time.January, 1))
	loan.SetInterestRate(types.MustNewMoney("6.5"))
	if err := accountRepo.Create(loan); err != nil {
		t.Fatalf("create loan account: %v", err)
	}

	interest := category.NewCategory("Interest", category.TypeExpense)
	if err := categoryRepo.Create(interest); err != nil {
		t.Fatalf("create interest category: %v", err)
	}
	escrow := category.NewCategory("Property Tax", category.TypeExpense)
	if err := categoryRepo.Create(escrow); err != nil {
		t.Fatalf("create escrow category: %v", err)
	}

	return loanShapeFixture{svc: svc, accountRepo: accountRepo, funding: funding, loan: loan, interest: interest, escrow: escrow}
}

// buildLoanSchedule returns a strictly loan-shaped (interest + principal +
// escrow) monthly schedule on the funding account. The parent amount is the
// signed sum of the three lines. Lines are tagged with loan_section.
func (f loanShapeFixture) buildLoanSchedule(t *testing.T) *Transaction {
	t.Helper()
	interest := types.MustNewMoney("-1354.17")
	principal := types.MustNewMoney("-1047.69")
	escrow := types.MustNewMoney("-650.00")
	total := interest.Add(principal).Add(escrow)

	st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), total)
	st.SetDayOfMonth(1)

	interestSplit := NewCategorizedSplit(st.ID, f.interest.ID, interest)
	interestSplit.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
	principalSplit := NewTransferSplit(st.ID, f.loan.ID, principal)
	principalSplit.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	escrowSplit := NewCategorizedSplit(st.ID, f.escrow.ID, escrow)
	escrowSplit.LoanSection = types.NullableString{String: LoanSectionEscrow, Valid: true}

	st.Splits = SplitCollection{interestSplit, principalSplit, escrowSplit}
	return st
}

func TestIsLoanShaped(t *testing.T) {
	f := newLoanShapeFixture(t)
	lookup := f.svc.loanAccountLookup()

	t.Run("well-formed loan schedule is loan-shaped", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		if !IsLoanShaped(st, lookup) {
			t.Error("expected a well-formed interest+principal+escrow schedule to be loan-shaped")
		}
	})

	t.Run("zero-rate loan without an interest line is loan-shaped", func(t *testing.T) {
		principal := types.MustNewMoney("-500.00")
		st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), principal)
		st.SetDayOfMonth(1)
		ps := NewTransferSplit(st.ID, f.loan.ID, principal)
		ps.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
		st.Splits = SplitCollection{ps}
		if !IsLoanShaped(st, lookup) {
			t.Error("expected a principal-only (zero-rate) schedule to be loan-shaped")
		}
	})

	t.Run("untagged split disqualifies", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		st.Splits[0].LoanSection = types.NullableString{Valid: false}
		if IsLoanShaped(st, lookup) {
			t.Error("a single untagged split must disqualify the schedule")
		}
	})

	t.Run("non-monthly cadence disqualifies", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		st.Frequency = FrequencyQuarterly
		if IsLoanShaped(st, lookup) {
			t.Error("a quarterly schedule must not be loan-shaped")
		}
	})

	t.Run("interval != 1 disqualifies", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		st.Interval = 2
		if IsLoanShaped(st, lookup) {
			t.Error("a bi-monthly (interval 2) schedule must not be loan-shaped")
		}
	})

	t.Run("secondary day disqualifies", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		st.SecondaryDayOfMonth = types.NullableInt{Int64: 15, Valid: true}
		if IsLoanShaped(st, lookup) {
			t.Error("a semi-monthly-flavored schedule must not be loan-shaped")
		}
	})

	t.Run("two principal lines disqualify", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		extra := NewTransferSplit(st.ID, f.loan.ID, types.MustNewMoney("-1.00"))
		extra.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
		st.Splits = append(st.Splits, extra)
		if IsLoanShaped(st, lookup) {
			t.Error("more than one principal line must disqualify")
		}
	})

	t.Run("two interest lines disqualify", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		extra := NewCategorizedSplit(st.ID, f.interest.ID, types.MustNewMoney("-1.00"))
		extra.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
		st.Splits = append(st.Splits, extra)
		if IsLoanShaped(st, lookup) {
			t.Error("more than one interest line must disqualify")
		}
	})

	t.Run("principal targeting a non-loan account disqualifies", func(t *testing.T) {
		// Point the principal transfer at the funding (checking) account type.
		st := f.buildLoanSchedule(t)
		other := createTestAccountForScheduled(t, f.accountRepo, "Savings")
		st.Splits[1] = NewTransferSplit(st.ID, other.ID, types.MustNewMoney("-1047.69"))
		st.Splits[1].LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
		if IsLoanShaped(st, lookup) {
			t.Error("a principal transfer to a non-loan account must disqualify")
		}
	})

	t.Run("principal targeting a closed loan account disqualifies", func(t *testing.T) {
		f2 := newLoanShapeFixture(t)
		f2.loan.Close(types.NewDate(2026, time.July, 1))
		if err := f2.accountRepo.Update(f2.loan); err != nil {
			t.Fatalf("close loan: %v", err)
		}
		st := f2.buildLoanSchedule(t)
		if IsLoanShaped(st, f2.svc.loanAccountLookup()) {
			t.Error("a principal transfer to a closed loan account must disqualify")
		}
	})

	t.Run("variable-amount schedule disqualifies", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		st.ClearAmount()
		if IsLoanShaped(st, lookup) {
			t.Error("a variable-amount schedule must not be loan-shaped")
		}
	})

	t.Run("nil and empty are not loan-shaped", func(t *testing.T) {
		if IsLoanShaped(nil, lookup) {
			t.Error("nil schedule must not be loan-shaped")
		}
		empty := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.Today(), types.MustNewMoney("-1.00"))
		if IsLoanShaped(empty, lookup) {
			t.Error("a schedule with no splits must not be loan-shaped")
		}
	})
}

func TestIsLoanAdoptable(t *testing.T) {
	f := newLoanShapeFixture(t)
	lookup := f.svc.loanAccountLookup()

	t.Run("tagged loan schedule is adoptable", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		if !IsLoanAdoptable(st, lookup) {
			t.Error("a tagged loan schedule should be adoptable")
		}
	})

	t.Run("untagged hand-built loan schedule is adoptable", func(t *testing.T) {
		// Same shape but with NO loan_section tags — the adoption path.
		st := f.buildLoanSchedule(t)
		for _, sp := range st.Splits {
			sp.LoanSection = types.NullableString{Valid: false}
		}
		if !IsLoanAdoptable(st, lookup) {
			t.Error("an untagged monthly schedule with one loan transfer should be adoptable")
		}
	})

	t.Run("two transfer lines are not adoptable", func(t *testing.T) {
		st := f.buildLoanSchedule(t)
		extra := NewTransferSplit(st.ID, f.loan.ID, types.MustNewMoney("-1.00"))
		st.Splits = append(st.Splits, extra)
		if IsLoanAdoptable(st, lookup) {
			t.Error("more than one transfer line must not be adoptable")
		}
	})

	t.Run("no loan transfer is not adoptable", func(t *testing.T) {
		net := types.MustNewMoney("-100.00")
		st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.Today(), net)
		st.Splits = SplitCollection{NewCategorizedSplit(st.ID, f.interest.ID, net)}
		if IsLoanAdoptable(st, lookup) {
			t.Error("a schedule with no loan transfer line must not be adoptable")
		}
	})
}
