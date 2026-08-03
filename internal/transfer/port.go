package transfer

import (
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// CreateTransfer satisfies scheduled.TransferPort so internal/scheduled can post
// a transfer occurrence without importing this package.
//
// The indirection exists because internal/investment's in-package test file
// split_counterpart_test.go imports internal/scheduled, and this package imports
// internal/investment — so a direct scheduled → transfer import is an "import
// cycle not allowed in test". See scheduled/transfer_port.go for the full chain.
//
// q is the caller's transaction. Binding through InTx(q) means the two transfer
// legs commit with whatever else the caller is writing — for scheduled posting,
// the schedule's next_date advance.
//
// Returns the shared transfer_id and the row ID of the regular-ledger leg, which
// is types.NilID for an inv↔inv transfer (both legs live in
// investment_transactions).
func (s *Service) CreateTransfer(
	q db.Queryer,
	fromAccountID, toAccountID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
	categoryID types.NullableID,
) (types.ID, types.ID, error) {
	res, err := s.InTx(q).Create(Spec{
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Date:          date,
		Amount:        amount,
		Memo:          memo,
		CategoryID:    categoryID,
	})
	if err != nil {
		return types.NilID, types.NilID, err
	}

	regularLegID := types.NilID
	for _, ref := range []LegRef{res.From, res.To} {
		if ref.Ledger == LedgerRegular {
			regularLegID = ref.RowID
			break
		}
	}
	return res.TransferID, regularLegID, nil
}
