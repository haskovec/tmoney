package transfer

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/transaction"
	xfer "github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
)

// resolvedTransfer is the format-agnostic view of a whole-transaction transfer
// that the `transfer edit` and `transfer delete` commands operate on.
//
// It is now a thin projection of xfer.Transfer, kept only so the command bodies
// in add.go / edit.go / delete.go do not have to change in the same commit that
// replaced the resolver underneath them. Phase 3 of
// specs/design-unified-transfer.md rewrites those commands against
// xfer.Transfer directly and deletes this type.
type resolvedTransfer struct {
	kind        xfer.Kind
	transferID  types.ID
	fromAccount *account.Account
	toAccount   *account.Account
	amount      types.Money // positive
	date        types.Date
	memo        string
	status      transaction.Status

	// categoryID is the transfer's category, read from whichever leg carries
	// one. Threaded back through the edit so an edit that does not touch the
	// category preserves it.
	categoryID types.NullableID

	// investmentTxnID is the ID of an investment-side leg of the transfer,
	// suitable for passing to investment.Service.UpdateTransferCash /
	// DeleteTransaction. Set for every dispatch kind except DispatchRegToReg.
	investmentTxnID types.ID
}

// errTransferLineSplit is returned by resolveTransferPair when the supplied
// leg belongs to a multi-line split (e.g. a paycheck's 401k contribution
// line). Editing or deleting those is out of scope for the CLI (plan D10).
type errTransferLineSplit struct {
	transactionID string
	parentID      string
}

func (e *errTransferLineSplit) Error() string {
	return fmt.Sprintf(
		"transaction %s is part of a multi-line split (parent: %s); "+
			"to edit or delete transfer-line splits, use the TUI",
		e.transactionID, e.parentID,
	)
}

// resolveTransferPair loads the whole-transaction transfer that the given leg
// belongs to. The leg ID may name either a regular transaction or an investment
// transaction.
//
// It now delegates entirely to transfer.Service.Resolve, which reads both
// ledgers in one query. The 268 lines of hand-rolled cross-table plumbing this
// replaced — resolveFromRegularLeg, resolveFromInvestmentLeg, findRegularLeg,
// findInvestmentLeg, refuseIfMultiLineSplit and investmentStatusToRegular —
// were one of three divergent copies of that logic (the TUI had the other two).
//
// Behavior is preserved exactly, including refusing split lines with
// *errTransferLineSplit: transfer.Resolve deliberately SUCCEEDS on a split line
// so callers can explain the refusal, and translating that into the CLI's
// existing error keeps the user-facing message identical.
func resolveTransferPair(svc *app.Services, legID types.ID) (*resolvedTransfer, error) {
	t, err := svc.Transfer.Resolve(legID)
	if err != nil {
		return nil, err
	}

	if t.Shape == xfer.ShapeSplitLine {
		return nil, &errTransferLineSplit{
			transactionID: legID.String(),
			parentID:      t.ParentTransactionID.String(),
		}
	}

	res := &resolvedTransfer{
		kind:        t.Kind,
		transferID:  t.TransferID,
		fromAccount: t.FromAccount,
		toAccount:   t.ToAccount,
		amount:      t.Amount,
		date:        t.Date,
		memo:        t.Memo,
		status:      t.Status,
		categoryID:  t.CategoryID,
	}

	for _, leg := range t.Legs() {
		if leg.Ledger == xfer.LedgerInvestment {
			res.investmentTxnID = leg.RowID
			break
		}
	}

	return res, nil
}
