package transfer

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// resolvedTransfer is the format-agnostic view of a whole-transaction transfer
// that the `transfer edit` and `transfer delete` commands operate on. It is
// produced by resolveTransferPair from a single leg's transaction ID.
type resolvedTransfer struct {
	kind        transaction.TransferDispatchKind
	transferID  types.ID
	fromAccount *account.Account
	toAccount   *account.Account
	amount      types.Money // positive
	date        types.Date
	memo        string
	status      transaction.Status

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
// belongs to and returns a format-agnostic view plus the dispatch kind. The
// leg ID may name either a regular transaction or an investment transaction.
//
// It refuses (returns *errTransferLineSplit) when the leg's transfer_id is
// owned by a multi-line split, per plan decision D10. It errors when the leg
// is not found or is not part of a transfer.
func resolveTransferPair(svc *app.Services, legID types.ID) (*resolvedTransfer, error) {
	// Try the regular-transaction table first.
	if txn, err := svc.TransactionRepo.GetByID(legID); err == nil {
		return resolveFromRegularLeg(svc, txn)
	} else if !isNotFound(err) {
		return nil, err
	}

	// Fall back to the investment-transaction table.
	if invTxn, err := svc.InvestmentRepo.GetByID(legID); err == nil {
		return resolveFromInvestmentLeg(svc, invTxn)
	} else if !isNotFound(err) {
		return nil, err
	}

	return nil, fmt.Errorf("transaction %s not found", legID.String())
}

// resolveFromRegularLeg builds a resolvedTransfer when the supplied leg is a
// row in the regular `transactions` table. The counterpart may be regular
// (reg↔reg) or investment (reg↔inv / inv↔reg).
func resolveFromRegularLeg(svc *app.Services, txn *transaction.Transaction) (*resolvedTransfer, error) {
	if !txn.IsTransfer() {
		return nil, fmt.Errorf("transaction %s is not a transfer", txn.ID.String())
	}
	transferID := txn.TransferID.ID

	if err := refuseIfMultiLineSplit(svc, txn.ID.String(), transferID); err != nil {
		return nil, err
	}

	thisAcct, err := svc.AccountRepo.GetByID(txn.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load account for leg %s: %w", txn.ID.String(), err)
	}
	otherAcct, err := svc.AccountRepo.GetByID(txn.TransferAccountID.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load counterpart account for leg %s: %w", txn.ID.String(), err)
	}

	// The negative-amount leg is the "from" side (money leaving).
	var fromAcct, toAcct *account.Account
	amount := txn.Amount
	if txn.Amount.IsNegative() {
		fromAcct, toAcct = thisAcct, otherAcct
		amount = amount.Neg()
	} else {
		fromAcct, toAcct = otherAcct, thisAcct
	}

	memo := ""
	if txn.Memo.Valid {
		memo = txn.Memo.String
	}

	res := &resolvedTransfer{
		kind:        transaction.ChooseTransferDispatch(fromAcct.Type, toAcct.Type),
		transferID:  transferID,
		fromAccount: fromAcct,
		toAccount:   toAcct,
		amount:      amount,
		date:        txn.Date,
		memo:        memo,
		status:      txn.Status,
	}

	// For inv-involving kinds, the investment-side leg lives in a different
	// table; find it so the investment-service dispatch has its handle.
	if res.kind != transaction.DispatchRegToReg {
		invLeg, err := findInvestmentLeg(svc, transferID)
		if err != nil {
			return nil, err
		}
		res.investmentTxnID = invLeg.ID
	}

	return res, nil
}

// resolveFromInvestmentLeg builds a resolvedTransfer when the supplied leg is a
// row in the `investment_transactions` table. This is always an inv-involving
// transfer (reg↔inv or inv↔inv).
func resolveFromInvestmentLeg(svc *app.Services, invTxn *investment.Transaction) (*resolvedTransfer, error) {
	if !invTxn.TransferID.Valid || !invTxn.TransferAccountID.Valid {
		return nil, fmt.Errorf("investment transaction %s is not a transfer", invTxn.ID.String())
	}
	transferID := invTxn.TransferID.ID

	if err := refuseIfMultiLineSplit(svc, invTxn.ID.String(), transferID); err != nil {
		return nil, err
	}

	thisAcct, err := svc.AccountRepo.GetByID(invTxn.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load account for leg %s: %w", invTxn.ID.String(), err)
	}
	otherAcct, err := svc.AccountRepo.GetByID(invTxn.TransferAccountID.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load counterpart account for leg %s: %w", invTxn.ID.String(), err)
	}

	var fromAcct, toAcct *account.Account
	amount := invTxn.TotalAmount
	if invTxn.TotalAmount.IsNegative() {
		fromAcct, toAcct = thisAcct, otherAcct
		amount = amount.Neg()
	} else {
		fromAcct, toAcct = otherAcct, thisAcct
	}

	memo := ""
	if invTxn.Memo.Valid {
		memo = invTxn.Memo.String
	}

	return &resolvedTransfer{
		kind:            transaction.ChooseTransferDispatch(fromAcct.Type, toAcct.Type),
		transferID:      transferID,
		fromAccount:     fromAcct,
		toAccount:       toAcct,
		amount:          amount,
		date:            invTxn.Date,
		memo:            memo,
		status:          investmentStatusToRegular(invTxn.Status),
		investmentTxnID: invTxn.ID,
	}, nil
}

// findInvestmentLeg returns the investment-table leg of a transfer identified
// by its shared transfer_id.
func findInvestmentLeg(svc *app.Services, transferID types.ID) (*investment.Transaction, error) {
	legs, err := svc.InvestmentRepo.ListByTransferID(transferID)
	if err != nil {
		return nil, fmt.Errorf("failed to load investment-side transfer leg: %w", err)
	}
	if len(legs) == 0 {
		return nil, fmt.Errorf("investment-side leg for transfer %s not found", transferID.String())
	}
	return legs[0], nil
}

// refuseIfMultiLineSplit returns *errTransferLineSplit when the transfer_id is
// owned by a multi-line split's transfer-line item (plan D10).
func refuseIfMultiLineSplit(svc *app.Services, legID string, transferID types.ID) error {
	split, err := svc.SplitRepo.GetByTransferID(transferID)
	if err != nil {
		return fmt.Errorf("failed to check for multi-line split: %w", err)
	}
	if split != nil {
		return &errTransferLineSplit{
			transactionID: legID,
			parentID:      split.TransactionID.String(),
		}
	}
	return nil
}

// investmentStatusToRegular maps an investment transaction status back to the
// canonical regular transaction.Status used by the CLI's transfer commands.
func investmentStatusToRegular(s investment.TransactionStatus) transaction.Status {
	switch s {
	case investment.TransactionStatusCleared:
		return transaction.StatusCleared
	case investment.TransactionStatusReconciled:
		return transaction.StatusReconciled
	default:
		return transaction.StatusUncleared
	}
}

// isNotFound reports whether err is (or wraps) a dberrors.NotFoundError.
func isNotFound(err error) bool {
	var nf *dberrors.NotFoundError
	return errors.As(err, &nf)
}
