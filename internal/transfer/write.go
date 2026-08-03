package transfer

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Edit is the mutable field set of an existing transfer. Accounts are absent
// deliberately: an Edit never moves a transfer between accounts.
//
// Delete + Create is the correct way to re-account a transfer — it is
// observable and undoable as two steps. A silent re-account would have to reason
// about opening dates and closed state across four accounts at once, which is
// exactly the complexity UpdateTransferShares carries today and gets wrong (it
// dereferences srcOld.TransferAccountID.ID having only checked TransferID.Valid).
// investment.UpdateTransferCash's signature accepts new account IDs, so the
// capability exists in the service layer today even though no front end can
// reach it.
type Edit struct {
	Date       types.Date
	Amount     types.Money // positive magnitude
	Memo       string
	CategoryID types.NullableID
	Status     transaction.Status
}

// Update rewrites both legs of an existing transfer in place.
//
// In place matters. investment.Service.UpdateTransferCash deletes both legs and
// recreates them under a BRAND-NEW transfer_id, so anything referencing the old
// id is silently orphaned, and the TUI's post-save cursor restore stashes a row
// ID that the update just deleted.
func (s *Service) Update(transferID types.ID, edit Edit) (*Result, error) {
	var res *Result
	if err := s.runInTx(func(b *Service) error {
		// The read happens INSIDE the transaction. The reconciled and void
		// guards this adds for investment-involving transfers have no backstop
		// underneath them — UpdateTransferCash has none — so a read-then-write
		// across the transaction boundary would be a genuine TOCTOU on a rule
		// nothing else enforces.
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if err := checkLive(before); err != nil {
			return err
		}

		spec := Spec{
			FromAccountID: before.From.AccountID,
			ToAccountID:   before.To.AccountID,
			Date:          edit.Date,
			Amount:        edit.Amount,
			Memo:          edit.Memo,
			CategoryID:    edit.CategoryID,
			Status:        edit.Status,
		}.withDefaults()

		from, to, err := b.guardSpec(spec)
		if err != nil {
			return err
		}
		plans := planLegs(from, to, spec, transferID)

		// Plans are ordered (From, To) and so are the legs, so each plan lands
		// on the leg it describes.
		if err := b.updateLeg(before.From, plans[0]); err != nil {
			return err
		}
		if err := b.updateLeg(before.To, plans[1]); err != nil {
			return err
		}

		res = &Result{
			TransferID: transferID,
			Kind:       ClassifyKind(from.Type, to.Type),
			From:       LegRef{Ledger: before.From.Ledger, RowID: before.From.RowID, AccountID: before.From.AccountID},
			To:         LegRef{Ledger: before.To.Ledger, RowID: before.To.RowID, AccountID: before.To.AccountID},
			Before:     before,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Reverse flips a transfer's direction, keeping its identity, amount, date, memo
// and category.
//
// It replaces the "flip the direction" behavior users reach today by editing an
// inv↔inv transfer's From/To in the dialog, which internally deletes and
// recreates both rows. Here it is two signed in-place updates.
func (s *Service) Reverse(transferID types.ID) (*Result, error) {
	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if err := checkLive(before); err != nil {
			return err
		}

		// Swapping the plans is the whole operation: the From leg's row now
		// holds the positive amount and vice versa. Each row keeps its account
		// and its transfer_account_id, which are unchanged by a direction flip.
		spec := Spec{
			FromAccountID: before.To.AccountID,
			ToAccountID:   before.From.AccountID,
			Date:          before.Date,
			Amount:        before.Amount,
			Memo:          before.Memo,
			CategoryID:    before.CategoryID,
			Status:        before.Status,
		}.withDefaults()

		from, to, err := b.guardSpec(spec)
		if err != nil {
			return err
		}
		plans := planLegs(from, to, spec, transferID)

		// plans[0] describes the new From (previously To) and plans[1] the new
		// To (previously From).
		if err := b.updateLeg(before.To, plans[0]); err != nil {
			return err
		}
		if err := b.updateLeg(before.From, plans[1]); err != nil {
			return err
		}

		res = &Result{
			TransferID: transferID,
			Kind:       ClassifyKind(from.Type, to.Type),
			From:       LegRef{Ledger: before.To.Ledger, RowID: before.To.RowID, AccountID: before.To.AccountID},
			To:         LegRef{Ledger: before.From.Ledger, RowID: before.From.RowID, AccountID: before.From.AccountID},
			Before:     before,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// SetStatus sets the same status on both legs.
func (s *Service) SetStatus(transferID types.ID, status transaction.Status) (*Result, error) {
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid transfer status: %q", status)
	}
	if status == transaction.StatusVoid {
		return nil, fmt.Errorf("use Void to void a transfer, not SetStatus")
	}

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if err := checkMutable(before); err != nil {
			return err
		}
		for _, leg := range before.Legs() {
			if err := b.setLegStatus(leg, status); err != nil {
				return err
			}
		}
		res = resultFrom(before)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// SetLegStatus sets the status of ONE leg, leaving the other alone.
//
// This is what the register's cleared toggle needs: clearing your side of a
// transfer says your bank has posted it, which is independent of whether the
// other account's side has. The toggle reaches transaction.Service.Update today,
// which rewrites the whole row and has no transfer-aware path at all.
//
// Reconciled legs are refused, but a reconciled leg on the OTHER side does not
// block this one — the two sides reconcile independently.
func (s *Service) SetLegStatus(legRowID types.ID, status transaction.Status) (*Result, error) {
	if !status.IsValid() {
		return nil, fmt.Errorf("invalid transfer status: %q", status)
	}
	if status == transaction.StatusVoid {
		return nil, fmt.Errorf("use Void to void a transfer, not SetLegStatus")
	}

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Resolve(legRowID)
		if err != nil {
			return err
		}
		if before.Shape == ShapeSplitLine {
			return &SplitLineError{TransferID: before.TransferID, ParentID: before.ParentTransactionID}
		}
		if before.Movement == MovementShares {
			return &ShareTransferError{TransferID: before.TransferID}
		}

		var target Leg
		found := false
		for _, leg := range before.Legs() {
			if leg.RowID == legRowID {
				target, found = leg, true
				break
			}
		}
		if !found {
			return fmt.Errorf("leg %s not found on transfer %s", legRowID.String(), before.TransferID.String())
		}
		if target.Status == transaction.StatusReconciled {
			return &ReconciledLegError{TransferID: before.TransferID, RowID: target.RowID}
		}

		if err := b.setLegStatus(target, status); err != nil {
			return err
		}
		res = resultFrom(before)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Void zeroes both legs of a transfer while keeping the rows, so the transfer
// stays visible in the register as a voided entry and can be restored.
//
// Only bank↔bank transfers can be voided: investment_transactions' status CHECK
// has no `void` value. The refusal is typed, replacing the misleading "expected 2
// transactions for transfer, found 1" a user gets today when voiding the bank leg
// of an inv↔reg transfer.
func (s *Service) Void(transferID types.ID) (*Result, error) {
	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if err := checkLive(before); err != nil {
			return err
		}
		if before.HasInvestmentLeg() {
			return &VoidNotSupportedError{TransferID: transferID, Kind: before.Kind}
		}

		for _, leg := range before.Legs() {
			row, err := b.txnRepo.GetByID(leg.RowID)
			if err != nil {
				return fmt.Errorf("failed to load leg %s for void: %w", leg.RowID.String(), err)
			}
			row.Amount = types.ZeroMoney
			row.Status = transaction.StatusVoid
			if err := b.txnRepo.Update(row); err != nil {
				return fmt.Errorf("failed to void leg %s: %w", leg.RowID.String(), err)
			}
		}

		res = resultFrom(before)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Restore puts a voided transfer's amounts, memos and statuses back.
//
// The values come from the caller's snapshot rather than being re-derived,
// because voiding destroys them. Snapshots are RowID-addressed: a voided pair has
// both amounts zeroed, so orientation cannot be recovered from the sign, and
// TransferPair.Validate's cross-references are a symmetric mutual pointer that
// cannot orient any pair either.
func (s *Service) Restore(transferID types.ID, legs []RestoreLeg) (*Result, error) {
	if len(legs) != 2 {
		return nil, fmt.Errorf("restore needs exactly 2 leg snapshots, got %d", len(legs))
	}

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if before.Shape == ShapeSplitLine {
			return &SplitLineError{TransferID: transferID, ParentID: before.ParentTransactionID}
		}
		if before.HasInvestmentLeg() {
			return &VoidNotSupportedError{TransferID: transferID, Kind: before.Kind}
		}

		byRow := map[types.ID]Leg{}
		for _, leg := range before.Legs() {
			byRow[leg.RowID] = leg
		}
		for _, snap := range legs {
			if _, ok := byRow[snap.RowID]; !ok {
				return fmt.Errorf("restore snapshot names row %s, which is not a leg of transfer %s",
					snap.RowID.String(), transferID.String())
			}
			row, err := b.txnRepo.GetByID(snap.RowID)
			if err != nil {
				return fmt.Errorf("failed to load leg %s for restore: %w", snap.RowID.String(), err)
			}
			row.Amount = snap.Amount
			row.Memo = snap.Memo
			row.Status = snap.Status
			if err := b.txnRepo.Update(row); err != nil {
				return fmt.Errorf("failed to restore leg %s: %w", snap.RowID.String(), err)
			}
		}

		res = resultFrom(before)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// RestoreLeg is one leg's pre-void state, addressed by row ID.
type RestoreLeg struct {
	RowID  types.ID
	Amount types.Money
	Memo   types.NullableString
	Status transaction.Status
}

// Delete removes both legs of a transfer.
func (s *Service) Delete(transferID types.ID) (*Result, error) {
	var res *Result
	if err := s.runInTx(func(b *Service) error {
		before, err := b.Get(transferID)
		if err != nil {
			return err
		}
		if err := checkMutable(before); err != nil {
			return err
		}
		for _, leg := range before.Legs() {
			if err := b.deleteLeg(leg); err != nil {
				return err
			}
		}
		res = resultFrom(before)
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Recreate re-creates a previously deleted transfer from a snapshot, reusing its
// original transfer_id so undo history and any external reference stay valid.
//
// transaction.Service.RecreateTransferPair routes through Service.Create per leg
// today and therefore skips pair validation entirely; this runs validatePair.
func (s *Service) Recreate(transferID types.ID, spec Spec) (*Result, error) {
	spec = spec.withDefaults()

	from, to, err := s.guardSpec(spec)
	if err != nil {
		return nil, err
	}
	plans := planLegs(from, to, spec, transferID)

	var res *Result
	if err := s.runInTx(func(b *Service) error {
		fromRef, err := b.insertLeg(transferID, plans[0])
		if err != nil {
			return err
		}
		toRef, err := b.insertLeg(transferID, plans[1])
		if err != nil {
			return err
		}
		if err := validatePair(transferID, fromRef, toRef, plans); err != nil {
			return err
		}
		res = &Result{
			TransferID: transferID,
			Kind:       ClassifyKind(from.Type, to.Type),
			From:       fromRef,
			To:         toRef,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// LinkExisting turns two pre-existing, unlinked rows into a transfer pair by
// stamping a shared transfer_id and mutual transfer_account_ids.
//
// This is what transferlink needs. It deliberately does NOT adopt Update's guard
// preamble: transferlink links rows that are already reconciled and rows dated
// before their account's opening date, and today it succeeds at both. Adding the
// guards here would be a silent capability reduction dressed as a refactor.
// LinkExisting changes who performs the write, not what may be linked.
// categoryID, when valid, is written to BOTH legs, which is how transferlink
// reconciles a pair where only one side was categorized (or the two disagreed).
// A zero categoryID leaves each leg's existing category untouched.
func (s *Service) LinkExisting(fromRowID, toRowID types.ID, categoryID types.NullableID) (types.ID, error) {
	if fromRowID == toRowID {
		return types.NilID, fmt.Errorf("cannot link a transaction to itself")
	}

	transferID := types.NewID()
	if err := s.runInTx(func(b *Service) error {
		fromRow, err := b.txnRepo.GetByID(fromRowID)
		if err != nil {
			return fmt.Errorf("failed to load transaction %s: %w", fromRowID.String(), err)
		}
		toRow, err := b.txnRepo.GetByID(toRowID)
		if err != nil {
			return fmt.Errorf("failed to load transaction %s: %w", toRowID.String(), err)
		}
		if fromRow.AccountID == toRow.AccountID {
			return &SameAccountError{AccountID: fromRow.AccountID}
		}

		fromRow.SetTransfer(transferID, toRow.AccountID)
		toRow.SetTransfer(transferID, fromRow.AccountID)
		if categoryID.Valid {
			fromRow.SetCategory(categoryID.ID)
			toRow.SetCategory(categoryID.ID)
		}

		if err := b.txnRepo.Update(fromRow); err != nil {
			return fmt.Errorf("failed to link transaction %s: %w", fromRowID.String(), err)
		}
		if err := b.txnRepo.Update(toRow); err != nil {
			return fmt.Errorf("failed to link transaction %s: %w", toRowID.String(), err)
		}
		return nil
	}); err != nil {
		return types.NilID, err
	}
	return transferID, nil
}

// resultFrom builds a Result describing a transfer that was mutated in place,
// carrying its pre-mutation state as Before.
func resultFrom(before *Transfer) *Result {
	return &Result{
		TransferID: before.TransferID,
		Kind:       before.Kind,
		From:       LegRef{Ledger: before.From.Ledger, RowID: before.From.RowID, AccountID: before.From.AccountID},
		To:         LegRef{Ledger: before.To.Ledger, RowID: before.To.RowID, AccountID: before.To.AccountID},
		Before:     before,
	}
}
