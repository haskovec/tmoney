package transfer

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// legSelect is the unified projection over both ledgers. Both tables index
// transfer_id (idx_transactions_transfer, idx_inv_tx_transfer).
//
// This is deliberately a Go query string and NOT a database VIEW: a view would
// have to be dropped and recreated by every future migration that rebuilds
// either table — the recipe migration 019 already follows for
// portfolio_holdings / account_balances / category_spending — and it buys
// nothing a string does not.
//
// Two deliberate choices in the projection:
//
//   - IDs and enums are CAST to VARCHAR. The ID value types parse strings, and
//     casting sidesteps DuckDB's typed-NULL handling on the investment arm's
//     absent category_id.
//   - status stays a raw string and is normalized in Go per ledger. The two
//     tables use different enums ('uncleared' vs 'pending'), so scanning both
//     straight into transaction.Status would yield an invalid value for every
//     investment leg.
//
// investment_transactions has no category_id column, so the investment arm
// projects a NULL for it.
const legSelect = `
SELECT 'regular' AS ledger,
       CAST(NULL AS VARCHAR) AS inv_type,
       CAST(t.id AS VARCHAR) AS row_id,
       CAST(t.account_id AS VARCHAR) AS account_id,
       t.date AS date,
       t.amount AS amount,
       t.memo AS memo,
       CAST(t.status AS VARCHAR) AS status,
       CAST(t.category_id AS VARCHAR) AS category_id,
       CAST(t.transfer_id AS VARCHAR) AS transfer_id,
       CAST(t.transfer_account_id AS VARCHAR) AS transfer_account_id
FROM transactions t
WHERE t.transfer_id IS NOT NULL
UNION ALL
SELECT 'investment' AS ledger,
       CAST(i.transaction_type AS VARCHAR) AS inv_type,
       CAST(i.id AS VARCHAR) AS row_id,
       CAST(i.account_id AS VARCHAR) AS account_id,
       i.date AS date,
       i.total_amount AS amount,
       i.memo AS memo,
       CAST(i.status AS VARCHAR) AS status,
       CAST(NULL AS VARCHAR) AS category_id,
       CAST(i.transfer_id AS VARCHAR) AS transfer_id,
       CAST(i.transfer_account_id AS VARCHAR) AS transfer_account_id
FROM investment_transactions i
WHERE i.transfer_id IS NOT NULL
`

// scanLegs reads legSelect rows into normalized Legs.
func scanLegs(rows *sql.Rows) ([]Leg, error) {
	defer func() { _ = rows.Close() }()

	var legs []Leg
	for rows.Next() {
		var (
			ledger    string
			invType   sql.NullString
			rawStatus string
			memo      types.NullableString
			otherAcct types.NullableID
			leg       Leg
		)
		if err := rows.Scan(
			&ledger,
			&invType,
			&leg.RowID,
			&leg.AccountID,
			&leg.Date,
			&leg.Amount,
			&memo,
			&rawStatus,
			&leg.CategoryID,
			new(types.ID), // transfer_id: known by the caller, not stored per leg
			&otherAcct,
		); err != nil {
			return nil, fmt.Errorf("failed to scan transfer leg: %w", err)
		}

		leg.Ledger = Ledger(ledger)
		if invType.Valid {
			leg.InvType = invType.String
		}
		if memo.Valid {
			leg.Memo = memo.String
		}
		leg.OtherAccountID = otherAcct.ID

		// Normalize the status per ledger — the two enums differ.
		switch leg.Ledger {
		case LedgerInvestment:
			leg.Status = StatusToRegular(investment.TransactionStatus(rawStatus))
		default:
			leg.Status = transaction.Status(rawStatus)
		}

		legs = append(legs, leg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transfer legs: %w", err)
	}
	return legs, nil
}

// Get loads a transfer by its transfer_id, reading both ledgers.
//
// Unlike transaction.TransferRepository.GetByTransferID it does not assert two
// rows on the regular table — it asserts two rows ACROSS both, and returns
// *MalformedPairError otherwise, naming the per-table counts. That single
// wrong assertion is why `tmoney transaction void <regular leg of an inv↔reg
// transfer>` fails today with "expected 2 transactions for transfer, found 1".
func (s *Service) Get(transferID types.ID) (*Transfer, error) {
	query := `SELECT * FROM (` + legSelect + `) legs WHERE legs.transfer_id = ?`

	rows, err := s.q().Query(query, transferID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query transfer legs: %w", err)
	}
	legs, err := scanLegs(rows)
	if err != nil {
		return nil, err
	}

	// Shape is decided BEFORE the leg-count assertion, because the two shapes
	// have different arities. A whole transfer is two ledger rows. A transfer
	// LINE inside a multi-line split is ONE ledger row (the counterpart minted
	// in the target account) plus the split row itself, which lives in
	// transaction_splits and has no transaction row of its own. Asserting two
	// ledger rows first would report every split line as malformed.
	split, err := s.splitRepo.GetByTransferID(transferID)
	if err != nil {
		return nil, fmt.Errorf("failed to check whether transfer %s is a split line: %w", transferID.String(), err)
	}
	if split != nil {
		return s.assembleSplitLine(transferID, legs, split)
	}

	if len(legs) != 2 {
		return nil, malformed(transferID, legs)
	}
	return s.assembleWhole(transferID, legs)
}

// malformed builds the leg-count error, or NotFound when nothing was found at
// all — a transfer_id naming zero rows is absent, not corrupt.
func malformed(transferID types.ID, legs []Leg) error {
	if len(legs) == 0 {
		return &dberrors.NotFoundError{Entity: "transfer", ID: transferID.String()}
	}
	var reg, inv int
	for _, l := range legs {
		if l.Ledger == LedgerInvestment {
			inv++
		} else {
			reg++
		}
	}
	return &MalformedPairError{
		TransferID:     transferID,
		RegularLegs:    reg,
		InvestmentLegs: inv,
	}
}

// Resolve loads the whole transfer from ANY leg's row ID, reading both ledgers.
//
// It replaces cli/transfer/resolve.go in its entirety and the cross-table
// halves of both TUI edit loaders. It is also a bug fix for the TUI: today
// loadEditTransferDialogData decides which service to call by scanning the
// dialog's loaded account list, which comes from accountSvc.List(true) —
// ACTIVE ONLY — and accountTypeByID returns "" for a missing account, which
// reads as non-investment. So an inv↔reg transfer whose investment counterpart
// is closed gets misrouted. Resolve reads account rows directly and is
// unaffected by what a dialog happens to have loaded.
//
// A split-line transfer RESOLVES SUCCESSFULLY, with Shape == ShapeSplitLine:
// reads must work so callers can explain the refusal. Only the verbs refuse.
//
// Errors: *dberrors.NotFoundError when no such row exists in either table;
// *transaction.IsNotTransferError when the row exists but carries no
// transfer_id.
func (s *Service) Resolve(legRowID types.ID) (*Transfer, error) {
	var transferID types.ID
	err := s.q().QueryRow(
		`SELECT legs.transfer_id FROM (`+legSelect+`) legs WHERE legs.row_id = ?`,
		legRowID.String(),
	).Scan(&transferID)

	switch {
	case err == sql.ErrNoRows:
		// The row is either absent or present-but-not-a-transfer. Distinguish,
		// so callers get an actionable error instead of a bare "not found".
		exists, existsErr := s.rowExists(legRowID)
		if existsErr != nil {
			return nil, existsErr
		}
		if exists {
			return nil, &transaction.IsNotTransferError{ID: legRowID.String()}
		}
		return nil, &dberrors.NotFoundError{Entity: "transaction", ID: legRowID.String()}
	case err != nil:
		return nil, fmt.Errorf("failed to resolve transfer for leg %s: %w", legRowID.String(), err)
	}

	return s.Get(transferID)
}

// rowExists reports whether legRowID names a row in either ledger, regardless
// of whether it is part of a transfer.
func (s *Service) rowExists(legRowID types.ID) (bool, error) {
	var exists bool
	err := s.q().QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM transactions WHERE CAST(id AS VARCHAR) = ?
			UNION ALL
			SELECT 1 FROM investment_transactions WHERE CAST(id AS VARCHAR) = ?
		)`,
		legRowID.String(), legRowID.String(),
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check whether row %s exists: %w", legRowID.String(), err)
	}
	return exists, nil
}

// assembleSplitLine builds the read model for a transfer LINE inside a
// multi-line split. The split row supplies one side and the minted counterpart
// row the other, so the pair is (transaction_splits row, ledger row) rather
// than two ledger rows.
//
// Every verb refuses ShapeSplitLine; this exists so the refusal can name the
// parent instead of the caller seeing "not found" or "malformed".
func (s *Service) assembleSplitLine(transferID types.ID, legs []Leg, split *transaction.Split) (*Transfer, error) {
	if len(legs) != 1 {
		// The counterpart is missing (or duplicated) — genuinely corrupt.
		return nil, malformed(transferID, legs)
	}
	counterpart := legs[0]

	parent, err := s.txnRepo.GetByID(split.TransactionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load split-line parent %s: %w", split.TransactionID.String(), err)
	}

	splitLeg := Leg{
		Ledger:         LedgerSplit,
		RowID:          split.ID,
		AccountID:      parent.AccountID,
		OtherAccountID: split.TransferAccountID.ID,
		Date:           parent.Date,
		Amount:         split.Amount,
		Status:         parent.Status,
		CategoryID:     types.NullableID{ID: split.CategoryID, Valid: true},
	}
	if split.Memo.Valid {
		splitLeg.Memo = split.Memo.String
	}

	from, to := orientLegs([]Leg{splitLeg, counterpart})

	parentAcct, err := s.accountRepo.GetByID(parent.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load split-line parent account: %w", err)
	}
	targetAcct, err := s.accountRepo.GetByID(counterpart.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load split-line target account: %w", err)
	}
	fromAcct, toAcct := parentAcct, targetAcct
	if from.RowID == counterpart.RowID {
		fromAcct, toAcct = targetAcct, parentAcct
	}

	return &Transfer{
		TransferID:          transferID,
		Kind:                ClassifyKind(fromAcct.Type, toAcct.Type),
		FromAccount:         fromAcct,
		ToAccount:           toAcct,
		Movement:            classifyMovement(from, to),
		Shape:               ShapeSplitLine,
		ParentTransactionID: split.TransactionID,
		From:                from,
		To:                  to,
		Amount:              split.Amount.Abs(),
		Date:                parent.Date,
		Memo:                splitLeg.Memo,
		Status:              parent.Status,
		CategoryID:          types.NullableID{ID: split.CategoryID, Valid: true},
	}, nil
}

// assembleWhole turns two ledger legs into a Transfer: orients them, classifies
// Kind and Movement, and picks the transfer-level fields.
func (s *Service) assembleWhole(transferID types.ID, legs []Leg) (*Transfer, error) {
	from, to := orientLegs(legs)

	fromAcct, err := s.accountRepo.GetByID(from.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load from-account for transfer %s: %w", transferID.String(), err)
	}
	toAcct, err := s.accountRepo.GetByID(to.AccountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load to-account for transfer %s: %w", transferID.String(), err)
	}

	t := &Transfer{
		TransferID:  transferID,
		Kind:        ClassifyKind(fromAcct.Type, toAcct.Type),
		Movement:    classifyMovement(from, to),
		Shape:       ShapeWhole,
		From:        from,
		To:          to,
		FromAccount: fromAcct,
		ToAccount:   toAcct,
		Amount:      to.Amount.Abs(),
		Date:        from.Date,
		Memo:       from.Memo,
		// Legs can disagree when only one side has been reconciled; the
		// transfer-level Status reports the From leg and the mutation guards
		// inspect BOTH legs, so a half-reconciled pair is still refused.
		Status: from.Status,
	}

	// The category lives on whichever leg can carry one. For a mirrored
	// regular pair either leg has it; for an inv↔reg transfer only the
	// regular leg does; an inv↔inv transfer has none.
	if from.CategoryID.Valid {
		t.CategoryID = from.CategoryID
	} else if to.CategoryID.Valid {
		t.CategoryID = to.CategoryID
	}

	return t, nil
}

// orientLegs puts the sending (negative-amount) leg first.
//
// A voided pair has both amounts zeroed, so the sign carries no orientation and
// none can be recovered: transfer_account_id is a symmetric mutual pointer, and
// created_at is stamped Go-side nanoseconds apart then truncated to microseconds
// by DuckDB, so it is frequently equal and never a reliable tie-break. For that
// case orientation is arbitrary but STABLE — ordered by row ID — so a voided
// transfer at least renders and round-trips consistently. The write path never
// reads orientation; every snapshot it takes is RowID-addressed.
func orientLegs(legs []Leg) (from, to Leg) {
	a, b := legs[0], legs[1]
	switch {
	case a.Amount.IsNegative() && !b.Amount.IsNegative():
		return a, b
	case b.Amount.IsNegative() && !a.Amount.IsNegative():
		return b, a
	default:
		ordered := []Leg{a, b}
		sort.Slice(ordered, func(i, j int) bool {
			return ordered[i].RowID.String() < ordered[j].RowID.String()
		})
		return ordered[0], ordered[1]
	}
}

// classifyMovement reports whether the pair moves cash or shares. A
// transfer_shares pair is inv↔inv by account type but is owned by
// investment.Service, so every cash verb must refuse it.
func classifyMovement(from, to Leg) Movement {
	shareType := string(investment.TransactionTypeTransferShares)
	if from.InvType == shareType || to.InvType == shareType {
		return MovementShares
	}
	return MovementCash
}
