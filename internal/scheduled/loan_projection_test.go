package scheduled

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

func TestFindLoanSchedule(t *testing.T) {
	t.Run("returns the loan-shaped schedule targeting the loan account", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		st := f.buildLoanSchedule(t)
		if err := f.svc.Create(st); err != nil {
			t.Fatalf("create loan schedule: %v", err)
		}

		got, err := f.svc.FindLoanSchedule(f.loan.ID)
		if err != nil {
			t.Fatalf("FindLoanSchedule: %v", err)
		}
		if got == nil {
			t.Fatal("FindLoanSchedule returned nil; want the loan-shaped schedule")
		}
		if got.ID != st.ID {
			t.Errorf("FindLoanSchedule returned schedule %s; want %s", got.ID, st.ID)
		}
	})

	t.Run("returns nil when no schedule targets the loan account", func(t *testing.T) {
		f := newLoanShapeFixture(t)
		// No schedule created at all.
		got, err := f.svc.FindLoanSchedule(f.loan.ID)
		if err != nil {
			t.Fatalf("FindLoanSchedule: %v", err)
		}
		if got != nil {
			t.Errorf("FindLoanSchedule returned %v; want nil", got)
		}
	})

	t.Run("skips a generic (untagged) multi-line schedule referencing the loan", func(t *testing.T) {
		f := newLoanShapeFixture(t)

		// A multi-line schedule with a transfer line into the loan account but
		// no loan_section tags: loan-adoptable, not strictly loan-shaped, so
		// FindLoanSchedule must not return it.
		principal := types.MustNewMoney("-1000.00")
		other := types.MustNewMoney("-500.00")
		total := principal.Add(other)
		st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), total)
		st.SetDayOfMonth(1)
		st.Splits = SplitCollection{
			NewTransferSplit(st.ID, f.loan.ID, principal),
			NewCategorizedSplit(st.ID, f.interest.ID, other),
		}
		if err := f.svc.Create(st); err != nil {
			t.Fatalf("create generic schedule: %v", err)
		}

		got, err := f.svc.FindLoanSchedule(f.loan.ID)
		if err != nil {
			t.Fatalf("FindLoanSchedule: %v", err)
		}
		if got != nil {
			t.Errorf("FindLoanSchedule returned a generic schedule %v; want nil", got)
		}
	})

	t.Run("returns the loan-shaped one even alongside a generic schedule", func(t *testing.T) {
		f := newLoanShapeFixture(t)

		loanShaped := f.buildLoanSchedule(t)
		if err := f.svc.Create(loanShaped); err != nil {
			t.Fatalf("create loan schedule: %v", err)
		}

		principal := types.MustNewMoney("-1000.00")
		other := types.MustNewMoney("-500.00")
		generic := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.September, 1), principal.Add(other))
		generic.SetDayOfMonth(1)
		generic.Splits = SplitCollection{
			NewTransferSplit(generic.ID, f.loan.ID, principal),
			NewCategorizedSplit(generic.ID, f.interest.ID, other),
		}
		if err := f.svc.Create(generic); err != nil {
			t.Fatalf("create generic schedule: %v", err)
		}

		got, err := f.svc.FindLoanSchedule(f.loan.ID)
		if err != nil {
			t.Fatalf("FindLoanSchedule: %v", err)
		}
		if got == nil || got.ID != loanShaped.ID {
			t.Errorf("FindLoanSchedule = %v; want loan-shaped schedule %s", got, loanShaped.ID)
		}
	})
}

func TestLoanScheduleInputs(t *testing.T) {
	f := newLoanShapeFixture(t)
	st := f.buildLoanSchedule(t)

	// buildLoanSchedule: interest -1354.17, principal -1047.69, escrow -650.00,
	// parent draft -3051.86. P&I = |parent| - escrow = 3051.86 - 650 = 2401.86.
	piPayment, escrowTotal, dayOfMonth := LoanScheduleInputs(st)

	if want := types.MustNewMoney("2401.86"); !piPayment.Equal(want) {
		t.Errorf("piPayment = %s; want %s", piPayment, want)
	}
	if want := types.MustNewMoney("650.00"); !escrowTotal.Equal(want) {
		t.Errorf("escrowTotal = %s; want %s", escrowTotal, want)
	}
	if dayOfMonth != 1 {
		t.Errorf("dayOfMonth = %d; want 1", dayOfMonth)
	}
}

func TestLoanScheduleInputsFallsBackToNextDateDay(t *testing.T) {
	f := newLoanShapeFixture(t)
	st := f.buildLoanSchedule(t)
	// Clear the stored day-of-month so the derivation falls back to NextDate.
	st.DayOfMonth = types.NullableInt{}
	st.NextDate = types.NewDate(2026, time.August, 15)

	_, _, dayOfMonth := LoanScheduleInputs(st)
	if dayOfMonth != 15 {
		t.Errorf("dayOfMonth = %d; want 15 (NextDate day)", dayOfMonth)
	}
}

func TestLoanScheduleInputsNoEscrow(t *testing.T) {
	f := newLoanShapeFixture(t)

	// interest -1354.17, principal -1047.69, no escrow → P&I = 2401.86, escrow 0.
	interest := types.MustNewMoney("-1354.17")
	principal := types.MustNewMoney("-1047.69")
	total := interest.Add(principal)
	st := NewTransactionWithAmount(f.funding.ID, FrequencyMonthly, types.NewDate(2026, time.August, 1), total)
	st.SetDayOfMonth(1)
	interestSplit := NewCategorizedSplit(st.ID, f.interest.ID, interest)
	interestSplit.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
	principalSplit := NewTransferSplit(st.ID, f.loan.ID, principal)
	principalSplit.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	st.Splits = SplitCollection{interestSplit, principalSplit}

	piPayment, escrowTotal, _ := LoanScheduleInputs(st)
	if want := types.MustNewMoney("2401.86"); !piPayment.Equal(want) {
		t.Errorf("piPayment = %s; want %s", piPayment, want)
	}
	if !escrowTotal.IsZero() {
		t.Errorf("escrowTotal = %s; want 0", escrowTotal)
	}
}
