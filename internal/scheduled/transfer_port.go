package scheduled

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// TransferPort is how the scheduled service posts a transfer occurrence without
// importing internal/transfer.
//
// The indirection is required, not stylistic. internal/transfer imports
// internal/investment, and internal/investment's in-package test file
// split_counterpart_test.go imports internal/scheduled — so a direct
// scheduled → transfer import closes a loop that Go rejects outright:
//
//	package .../internal/investment
//	  imports .../internal/scheduled from split_counterpart_test.go
//	  imports .../internal/transfer from scheduled_service.go
//	  imports .../internal/investment from read.go: import cycle not allowed in test
//
// So the interface is declared by the CONSUMER and satisfied by
// *transfer.Service, with only db.Queryer and internal/types crossing the
// boundary. That is the same shape as
// transaction.InvestmentCounterpartPort, and the same reason internal/db owns
// Queryer: it is the one vocabulary two otherwise-unrelated packages share.
//
// The transaction is passed per call rather than via a bound-copy return, so no
// method here names a foreign type. A returning InTx would have to name its own
// interface as the return type, which is exactly what forced the old
// CounterpartInTx naming hack.
type TransferPort interface {
	// CreateTransfer writes both legs of a transfer inside q, which is the
	// posting transaction — so the posted legs and the schedule's next_date
	// advance commit together, closing the double-post window.
	//
	// It returns the shared transfer_id and the row ID of the REGULAR-ledger
	// leg, which is types.NilID for an investment↔investment transfer (both of
	// whose legs live in investment_transactions). Callers that need a
	// *transaction.Transaction must tolerate the nil case.
	CreateTransfer(
		q db.Queryer,
		fromAccountID, toAccountID types.ID,
		date types.Date,
		amount types.Money,
		memo string,
		categoryID types.NullableID,
	) (transferID types.ID, regularLegID types.ID, err error)

	// StoresCategory reports whether a transfer between these two account types
	// can carry a category label. It is false for exactly one pair,
	// investment↔investment, whose legs both live in investment_transactions and
	// have nowhere to put one.
	//
	// This package needs the answer in two places — healing rows that an older
	// binary let through, and refusing to hand the port a category it will
	// reject — and must not restate the rule, because internal/transfer owns it.
	// Asking through the port is how it calls the rule without importing it.
	StoresCategory(from, to account.Type) bool
}
