package scheduled

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// LoanSection tags a scheduled-split line's role in a loan payment. Stored in
// scheduled_split_items.loan_section (migration 028) and mutually exclusive
// with paycheck_section. A loan-shaped schedule tags every line.
const (
	LoanSectionInterest  = "interest"
	LoanSectionPrincipal = "principal"
	LoanSectionEscrow    = "escrow"
)

// AccountLookup resolves an account by ID. Loan-shape detection needs it to
// verify a principal transfer line targets an active loan-type account. The
// scheduled service backs it with account.Repository.GetByID; the TUI supplies
// one from the account service. It must resolve closed loan accounts too, so
// callers should not source it from a list restricted to active accounts.
type AccountLookup func(types.ID) (*account.Account, error)

// isLoanTarget reports whether lookup resolves id to an active loan-type
// account. A resolve failure (missing account, DB error) or a nil account
// yields false, keeping detection a total function (matching the bool-only
// contract of the paycheck heuristic looksLikePaycheck).
func isLoanTarget(lookup AccountLookup, id types.ID) bool {
	if lookup == nil {
		return false
	}
	acct, err := lookup(id)
	if err != nil || acct == nil {
		return false
	}
	return acct.Active && acct.Type == account.TypeLoan
}

// isMonthlyLoanCadence is the cadence gate shared by IsLoanShaped and
// IsLoanAdoptable: monthly frequency, interval 1, and no semi-monthly
// secondary day. Any other cadence would book a full month's interest at the
// wrong rhythm, so it disqualifies a schedule from loan treatment.
func isMonthlyLoanCadence(st *Transaction) bool {
	return st.Frequency == FrequencyMonthly &&
		st.Interval == 1 &&
		!st.SecondaryDayOfMonth.Valid
}

// loanAccountLookup returns an AccountLookup backed by the service's account
// repository (nil when no repo is wired, as in some fixtures — detection then
// treats every schedule as non-loan). GetByID resolves closed accounts too.
func (s *Service) loanAccountLookup() AccountLookup {
	if s.accountRepo == nil {
		return nil
	}
	return s.accountRepo.GetByID
}

// isLoanShaped reports whether st is loan-shaped, using the service's account
// repository to resolve the principal transfer target.
func (s *Service) isLoanShaped(st *Transaction) bool {
	return IsLoanShaped(st, s.loanAccountLookup())
}

// IsLoanShaped reports whether st is a strictly loan-shaped schedule — and so
// gets recompute-at-post, the final-payment clamp, payoff completion, and the
// loan affordances. All of the following must hold:
//
//   - multi-line with a fixed parent amount;
//   - monthly frequency, interval 1, no semi-monthly secondary day;
//   - every split carries a non-NULL loan_section;
//   - exactly one principal split, a transfer line to an active loan account;
//   - at most one interest split (categorized; absent only for 0% loans);
//   - zero or more escrow splits (categorized).
//
// Anything else is a generic multi-line schedule whose template posts verbatim.
func IsLoanShaped(st *Transaction, lookup AccountLookup) bool {
	if st == nil || len(st.Splits) == 0 || !st.HasAmount() {
		return false
	}
	if !isMonthlyLoanCadence(st) {
		return false
	}

	var principalCount, interestCount int
	var principalTarget types.ID
	principalIsTransfer := false
	for _, sp := range st.Splits {
		if sp == nil || !sp.LoanSection.Valid {
			return false
		}
		switch sp.LoanSection.String {
		case LoanSectionInterest:
			interestCount++
			if !sp.IsCategorized() {
				return false
			}
		case LoanSectionPrincipal:
			principalCount++
			principalIsTransfer = sp.IsTransfer()
			principalTarget = sp.TransferAccountID.ID
		case LoanSectionEscrow:
			if !sp.IsCategorized() {
				return false
			}
		default:
			return false
		}
	}

	if principalCount != 1 || interestCount > 1 || !principalIsTransfer {
		return false
	}
	return isLoanTarget(lookup, principalTarget)
}

// IsLoanAdoptable reports whether st is loan-adoptable — the loose shape the
// Edit-as-loan affordance offers so a hand-built or accidentally-demoted loan
// schedule can be (re)promoted by saving through the wizard. It requires a
// fixed-amount, monthly (interval 1, no secondary day) multi-line schedule
// with exactly one transfer line targeting an active loan-type account,
// regardless of loan_section tags.
func IsLoanAdoptable(st *Transaction, lookup AccountLookup) bool {
	if st == nil || len(st.Splits) == 0 || !st.HasAmount() {
		return false
	}
	if !isMonthlyLoanCadence(st) {
		return false
	}

	transferCount := 0
	var target types.ID
	for _, sp := range st.Splits {
		if sp == nil {
			return false
		}
		if sp.IsTransfer() {
			transferCount++
			target = sp.TransferAccountID.ID
		}
	}
	if transferCount != 1 {
		return false
	}
	return isLoanTarget(lookup, target)
}
