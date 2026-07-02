package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/types"
)

// LoanEscrowLine is one fixed escrow pass-through for a loan schedule's
// month-one snapshot: a category and its positive monthly magnitude. Escrow
// lines are fixed and take no part in the interest/principal split math.
type LoanEscrowLine struct {
	CategoryID types.ID
	Amount     types.Money // positive magnitude
	Memo       string
}

// LoanSnapshotInput describes the raw inputs the loan wizard and `loan add`
// collect to build a loan-shaped schedule's month-one template. Owed and
// PIPayment are positive magnitudes; APR is a percentage (6.5 = 6.5%).
// InterestCatID is required only when the computed month-one interest is
// greater than $0.00.
type LoanSnapshotInput struct {
	LoanAccountID types.ID
	APR           types.Money
	Owed          types.Money
	PIPayment     types.Money
	InterestCatID types.ID
	Escrow        []LoanEscrowLine
}

// BuildLoanSnapshot computes the month-one interest/principal split for a new
// loan-shaped schedule and returns the parent amount plus the tagged scheduled
// split rows. It is the creation-time mirror of ComputeLoanSplits — the stored
// template is exactly what the first post recomputes against the same balance:
//
//   - the interest line is omitted when computed interest rounds to $0.00 (a 0%
//     loan, or a nearly-paid balance), matching template validation's rejection
//     of a zero-amount line;
//   - the final-payment clamp shrinks the parent, so a loan created with one
//     payment left stores a clamped parent that its lines sum to;
//   - every line carries its loan_section tag and posts with funding-account
//     (negative) signs; the principal line is a transfer into the loan account.
//
// It wraps internal/loan.SplitPayment and propagates loan.ErrNegativeAmortization
// (P&I payment does not exceed the month's interest).
func BuildLoanSnapshot(in LoanSnapshotInput) (parent types.Money, splits []*Split, final bool, err error) {
	if in.LoanAccountID.IsNil() {
		return types.ZeroMoney, nil, false, fmt.Errorf("loan account is required")
	}

	interest, principal, final, err := loan.SplitPayment(in.Owed, in.APR, in.PIPayment)
	if err != nil {
		return types.ZeroMoney, nil, false, err
	}

	total := types.ZeroMoney
	splits = make([]*Split, 0, 2+len(in.Escrow))

	// Interest line — omitted when $0.00 (0% loans and nearly-paid balances have
	// no interest line; a zero-amount line is rejected by template validation).
	if interest.IsPositive() {
		if in.InterestCatID.IsNil() {
			return types.ZeroMoney, nil, false,
				fmt.Errorf("interest category is required for a loan that accrues interest")
		}
		line := NewCategorizedSplit(types.NilID, in.InterestCatID, interest.Neg())
		line.LoanSection = types.NullableString{String: LoanSectionInterest, Valid: true}
		splits = append(splits, line)
		total = total.Add(line.Amount)
	}

	// Principal line — a transfer into the loan account, moving its negative
	// balance toward zero.
	pLine := NewTransferSplit(types.NilID, in.LoanAccountID, principal.Neg())
	pLine.LoanSection = types.NullableString{String: LoanSectionPrincipal, Valid: true}
	splits = append(splits, pLine)
	total = total.Add(pLine.Amount)

	// Escrow lines — fixed categorized pass-throughs (property tax, insurance,
	// PMI) so the schedule total matches the real bank draft.
	for _, e := range in.Escrow {
		if e.CategoryID.IsNil() {
			return types.ZeroMoney, nil, false, fmt.Errorf("escrow line is missing a category")
		}
		if !e.Amount.IsPositive() {
			return types.ZeroMoney, nil, false,
				fmt.Errorf("escrow amount must be positive, got %s", e.Amount)
		}
		line := NewCategorizedSplit(types.NilID, e.CategoryID, e.Amount.Neg())
		line.LoanSection = types.NullableString{String: LoanSectionEscrow, Valid: true}
		if e.Memo != "" {
			line.SetMemo(e.Memo)
		}
		splits = append(splits, line)
		total = total.Add(line.Amount)
	}

	return total, splits, final, nil
}
