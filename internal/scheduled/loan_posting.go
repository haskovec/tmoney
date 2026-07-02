package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// LoanSplits is the recomputed month for a loan-shaped schedule: the parent
// amount and split lines that posting the given occurrence should create,
// derived from the loan's live balance rather than the stored template. All
// amounts are signed as they post on the funding account (negative outflows);
// the principal line is a transfer whose counterpart posts positive into the
// loan account. Interest / Principal / EscrowTotal are the positive magnitudes,
// exposed for previews and the amortization view. Final marks the clamped last
// payment.
type LoanSplits struct {
	ParentAmount types.Money
	Splits       []*transaction.Split
	Interest     types.Money
	Principal    types.Money
	EscrowTotal  types.Money
	Final        bool
}

// loanTemplateSections partitions a loan-shaped template's lines by section.
type loanTemplateSections struct {
	interest    *Split
	principal   *Split
	escrow      []*Split
	escrowTotal types.Money // positive magnitude
}

// sectionsOf partitions st.Splits by loan_section. Untagged lines are ignored
// (callers gate on IsLoanShaped first, where every line is tagged).
func sectionsOf(st *Transaction) loanTemplateSections {
	sec := loanTemplateSections{escrowTotal: types.ZeroMoney}
	for _, sp := range st.Splits {
		if sp == nil || !sp.LoanSection.Valid {
			continue
		}
		switch sp.LoanSection.String {
		case LoanSectionInterest:
			sec.interest = sp
		case LoanSectionPrincipal:
			sec.principal = sp
		case LoanSectionEscrow:
			sec.escrow = append(sec.escrow, sp)
			sec.escrowTotal = sec.escrowTotal.Add(sp.Amount.Abs())
		}
	}
	return sec
}

// ComputeLoanSplits recomputes the interest/principal split (and carries the
// fixed escrow lines through) for one occurrence of a loan-shaped schedule,
// against the loan's balance as of occurrenceDate. The P&I payment is derived
// as the template parent-amount magnitude minus the escrow-line magnitudes
// (never stored separately). It wraps internal/loan.SplitPayment.
//
// Signs: every returned line and the parent amount are negative on the funding
// account; the principal transfer counterpart posts positive into the loan.
// A computed interest line of exactly $0.00 is omitted (a 0% loan, or a nearly
// paid-off balance). Typed errors: ErrLoanPaidOff (owed ≤ 0),
// ErrLoanNoInterestRate (NULL APR), ErrNegativeAmortization (payment ≤
// interest), and ErrLoanMissingInterestLine (computed interest > $0.00 but the
// template has no interest line).
//
// Callers should confirm IsLoanShaped(st) first; ComputeLoanSplits still
// defends against a missing principal transfer line.
func (s *Service) ComputeLoanSplits(st *Transaction, occurrenceDate types.Date) (*LoanSplits, error) {
	sec := sectionsOf(st)
	if sec.principal == nil || !sec.principal.TransferAccountID.Valid {
		return nil, fmt.Errorf("loan schedule has no principal transfer line")
	}

	loanAcct, err := s.accountRepo.GetByID(sec.principal.TransferAccountID.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load loan account: %w", err)
	}
	if !loanAcct.InterestRate.Valid {
		return nil, ErrLoanNoInterestRate
	}
	apr := loanAcct.InterestRate.Money

	signedBalance, err := s.accountRepo.BalanceAsOf(loanAcct.ID, occurrenceDate)
	if err != nil {
		return nil, fmt.Errorf("failed to read loan balance: %w", err)
	}
	owed := signedBalance.Neg()
	if !owed.IsPositive() {
		return nil, ErrLoanPaidOff
	}

	// P&I payment = parent draft magnitude − escrow magnitudes.
	piPayment := st.Amount.Money.Abs().Sub(sec.escrowTotal)

	interest, principal, final, err := loan.SplitPayment(owed, apr, piPayment)
	if err != nil {
		return nil, err // wraps loan.ErrNegativeAmortization
	}
	if sec.interest == nil && interest.IsPositive() {
		return nil, ErrLoanMissingInterestLine
	}

	// Build computed lines in the template's order (omit a $0.00 interest line;
	// posted-transaction validation rejects zero-amount splits).
	splits := make([]*transaction.Split, 0, len(st.Splits))
	total := types.ZeroMoney
	for _, tmpl := range st.Splits {
		if tmpl == nil || !tmpl.LoanSection.Valid {
			continue
		}
		var line *transaction.Split
		switch tmpl.LoanSection.String {
		case LoanSectionInterest:
			if !interest.IsPositive() {
				continue // omit zero interest
			}
			line = transaction.NewSplit(types.ID{}, tmpl.CategoryID.ID, interest.Neg())
		case LoanSectionPrincipal:
			line = &transaction.Split{
				BaseModel:         types.NewBaseModel(),
				Amount:            principal.Neg(),
				TransferAccountID: types.NullableID{ID: tmpl.TransferAccountID.ID, Valid: true},
			}
		case LoanSectionEscrow:
			// Fixed pass-through; the stored amount is already the signed
			// (negative) funding-account outflow.
			line = transaction.NewSplit(types.ID{}, tmpl.CategoryID.ID, tmpl.Amount)
		default:
			continue
		}
		if tmpl.Memo.Valid && tmpl.Memo.String != "" {
			line.SetMemo(tmpl.Memo.String)
		}
		total = total.Add(line.Amount)
		splits = append(splits, line)
	}

	return &LoanSplits{
		ParentAmount: total,
		Splits:       splits,
		Interest:     interest,
		Principal:    principal,
		EscrowTotal:  sec.escrowTotal,
		Final:        final,
	}, nil
}

// buildLoanTransaction assembles a loan payment's parent transaction and child
// splits from the recomputed interest/principal split (ComputeLoanSplits)
// rather than the stored template. The parent amount is the recomputed total
// draft — the final-payment clamp shrinks it — and the interest line is omitted
// when computed interest is $0.00. It is the loan branch of
// buildMultiLineTransaction, so it flows through Post, PostWithDate, and
// AutoPost alike.
func (s *Service) buildLoanTransaction(st *Transaction, date types.Date) (*builtMultiLineTransaction, error) {
	ls, err := s.ComputeLoanSplits(st, date)
	if err != nil {
		return nil, err
	}
	parent := transaction.NewTransaction(st.AccountID, date, ls.ParentAmount)
	if st.HasPayee() {
		parent.SetPayee(st.PayeeID.ID)
	}
	if st.Memo.Valid && st.Memo.String != "" {
		parent.SetMemo(st.Memo.String)
	}
	for _, sp := range ls.Splits {
		sp.TransactionID = parent.ID
	}
	return &builtMultiLineTransaction{parent: parent, splits: ls.Splits}, nil
}

// loanAccountID returns the loan account targeted by st's principal transfer
// line, if any. Used by the payoff-completion check.
func loanAccountID(st *Transaction) (types.ID, bool) {
	sec := sectionsOf(st)
	if sec.principal != nil && sec.principal.TransferAccountID.Valid {
		return sec.principal.TransferAccountID.ID, true
	}
	return types.ID{}, false
}

// finalizeLoanPayoff marks a loan-shaped schedule completed when, after a post,
// the loan's full balance has reached ≥ 0 (paid off, or overshot by a
// penny-tweaked edit). It is a no-op for non-loan schedules and for loans that
// still owe. Callers invoke it after writing the transaction and advancing the
// schedule, before persisting, so MarkCompleted's field changes are saved.
func (s *Service) finalizeLoanPayoff(st *Transaction) error {
	if !s.isLoanShaped(st) {
		return nil
	}
	loanID, ok := loanAccountID(st)
	if !ok {
		return nil
	}
	balance, err := s.accountRepo.Balance(loanID)
	if err != nil {
		return fmt.Errorf("failed to read loan balance for payoff check: %w", err)
	}
	if !balance.IsNegative() { // balance ≥ 0 ⇒ nothing owed
		st.MarkCompleted()
	}
	return nil
}
