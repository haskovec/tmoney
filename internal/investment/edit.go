package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/types"
)

// Editing an investment transaction.
//
// Every method here is the same shape: heal the stored position/lot state in its
// own committed transaction, then — inside ONE transaction — reverse and delete
// the old row and re-create it from the new values, then reconcile the auto-price
// afterwards. Re-creating through the ordinary create path is what keeps an
// edited transaction indistinguishable from one entered correctly the first time.
//
// The methods move to EditService in the next step of this file's history; the
// reverse helpers they call stay on Service, in reverse.go, because delete needs
// them as well.

// UpdateBuy edits an existing buy transaction by reversing its
// position/lot effect, deleting the old record, and creating a new one
// with the supplied parameters. The reverse, delete, and re-create run in one
// transaction, so the edit either fully lands or the original is left intact —
// there is no partial "reversed but not reapplied" state to compensate for.
// EditService owns the ten edit entry points. It is the third and last type
// extracted out of investment.Service, and the only one that must share a
// transaction with the core rather than merely joining a caller's.
//
// It holds the core service and nothing else, which is mechanism 1 from design
// section 2.1: A holds B and rebinds it. That mechanism needs an ACYCLIC A -> B,
// and here it is acyclic by measurement — every Update* re-creates by calling a
// create method on the bound core (b.Buy, b.Sell, b.Dividend, ...), and nothing
// in the create path calls an Update*. A cycle could not be expressed this way:
// two types holding each other is a construction-order cycle whose InTx would
// recurse.
//
// The reverse helpers these methods depend on stay on Service, in reverse.go,
// because DeleteTransaction needs them too. That is why the old update_edit.go
// was split by owner rather than by file.
type EditService struct {
	core *Service
}

// NewEditService creates the edit family over an existing investment service.
func NewEditService(core *Service) *EditService {
	return &EditService{core: core}
}

// InTx returns a copy bound to tx by rebinding the ONE field it holds. Every
// write an edit performs goes through that core, so rebinding it is sufficient
// and is checkable at a glance — which is the point of holding one field.
//
// Binding matters here in a way it did not for the other two extractions: the
// core's runInTx JOINS when the core is already bound, so an edit invoked on a
// bound EditService runs inside the caller's transaction instead of opening a
// second one. Opening a second one would deadlock db.WithTx's mutex.
func (s *EditService) InTx(tx db.Queryer) *EditService {
	c := *s
	c.core = s.core.InTx(tx)
	return &c
}

func (s *EditService) UpdateBuy(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
) (*Transaction, error) {
	// Heal stored position/lot state for the target (account, security) in its
	// own committed tx before the edit tx, mirroring what Buy does when called
	// standalone. The bound Buy inside the tx skips its own re-heal.
	if err := s.core.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.Buy(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo)
		return err
	}); err != nil {
		return nil, err
	}
	// Reconcile the auto-price at the old (security, date): drop it if this edit
	// orphaned it, or re-point it to a surviving same-day transaction. Best-effort
	// cosmetic cleanup, deliberately outside the edit tx.
	if old.SecurityID.Valid {
		s.core.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateSell edits an existing sell transaction. The reverse, delete, and
// re-create run in one transaction — the edit fully lands or the original is
// left intact.
func (s *EditService) UpdateSell(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	if err := s.core.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.Sell(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo, lotAllocations)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.core.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateFeeLiquidation edits an existing fee-via-liquidation transaction by
// reversing its share/lot effect, deleting the old record, and re-creating it
// with the supplied parameters. fee_liquidation has no net cash effect (the
// whole total_amount is the fee), so only share counts/lots are reversed —
// reverseTxnEffects routes fee_liquidation through the same share-removal arm as
// sell, so this mirrors UpdateSell exactly. The reverse, delete, and re-create
// run in one transaction — the edit fully lands or the original is left intact.
//
// FeeLiquidation computes its FIFO lot allocation from the post-reverse lot
// state: called on the bound service below, its lookups see the uncommitted
// reverse, so growing the share count past the pre-reverse remaining works.
func (s *EditService) UpdateFeeLiquidation(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	commission types.Money,
	memo string,
	lotAllocations []SellLotAllocation,
) (*Transaction, error) {
	if err := s.core.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.FeeLiquidation(accountID, securityID, date, shares, totalAmount, pricePerShare, commission, memo, lotAllocations)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.core.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateReinvestDividend edits an existing reinvest-dividend transaction. The
// reverse, delete, and re-create run in one transaction — the edit fully lands
// or the original is left intact.
func (s *EditService) UpdateReinvestDividend(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	shares types.Quantity,
	totalAmount *types.Money,
	pricePerShare *types.Money,
	memo string,
) (*Transaction, error) {
	if err := s.core.healInOwnTx(accountID, securityID); err != nil {
		return nil, err
	}
	var old, newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		var err error
		if old, err = b.loadAndReverseForEdit(oldID); err != nil {
			return err
		}
		newTxn, err = b.ReinvestDividend(accountID, securityID, date, shares, totalAmount, pricePerShare, memo)
		return err
	}); err != nil {
		return nil, err
	}
	if old.SecurityID.Valid {
		s.core.cleanupAutoPrice(old.SecurityID.ID, old.Date)
	}
	return newTxn, nil
}

// UpdateDividend edits an existing cash dividend transaction. Dividends have no
// position/lot effect, so the flow is delete-old + create-new; both writes run
// in one transaction so a create failure leaves the original row intact.
func (s *EditService) UpdateDividend(
	oldID types.ID,
	accountID, securityID types.ID,
	date types.Date,
	amount types.Money,
	memo string,
) (*Transaction, error) {
	if err := s.core.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Dividend(accountID, securityID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateDeposit edits an existing deposit transaction. Delete-old + create-new
// commit in one transaction.
func (s *EditService) UpdateDeposit(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.core.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Deposit(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateWithdrawal edits an existing withdrawal transaction. Delete-old +
// create-new commit in one transaction.
func (s *EditService) UpdateWithdrawal(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.core.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Withdrawal(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateFee edits an existing fee transaction. Delete-old + create-new commit in
// one transaction.
func (s *EditService) UpdateFee(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.core.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Fee(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateInterest edits an existing interest transaction. Delete-old + create-new
// commit in one transaction.
func (s *EditService) UpdateInterest(oldID types.ID, accountID types.ID, date types.Date, amount types.Money, memo string) (*Transaction, error) {
	if err := s.core.guardEditByOldID(oldID); err != nil {
		return nil, err
	}
	var newTxn *Transaction
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.repo.Delete(oldID); err != nil {
			return fmt.Errorf("failed to delete transaction for edit: %w", err)
		}
		var err error
		newTxn, err = b.Interest(accountID, date, amount, memo)
		return err
	}); err != nil {
		return nil, err
	}
	return newTxn, nil
}

// UpdateTransferShares edits an existing share transfer between two
// investment accounts. Both sides are reversed before creating the new pair.
//
// oldSourceTxnID must be the sending leg of an intact pair. The receiving leg
// is refused with a ShareTransferDestinationLegError: callers pass the
// account the edit starts from as the new source, so an edit begun on the
// destination would reverse the transfer's direction. A pair that is not
// intact — no other account on the row, no other leg, or a leg in another
// account — is refused with a BrokenShareTransferError before anything is
// reversed, so a missing leg can no longer leave its shares behind.
func (s *EditService) UpdateTransferShares(
	oldSourceTxnID types.ID,
	sourceAccountID, destAccountID types.ID,
	date types.Date,
	securityID types.ID,
	shares types.Quantity,
	memo string,
	lotAllocations []SellLotAllocation,
) (*ShareTransferResult, error) {
	srcOld, err := s.core.repo.GetByID(oldSourceTxnID)
	if err != nil {
		return nil, fmt.Errorf("failed to load source transfer for edit: %w", err)
	}
	if !srcOld.TransferID.Valid || srcOld.Type != TransactionTypeTransferShares {
		return nil, fmt.Errorf("UpdateTransferShares: txn %s is not a share transfer", oldSourceTxnID)
	}
	dstOld, err := s.core.shareTransferCounterpart(srcOld)
	if err != nil {
		return nil, err
	}

	// A closed account is frozen — refuse before any destructive reverse/delete.
	// Guard both legs of the existing transfer and both new target accounts,
	// mirroring the transfer owner's checkTransferEditable. A share-only
	// account can be closed (the balance check is cash-only), so the old
	// destination must be checked or its leg would be silently reversed/deleted
	// below.
	for _, id := range []types.ID{srcOld.AccountID, dstOld.AccountID, sourceAccountID, destAccountID} {
		if err := s.core.ensureAccountOpen(id); err != nil {
			return nil, err
		}
	}

	// Heal every account/security pair the edit touches, old and new, each in
	// its own committed tx before the edit tx, mirroring what TransferShares
	// does when called standalone. Reverse must run on repaired lots of the
	// OLD accounts; the bound TransferShares inside the tx skips its re-heal,
	// so the NEW accounts are repaired here too.
	oldSecurityID := srcOld.SecurityID.ID
	healed := make(map[[2]types.ID]bool)
	for _, pair := range [][2]types.ID{
		{srcOld.AccountID, oldSecurityID},
		{dstOld.AccountID, oldSecurityID},
		{sourceAccountID, securityID},
		{destAccountID, securityID},
	} {
		if healed[pair] {
			continue
		}
		healed[pair] = true
		if err := s.core.healInOwnTx(pair[0], pair[1]); err != nil {
			return nil, err
		}
	}

	// Reverse both legs, delete both old rows, and create the new pair in one
	// transaction — the edit fully lands or the original pair is left intact.
	var result *ShareTransferResult
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.reverseTxnEffects(srcOld); err != nil {
			return err
		}
		if err := b.reverseTxnEffects(dstOld); err != nil {
			return err
		}
		if err := b.repo.Delete(dstOld.ID); err != nil {
			return fmt.Errorf("failed to delete destination transfer for edit: %w", err)
		}
		if err := b.repo.Delete(oldSourceTxnID); err != nil {
			return fmt.Errorf("failed to delete source transfer for edit: %w", err)
		}
		var terr error
		result, terr = b.TransferShares(sourceAccountID, destAccountID, securityID, date, shares, memo, lotAllocations)
		return terr
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// shareTransferCounterpart checks that src is the sending leg of an intact
// share transfer and returns the receiving leg. The pair is found by
// transfer_id, not by listing the other account's rows.
func (s *Service) shareTransferCounterpart(src *Transaction) (*Transaction, error) {
	broken := func(reason string) error {
		return &BrokenShareTransferError{ID: src.ID.String(), TransferID: src.TransferID.ID.String(), Reason: reason}
	}
	if !src.TransferAccountID.Valid {
		return nil, broken("the row names no other account")
	}
	if !src.SecurityID.Valid {
		return nil, broken("the row names no security")
	}
	switch {
	case src.IsShareTransferDestination():
		return nil, &ShareTransferDestinationLegError{
			ID:              src.ID.String(),
			TransferID:      src.TransferID.ID.String(),
			SourceAccountID: src.TransferAccountID.ID,
		}
	case !src.IsShareTransferSource():
		return nil, broken("it has no cost basis, so its direction is unknown")
	}

	legs, err := s.repo.ListByTransferID(src.TransferID.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load share transfer legs: %w", err)
	}
	var others []*Transaction
	for _, t := range legs {
		if t.ID != src.ID {
			others = append(others, t)
		}
	}
	if len(others) != 1 {
		return nil, broken(fmt.Sprintf("it has %d other legs, want 1", len(others)))
	}
	dst := others[0]
	if dst.AccountID != src.TransferAccountID.ID || !dst.IsShareTransferDestination() {
		return nil, broken("the other leg is not the receiving side in the named account")
	}
	return dst, nil
}
