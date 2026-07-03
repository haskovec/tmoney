package scheduled

import "github.com/haskovec/tmoney/internal/types"

// FindLoanSchedule returns the loan-shaped schedule whose principal transfer
// line targets loanAcctID, or (nil, nil) when the loan account has no
// loan-shaped schedule.
//
// A loan payment schedule's own AccountID is the *funding* account (e.g.
// Checking), not the loan being paid down — the loan account is the target of
// the principal split's transfer — so callers cannot locate it with
// ListByAccount. This scans the schedules that reference loanAcctID (source,
// transfer destination, or transfer-line split target) and returns the first
// that is strictly loan-shaped (IsLoanShaped) with its principal line
// transferring into loanAcctID.
//
// Used by the amortization view and the CLI loan list/show commands to attach a
// loan account to its live payment schedule.
func (s *Service) FindLoanSchedule(loanAcctID types.ID) (*Transaction, error) {
	refs, err := s.ListReferencing(loanAcctID)
	if err != nil {
		return nil, err
	}
	for _, st := range refs {
		if !s.isLoanShaped(st) {
			continue
		}
		if id, ok := loanAccountID(st); ok && id == loanAcctID {
			return st, nil
		}
	}
	return nil, nil
}

// LoanScheduleInputs derives the amortization-projection inputs carried by a
// loan-shaped schedule: the P&I payment, the fixed escrow total (a positive
// magnitude), and the day-of-month anchor for internal/loan.Project.
//
// It reuses the same P&I derivation as ComputeLoanSplits — the parent draft
// magnitude minus the escrow-line magnitudes, never a stored field — so a
// projection matches exactly what posting will compute. dayOfMonth is the
// schedule's stored day-of-month when set (loan schedules always store one),
// falling back to the day component of NextDate. st must be loan-shaped.
func LoanScheduleInputs(st *Transaction) (piPayment, escrowTotal types.Money, dayOfMonth int) {
	sec := sectionsOf(st)
	escrowTotal = sec.escrowTotal
	piPayment = st.Amount.Money.Abs().Sub(escrowTotal)
	dayOfMonth = st.NextDate.Day()
	if st.DayOfMonth.Valid {
		dayOfMonth = int(st.DayOfMonth.Int64)
	}
	return piPayment, escrowTotal, dayOfMonth
}
