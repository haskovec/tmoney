package scheduled

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Manual posting: turning one due occurrence of a schedule into real ledger rows
// because a person asked for it.
//
// Everything here is an ENTRY POINT in the sense of
// specs/design-service-decomposition.md section 2.3. An entry point opens the
// transaction exactly once, calls participants (post_occurrence.go) on the bound
// copy, and owns the commit boundary — the schedule advance, the payoff check and
// the persist. Participants never open a transaction, so an entry point is the
// only layer that can, and double-opening is structurally impossible.
//
// Auto-post is the other entry point onto the same participants; it lives in
// auto_post.go because its envelope is a loop rather than a single occurrence.

// Post creates a real transaction from a scheduled transaction and advances the schedule.
// If the scheduled transaction has no fixed amount, the provided amount is used.
// If the scheduled transaction has a fixed amount and an amount is provided, it uses the provided amount.
// For multi-line scheduled transactions (with child Splits), the amount override is
// not supported — per-instance amount edits go through the post-time preview dialog.
// Returns the created transaction.
func (s *Service) Post(id types.ID, amount *types.Money) (*transaction.Transaction, error) {
	txn, _, err := s.PostReturningSplits(id, amount)
	return txn, err
}

// PostReturningSplits behaves exactly like Post but also returns the child
// splits it persisted (nil for single-line schedules). The undo command for a
// plain post uses it to capture and replay the exact posted rows on redo:
// loan-shaped schedules recompute interest/principal from the loan's live
// balance, which can change between the original post and a later redo, so a
// redo that re-ran Post would produce different rows. Store-and-replay keeps
// redo deterministic.
func (s *Service) PostReturningSplits(id types.ID, amount *types.Money) (*transaction.Transaction, []*transaction.Split, error) {
	return s.postManually(id, nil, amount)
}

// PostWithDate creates a transaction from a scheduled transaction with a specific date.
// Useful when posting a due transaction on a date other than the scheduled next_date.
func (s *Service) PostWithDate(id types.ID, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	txn, _, err := s.postManually(id, &date, amount)
	return txn, err
}

// postManually is the shared envelope of Post, PostReturningSplits and
// PostWithDate: refuse the schedule if it cannot post, decide this occurrence's
// amount, then write the occurrence and advance the schedule inside ONE
// transaction so the posted rows and the next_date advance commit together.
// That single boundary is what closes the double-post window — a failure to
// advance rolls the post back rather than leaving a schedule that posts again on
// the next run.
//
// date is nil for the schedule's own next_date, or the caller's chosen date.
func (s *Service) postManually(id types.ID, date *types.Date, amount *types.Money) (*transaction.Transaction, []*transaction.Split, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, nil, err
	}
	if st.IsCompleted() {
		return nil, nil, &CompletedError{ID: id.String()}
	}

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, nil, err
	}

	if len(st.Splits) > 0 && amount != nil {
		return nil, nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
	}
	if st.IsTransfer() && s.txnRepo == nil {
		return nil, nil, fmt.Errorf("posting a transfer schedule requires a transaction repository; scheduled.NewService was called with txnRepo=nil")
	}

	occurrenceDate := st.NextDate
	if date != nil {
		occurrenceDate = *date
	}
	resolved, err := s.manualAmount(st, amount)
	if err != nil {
		return nil, nil, err
	}

	var out postedOccurrence
	if err := s.runInTx(func(b *Service) error {
		posted, perr := b.postOccurrence(st, occurrenceDate, resolved)
		if perr != nil {
			return perr
		}
		out = posted
		// Advance the schedule. If AdvanceSchedule returns false the series is
		// complete; either way the updated state is persisted below.
		st.AdvanceSchedule()
		// Payoff completion applies on every posting path: a loan-shaped schedule
		// whose balance reached zero is marked completed here, before the persist,
		// so MarkCompleted's field changes are saved. finalizeLoanPayoff reads the
		// loan balance through the bound accountRepo, so it sees the principal
		// counterpart this occurrence just posted. It is a no-op for any schedule
		// that is not loan-shaped, which includes every single-line and transfer
		// schedule — IsLoanShaped requires split lines.
		if ferr := b.finalizeLoanPayoff(st); ferr != nil {
			return fmt.Errorf("failed to finalize loan payoff: %w", ferr)
		}
		return b.repo.Update(st)
	}); err != nil {
		// A loan already paid off at post time is a terminal state: refuse the post
		// and mark the schedule completed on the spot, so an ad-hoc payoff transfer
		// cannot strand a never-postable due schedule. The transaction above rolled
		// back without writing anything, so this persist stands alone.
		if errors.Is(err, ErrLoanPaidOff) {
			st.MarkCompleted()
			_ = s.repo.Update(st)
		}
		return nil, nil, err
	}
	return out.txn, out.splits, nil
}

// manualAmount decides what a MANUAL post should pay for this occurrence.
//
// A caller override wins; a fixed-amount schedule needs no help; a multi-line
// schedule is priced from its own lines. What is left is a variable-amount
// single-line schedule, which is estimated from the STORED row — EstimateAmount
// re-reads it — and refused outright when no estimate can be produced. Refusing
// is the difference from auto-post, which treats the same condition as a skip
// (see autoPostAmount): a person who asked for a post is told why it did not
// happen, whereas a batch must keep going.
func (s *Service) manualAmount(st *Transaction, amount *types.Money) (*types.Money, error) {
	if amount != nil || st.HasAmount() || len(st.Splits) > 0 {
		return amount, nil
	}
	// A transfer schedule always carries a fixed amount (Transaction.Validate
	// requires one), so reaching here means the template is unusable.
	if st.IsTransfer() {
		return nil, &AmountRequiredError{ID: st.ID.String()}
	}
	estimated, err := s.EstimateAmount(st.ID)
	if err != nil || estimated == nil {
		return nil, &AmountRequiredError{ID: st.ID.String()}
	}
	return estimated, nil
}

// PostWithEdits creates a real transaction using a caller-supplied
// parent (and, for multi-line schedules, splits) and advances the
// schedule by one cadence. Unlike Post, this entry point lets the
// caller carry per-instance edits — e.g. a date or memo change made
// in the post-time preview dialog — without those edits leaking into
// the stored template.
//
// The parent transaction and splits must already reflect the user's
// edits (date / payee / category / amount / memo / status for single-
// line; header + line edits for multi-line). For multi-line callers,
// transfer-line splits carry only TransferAccountID; the underlying
// transaction service mints fresh TransferIDs and creates paired
// counterparts in the target accounts.
//
// It is an entry point like postManually, but it writes no occurrence built from
// the template, so it calls no participant: the rows it persists are the
// caller's.
//
// Schedule advancement uses the *template's* original next_date as
// the basis (st.AdvanceSchedule calls st.CalculateNextDate over
// st.NextDate before the user's edits are applied), so a one-off
// date edit in the preview never shifts the schedule's cadence.
//
// Returns the persisted parent transaction.
func (s *Service) PostWithEdits(id types.ID, txn *transaction.Transaction, splits []*transaction.Split) (*transaction.Transaction, error) {
	if s.txnSvc == nil {
		return nil, fmt.Errorf("PostWithEdits requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}
	if txn == nil {
		return nil, fmt.Errorf("PostWithEdits requires a non-nil parent transaction")
	}

	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if st.IsCompleted() {
		return nil, &CompletedError{ID: id.String()}
	}

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, err
	}

	// Stamp the parent transaction's ID onto every split. Callers
	// (e.g. the TUI preview dialog) typically build splits via
	// SplitDialog.buildSplits, which leaves TransactionID zero
	// because the parent isn't constructed at that point; the
	// transaction service's validation requires it.
	for _, sp := range splits {
		if sp != nil {
			sp.TransactionID = txn.ID
		}
	}

	// Post the (possibly edited) transaction and advance the schedule in one
	// transaction: the posted rows and the next_date advance commit together
	// (b.txnSvc is bound, so its Create/CreateWithSplits join this tx), closing
	// the double-post window. finalizeLoanPayoff reads the loan balance through
	// the bound accountRepo so it sees the just-posted principal counterpart.
	if err := s.runInTx(func(b *Service) error {
		if len(splits) > 0 {
			if err := b.txnSvc.CreateWithSplits(txn, splits); err != nil {
				return fmt.Errorf("failed to create transaction: %w", err)
			}
		} else {
			if err := b.txnSvc.Create(txn); err != nil {
				return fmt.Errorf("failed to create transaction: %w", err)
			}
		}

		st.AdvanceSchedule()
		// Payoff completion applies on every posting path, including this
		// flagship manual-preview flow: a penny-tweaked edit that brings the
		// loan to (or past) zero marks the schedule completed.
		if err := b.finalizeLoanPayoff(st); err != nil {
			return fmt.Errorf("failed to finalize loan payoff: %w", err)
		}
		return b.repo.Update(st)
	}); err != nil {
		return nil, err
	}
	return txn, nil
}

// Skip advances the schedule without creating a transaction.
// Useful when a scheduled payment is not made (e.g., skipping a bill payment).
func (s *Service) Skip(id types.ID) error {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return &CompletedError{ID: id.String()}
	}

	// Advance the schedule without creating a transaction
	st.AdvanceSchedule()

	// Update the scheduled transaction
	return s.repo.Update(st)
}
