package transaction

import "github.com/haskovec/tmoney/internal/account"

// TransferDispatchKind identifies which service path a unified Transfer flow
// should take, based on the (From.Type, To.Type) combination. Mapped 1:1 to
// the four service paths used by the TUI's unified Transfer dialog and the
// CLI's `tmoney transfer add`.
type TransferDispatchKind int

const (
	// DispatchRegToReg covers bank↔bank — both legs are non-investment.
	// Calls transaction.Service.CreateTransfer.
	DispatchRegToReg TransferDispatchKind = iota
	// DispatchInvToReg covers cash leaving an investment account for a
	// regular account (e.g. brokerage → checking withdrawal). Calls
	// investment.Service.TransferCash.
	DispatchInvToReg
	// DispatchRegToInv covers cash flowing from a regular account into an
	// investment account (e.g. checking → 401k contribution). Calls
	// investment.Service.DepositFromAccount.
	DispatchRegToInv
	// DispatchInvToInv covers cash moving between two investment accounts
	// (e.g. IRA → IRA rollover). Calls
	// investment.Service.TransferCashBetweenInvestments.
	DispatchInvToInv
)

// ChooseTransferDispatch picks the service path for a unified Transfer from
// the From/To account types. HSA counts as an investment type (see
// account.Type.IsInvestmentType), so an HSA on either side routes via the
// investment branches.
func ChooseTransferDispatch(from, to account.Type) TransferDispatchKind {
	fromInv := from.IsInvestmentType()
	toInv := to.IsInvestmentType()
	switch {
	case fromInv && toInv:
		return DispatchInvToInv
	case fromInv:
		return DispatchInvToReg
	case toInv:
		return DispatchRegToInv
	default:
		return DispatchRegToReg
	}
}
