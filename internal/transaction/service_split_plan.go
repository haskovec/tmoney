package transaction

import (
	"github.com/haskovec/tmoney/internal/types"
)

// Planning for ReplaceSplits.
//
// ReplaceSplits cannot decide row by row: whether a transfer line is retained,
// re-categorized or dropped depends on the whole incoming slice. planSplitReplacement
// computes that verdict for the entire replacement up front, and
// preflightSplitReplacement rejects the plan before any write happens, so a refusal
// never leaves a partially rewritten split set behind.

// retainedTransferChange records a transfer line kept across a ReplaceSplits
// whose amount or category changed, so its counterpart can be re-synced.
// amountChanged distinguishes a category-only change (which never touches an
// investment-side counterpart) from an amount change.
type retainedTransferChange struct {
	transferID    types.ID
	newAmount     types.Money
	newCategory   types.NullableID
	amountChanged bool
}

// splitReplacementPlan captures the transfer-line diff computed by
// planSplitReplacement for ReplaceSplits.
type splitReplacementPlan struct {
	// Counterparts to delete (old transfer lines with no match in the new set).
	removedTransferIDs []types.ID
	// Retained transfer lines whose amount or category changed (re-synced onto
	// the counterpart).
	retainedChanged []retainedTransferChange
	// New transfer lines with no match in the old set. Each already has a
	// transfer_id assigned; a counterpart must be minted for it.
	addedSplits []*Split
}

// planSplitReplacement diffs the desired transfer lines against the current
// ones and assigns transfer_ids onto the new splits in place: a retained line
// (matched first by transfer_id, then by target account) inherits its match's
// transfer_id; an added line keeps a caller-supplied transfer_id or is minted
// a fresh one. A retained line whose amount or category changed is recorded so
// its counterpart is re-synced. Plain categorized (non-transfer) lines are
// ignored (they carry no counterpart and are recreated wholesale by
// ReplaceSplits).
func planSplitReplacement(oldSplits, newSplits []*Split) splitReplacementPlan {
	type oldTransfer struct {
		split    *Split
		consumed bool
	}
	olds := make([]*oldTransfer, 0, len(oldSplits))
	for _, os := range oldSplits {
		if os.TransferAccountID.Valid {
			olds = append(olds, &oldTransfer{split: os})
		}
	}

	matchByTransferID := func(id types.NullableID) *oldTransfer {
		if !id.Valid {
			return nil
		}
		for _, o := range olds {
			if !o.consumed && o.split.TransferID.Valid && o.split.TransferID.ID == id.ID {
				return o
			}
		}
		return nil
	}
	matchByTarget := func(acctID types.ID) *oldTransfer {
		for _, o := range olds {
			if !o.consumed && o.split.TransferAccountID.ID == acctID {
				return o
			}
		}
		return nil
	}

	var plan splitReplacementPlan
	for _, ns := range newSplits {
		if !ns.TransferAccountID.Valid {
			continue
		}
		match := matchByTransferID(ns.TransferID)
		if match == nil {
			match = matchByTarget(ns.TransferAccountID.ID)
		}
		if match != nil {
			match.consumed = true
			// Retained: adopt the existing counterpart's transfer_id.
			ns.TransferID = match.split.TransferID
			amountChanged := !ns.Amount.Equal(match.split.Amount)
			categoryChanged := ns.CategoryID != match.split.CategoryID
			if amountChanged || categoryChanged {
				plan.retainedChanged = append(plan.retainedChanged, retainedTransferChange{
					transferID:    match.split.TransferID.ID,
					newAmount:     ns.Amount,
					newCategory:   splitCategoryNullable(ns),
					amountChanged: amountChanged,
				})
			}
			continue
		}
		// Added: mint a transfer_id unless the caller replayed one (e.g. the
		// void-undo restore path carries the captured lines' original ids).
		if !ns.TransferID.Valid {
			ns.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
		}
		plan.addedSplits = append(plan.addedSplits, ns)
	}

	for _, o := range olds {
		if !o.consumed && o.split.TransferID.Valid {
			plan.removedTransferIDs = append(plan.removedTransferIDs, o.split.TransferID.ID)
		}
	}
	return plan
}

// preflightSplitReplacement verifies every counterpart mutation the plan
// implies can succeed, so ReplaceSplits fails before it deletes any split row.
// Counterparts that will be deleted or re-synced (amount or category change)
// must not be reconciled; added transfer lines must target a routable account.
func (s *Service) preflightSplitReplacement(plan splitReplacementPlan) error {
	for _, transferID := range plan.removedTransferIDs {
		if err := s.ensureCounterpartNotReconciled(transferID); err != nil {
			return err
		}
	}
	for _, change := range plan.retainedChanged {
		if err := s.ensureRetainedCounterpartMutable(change.transferID, change.amountChanged); err != nil {
			return err
		}
	}
	for _, split := range plan.addedSplits {
		if _, err := s.ensureTransferTargetRoutable(split.TransferAccountID.ID); err != nil {
			return err
		}
	}
	return nil
}
