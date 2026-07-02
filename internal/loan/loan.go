// Package loan holds the pure amortization math for tmoney's loan feature: the
// monthly periodic rate, the standard P&I payment, the per-occurrence
// interest/principal split, and a forward projection to payoff. Everything here
// is a pure function over types.Money / types.Date with no database or UI
// dependency, mirroring internal/investment/computation.go. Callers (the
// scheduled service, the amortization view, the CLI) supply the loan's live
// balance and APR and consume the results.
//
// Rounding follows the US mortgage/car-loan convention documented in
// specs/loan-wizard.md: the monthly rate is kept at full division precision and
// never pre-rounded; only final money figures are rounded, half-up to cents
// (alpacadecimal's Round is half-away-from-zero, which equals half-up for the
// non-negative magnitudes used here).
package loan

import (
	"errors"
	"fmt"
	"time"

	"github.com/alpacahq/alpacadecimal"

	"github.com/haskovec/tmoney/internal/types"
)

// maxProjectionRows caps a forward projection at 1,200 payments (100 monthly
// years). The negative-amortization guard only requires principal > 0, so a
// deliberately tiny payment (e.g. $1/month principal) could otherwise project
// forever; hitting the cap sets Projection.Truncated.
const maxProjectionRows = 1200

// ErrNegativeAmortization is returned when a P&I payment does not exceed the
// month's interest, so the balance would grow rather than shrink. Callers detect
// it with errors.Is to distinguish it from other failures.
var ErrNegativeAmortization = errors.New("payment does not cover interest (negative amortization)")

// MonthlyRate returns the monthly periodic rate r = APR / 100 / 12 at the
// library's division precision (16 places). apr is a percentage (e.g. 6.5 for
// 6.5% APR). It backs the Payment closed form, where a sub-cent rate difference
// is immaterial to a rounded prefill. The authoritative per-payment interest is
// computed in SplitPayment as owed × APR / 1200 (multiplying before dividing)
// rather than through this pre-rounded factor, so a balance can never scale the
// rate's rounding error into a wrong cent.
func MonthlyRate(apr types.Money) alpacadecimal.Decimal {
	return apr.Decimal().Div(alpacadecimal.NewFromInt(100)).Div(alpacadecimal.NewFromInt(12))
}

// Payment returns the fixed monthly principal-and-interest payment that amortizes
// principal over termMonths at the given APR, rounded half-up to cents. It is a
// prefill estimate (servicers round differently); the authoritative per-payment
// split comes from SplitPayment against the live balance.
//
// For a 0% APR it returns ceil-to-cent(principal / termMonths) so that
// payment × termMonths ≥ principal — the final payment then clamps the fractional
// remainder instead of leaving a stray one-cent (n+1)th payment.
func Payment(principal, apr types.Money, termMonths int) (types.Money, error) {
	if termMonths <= 0 {
		return types.ZeroMoney, fmt.Errorf("term months must be positive, got %d", termMonths)
	}
	if principal.IsNegative() {
		return types.ZeroMoney, fmt.Errorf("principal must not be negative, got %s", principal)
	}

	p := principal.Decimal()
	r := MonthlyRate(apr)

	// 0% (or, defensively, a non-positive) rate: level principal, rounded up so
	// the payments cover the balance and the last one clamps down.
	if !r.IsPositive() {
		perMonth := p.Div(alpacadecimal.NewFromInt(int64(termMonths)))
		return types.NewMoneyFromDecimal(perMonth.RoundCeil(2)), nil
	}

	// M = P·r / (1 − (1 + r)^−n)
	onePlusR := alpacadecimal.NewFromInt(1).Add(r)
	pow := powInt(onePlusR, termMonths) // (1+r)^n, computed exactly
	denom := alpacadecimal.NewFromInt(1).Sub(alpacadecimal.NewFromInt(1).Div(pow))
	m := p.Mul(r).Div(denom)
	return types.NewMoneyFromDecimal(m.Round(2)), nil
}

// SplitPayment divides a fixed P&I payment into its interest and principal parts
// against the owed balance (a positive magnitude). Escrow lines are fixed
// pass-throughs and take no part in this math.
//
//	interest  = round_half_up(owed × r, 2)
//	principal = piPayment − interest
//
// If the computed principal would exceed the owed balance (the final payment),
// principal is clamped to owed and final is true, shrinking the effective draft.
// If the payment fails to cover the interest on a positive balance (principal ≤ 0
// while owed > 0), it returns ErrNegativeAmortization.
func SplitPayment(owed, apr, piPayment types.Money) (interest, principal types.Money, final bool, err error) {
	// interest = round_half_up(owed × APR/1200, 2). Multiply owed by the APR
	// exactly first, then divide — going through MonthlyRate would materialize a
	// factor pre-rounded to 16 places, and the balance would then scale that
	// rounding error, flipping an exact half-cent interest tie downward. The spec
	// requires the rate never be pre-rounded and only the final interest figure
	// rounded.
	interest = types.NewMoneyFromDecimal(
		owed.Decimal().Mul(apr.Decimal()).Div(alpacadecimal.NewFromInt(1200)).Round(2))
	principal = piPayment.Sub(interest)

	switch {
	case principal.Cmp(owed) > 0:
		// Final payment: never pay down more than what is owed.
		principal = owed
		final = true
	case !principal.IsPositive() && owed.IsPositive():
		return types.ZeroMoney, types.ZeroMoney, false,
			fmt.Errorf("%w: P&I payment %s does not exceed interest %s on balance %s",
				ErrNegativeAmortization, piPayment, interest, owed)
	}
	return interest, principal, final, nil
}

// Row is one payment in an amortization projection.
type Row struct {
	N            int         // 1-based payment number
	Date         types.Date  // scheduled payment date
	TotalDraft   types.Money // interest + principal + escrow (the amount leaving the funding account)
	Interest     types.Money
	Principal    types.Money
	Escrow       types.Money // fixed escrow pass-through for the period
	BalanceAfter types.Money // owed remaining after this payment
	Final        bool        // true when principal was clamped to the remaining balance (last, shrunken payment)
}

// Projection is a forward amortization schedule from the current balance to
// payoff. Truncated is true when the projection stopped at the row cap with a
// balance still owing, in which case the final row is not the payoff.
type Projection struct {
	Rows      []Row
	Truncated bool
}

// Stats summarizes a Projection. When Truncated is true, PayoffDate and
// TotalInterestRemaining are not meaningful (the loan does not pay off within the
// cap) and callers render them as "100y+".
type Stats struct {
	PaymentsRemaining      int
	PayoffDate             types.Date
	TotalInterestRemaining types.Money
	Truncated              bool
}

// Project builds the remaining amortization schedule for a loan with the given
// owed balance, APR, fixed P&I payment, and fixed escrow total, starting at
// nextDate. Subsequent payment dates advance one calendar month at a time,
// anchored to dayOfMonth (1-31, or -1 for the last day of the month) with
// month-end clamping (a 31 falls back to Feb 28/29, etc.).
//
// Pass the loan schedule's stored day-of-month. Loan schedules always carry an
// explicit one (the wizard seeds it from the first payment date), for which this
// clamping matches the dates the scheduler posts. Project deliberately does not
// reproduce the scheduler's legacy roll-forward for a schedule with no
// day-of-month set (where Go's month arithmetic spills a 31 into the following
// month), so callers must supply an explicit day-of-month.
//
// Rows run until the balance reaches zero or the row cap (maxProjectionRows) is
// hit, whichever comes first. A negative-amortization payment returns
// ErrNegativeAmortization.
func Project(owed, apr, piPayment, escrowTotal types.Money, nextDate types.Date, dayOfMonth int) (Projection, error) {
	var proj Projection
	bal := owed
	for bal.IsPositive() {
		if len(proj.Rows) >= maxProjectionRows {
			proj.Truncated = true
			break
		}
		interest, principal, final, err := SplitPayment(bal, apr, piPayment)
		if err != nil {
			return Projection{}, err
		}

		n := len(proj.Rows) + 1
		balAfter := bal.Sub(principal)
		proj.Rows = append(proj.Rows, Row{
			N:            n,
			Date:         addMonths(nextDate, n-1, dayOfMonth),
			TotalDraft:   interest.Add(principal).Add(escrowTotal),
			Interest:     interest,
			Principal:    principal,
			Escrow:       escrowTotal,
			BalanceAfter: balAfter,
			Final:        final,
		})

		bal = balAfter
		if final {
			break
		}
	}
	return proj, nil
}

// RemainingStats summarizes a projection: payments left, payoff date, and total
// interest still to be paid. When the projection is truncated the payoff date and
// interest total are left unknown (the Truncated flag is propagated).
func RemainingStats(p Projection) Stats {
	s := Stats{
		PaymentsRemaining:      len(p.Rows),
		Truncated:              p.Truncated,
		TotalInterestRemaining: types.ZeroMoney,
	}
	for i := range p.Rows {
		s.TotalInterestRemaining = s.TotalInterestRemaining.Add(p.Rows[i].Interest)
	}
	if !p.Truncated && len(p.Rows) > 0 {
		s.PayoffDate = p.Rows[len(p.Rows)-1].Date
	}
	return s
}

// powInt returns base^n for a non-negative integer exponent using exact
// exponentiation by squaring (multiplications only), avoiding the approximation
// alpacadecimal.Pow uses for general real powers.
func powInt(base alpacadecimal.Decimal, n int) alpacadecimal.Decimal {
	result := alpacadecimal.NewFromInt(1)
	b := base
	for n > 0 {
		if n&1 == 1 {
			result = result.Mul(b)
		}
		n >>= 1
		if n > 0 {
			b = b.Mul(b)
		}
	}
	return result
}

// addMonths advances base by months calendar months, anchored to dayOfMonth
// (1-31, or -1 for last day of month) with month-end clamping. It mirrors the
// scheduled engine's addMonthsWithDayHandling so a projection's dates match the
// dates the scheduler will post on. months is expected to be non-negative.
func addMonths(base types.Date, months, dayOfMonth int) types.Date {
	targetYear := base.Year()
	targetMonth := int(base.Month()) + months
	for targetMonth > 12 {
		targetMonth -= 12
		targetYear++
	}

	month := time.Month(targetMonth)
	last := lastDayOf(targetYear, month)
	day := dayOfMonth
	if day == -1 || day > last {
		day = last
	}
	return types.NewDate(targetYear, month, day)
}

// lastDayOf returns the last day number of the given month/year.
func lastDayOf(year int, month time.Month) int {
	// Day 0 of the next month is the last day of this month.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
