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
	if !srcOld.TransferID.Valid {
		return nil, fmt.Errorf("UpdateTransferShares: txn %s is not a share transfer", oldSourceTxnID)
	}
	// A closed account is frozen — refuse before any destructive reverse/delete.
	// Guard BOTH legs of the EXISTING transfer (old source + old destination,
	// which lives on srcOld.TransferAccountID) as well as both NEW target
	// accounts, mirroring the transaction package's checkTransferEditable. A
	// share-only account can be closed (the balance check is cash-only), so the
	// old destination must be checked or its leg would be silently
	// reversed/deleted below.
	if err := s.core.ensureAccountOpen(srcOld.AccountID); err != nil {
		return nil, err
	}
	if srcOld.TransferAccountID.Valid {
		if err := s.core.ensureAccountOpen(srcOld.TransferAccountID.ID); err != nil {
			return nil, err
		}
	}
	if err := s.core.ensureAccountOpen(sourceAccountID); err != nil {
		return nil, err
	}
	if err := s.core.ensureAccountOpen(destAccountID); err != nil {
		return nil, err
	}
	// Find the destination side by transfer_id.
	var dstOld *Transaction
	all, err := s.core.repo.ListByAccount(srcOld.TransferAccountID.ID, TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list destination transfers: %w", err)
	}
	for _, t := range all {
		if t.TransferID.Valid && t.TransferID.ID == srcOld.TransferID.ID && t.ID != srcOld.ID {
			dstOld = t
			break
		}
	}

	// Heal both legs' stored state in their own committed txs before the edit tx,
	// mirroring what TransferShares does when called standalone. The bound
	// TransferShares inside the tx skips its own re-heal.
	if err := s.core.healInOwnTx(sourceAccountID, securityID); err != nil {
		return nil, err
	}
	if err := s.core.healInOwnTx(destAccountID, securityID); err != nil {
		return nil, err
	}

	// Reverse both legs, delete both old rows, and create the new pair in one
	// transaction — the edit fully lands or the original pair is left intact.
	var result *ShareTransferResult
	if err := s.core.runInTx(func(b *Service) error {
		if err := b.reverseTxnEffects(srcOld); err != nil {
			return err
		}
		if dstOld != nil {
			if err := b.reverseTxnEffects(dstOld); err != nil {
				return err
			}
			if err := b.repo.Delete(dstOld.ID); err != nil {
				return fmt.Errorf("failed to delete destination transfer for edit: %w", err)
			}
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
