package loan

import (
	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	loandom "github.com/haskovec/tmoney/internal/loan"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// loanInfo bundles a loan account with the live amortization state derived from
// its loan-shaped payment schedule. Everything here is computed from the loan's
// balance, its APR, and the schedule's derived P&I payment — nothing is stored.
type loanInfo struct {
	account *accountdom.Account

	// hasSchedule is true when a loan-shaped schedule targets the account.
	hasSchedule bool
	schedule    *scheduleddom.Transaction

	owed        types.Money // positive magnitude of what is owed
	aprValid    bool
	apr         types.Money // percentage (e.g. 6.5)
	piPayment   types.Money // fixed P&I payment (escrow-exclusive)
	escrowTotal types.Money // fixed escrow pass-through per period (positive magnitude)

	projection loandom.Projection
	stats      loandom.Stats

	// projErr is set when a schedule + APR exist but the projection could not be
	// computed (e.g. negative amortization).
	projErr error
}

// resolveLoanInfo derives the live amortization state for a loan account,
// mirroring the TUI amortization view: it locates the loan-shaped schedule by
// its principal transfer target (FindLoanSchedule), reads the balance as of the
// next payment date (the same as-of balance the next post computes against), and
// runs internal/loan.Project. A missing schedule, a missing APR, and negative
// amortization all resolve to a graceful partial state rather than an error.
func resolveLoanInfo(svc *app.Services, acct *accountdom.Account) (*loanInfo, error) {
	info := &loanInfo{account: acct}
	if acct.InterestRate.Valid {
		info.aprValid = true
		info.apr = acct.InterestRate.Money
	}

	sched, err := svc.Scheduled.FindLoanSchedule(acct.ID)
	if err != nil {
		return nil, err
	}

	if sched == nil {
		bal, gerr := svc.Account.GetBalance(acct.ID)
		if gerr != nil {
			return nil, gerr
		}
		info.owed = bal.CurrentBalance.Neg()
		return info, nil
	}

	info.hasSchedule = true
	info.schedule = sched
	piPayment, escrowTotal, dayOfMonth := scheduleddom.LoanScheduleInputs(sched)
	info.piPayment = piPayment
	info.escrowTotal = escrowTotal

	signedBal, berr := svc.Account.BalanceAsOf(acct.ID, sched.NextDate)
	if berr != nil {
		return nil, berr
	}
	info.owed = signedBal.Neg()

	if info.aprValid {
		proj, perr := loandom.Project(info.owed, info.apr, piPayment, escrowTotal, sched.NextDate, dayOfMonth)
		if perr != nil {
			info.projErr = perr
		} else {
			info.projection = proj
			info.stats = loandom.RemainingStats(proj)
		}
	}
	return info, nil
}
