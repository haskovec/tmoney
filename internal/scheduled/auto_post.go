package scheduled

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Catch-up posting: find every schedule that has come due since it was last
// posted and post each missed occurrence.
//
// AutoPost is an ENTRY POINT onto the same participants the manual paths use
// (post_occurrence.go). It differs from posting.go only in its envelope, and the
// three differences are deliberate:
//
//   - it prices a variable amount against the IN-MEMORY schedule that earlier
//     occurrences of this catch-up already advanced, and against the rows they
//     just posted, rather than re-reading the stored row;
//   - a skip is not an error: the loop breaks and returns nil so the transaction
//     still commits any completion mark, instead of rolling the candidate back;
//   - it advances per occurrence but persists the schedule ONCE after the loop,
//     and only conditionally.
//
// All three now live here, in the envelope, which is exactly why the shared
// participants can be shared: they take the amount as a parameter, classify no
// skip, and persist no schedule.

// AutoPostResult describes the outcome of an auto-post operation for a single scheduled transaction.
type AutoPostResult struct {
	ScheduledTransactionID types.ID
	// Transactions holds the posted REGULAR-ledger rows. It never contains nil:
	// an investment↔investment transfer occurrence has no regular row at all, and
	// appending nil for it used to make undo panic on a nil dereference.
	Transactions []*transaction.Transaction
	// TransferIDs holds the transfer_id of each transfer occurrence posted, so
	// undo can address the PAIR rather than a single leg. Deleting one leg through
	// transaction.Service is refused outright, and would orphan the counterpart
	// even if it were not.
	TransferIDs    []types.ID
	BeforeSchedule *Transaction // Schedule state before auto-posting (for undo)
	Skipped        bool         // True if skipped due to variable amount with no estimate
	SkipReason     string       // Reason for skipping
}

// AutoPostSummary contains the results of running auto-post on file open.
type AutoPostSummary struct {
	Results      []AutoPostResult
	PostedCount  int // Total number of transactions created
	SkippedCount int // Number of scheduled transactions skipped
}

// AutoPost finds all due auto-post scheduled transactions and posts them.
// This should be called on file open (both TUI and CLI).
// Multiple overdue occurrences are posted, each with their correct scheduled date.
// Variable-amount transactions without an estimate are skipped.
func (s *Service) AutoPost() (*AutoPostSummary, error) {
	summary := &AutoPostSummary{}

	candidates, err := s.repo.ListAutoPostDue()
	if err != nil {
		return nil, fmt.Errorf("failed to list auto-post due transactions: %w", err)
	}

	today := types.Today()

	for _, st := range candidates {
		result := AutoPostResult{
			ScheduledTransactionID: st.ID,
		}

		// Capture schedule state before any modifications (deep copy for undo)
		beforeSchedule := *st
		result.BeforeSchedule = &beforeSchedule

		// Skip — don't error the batch — a schedule that references a closed
		// account; it is left due and unadvanced (decision 8). This is a read,
		// so it runs outside the per-candidate tx.
		if cerr := s.ensureNoClosedAccounts(st); cerr != nil {
			var closedErr *ClosedAccountError
			if !errors.As(cerr, &closedErr) {
				return nil, cerr
			}
			result.Skipped = true
			result.SkipReason = "references a closed account"
		}

		// Skip — again, don't error the batch — a transfer schedule carrying a
		// category its pair cannot store. Posting it can only fail at the transfer
		// owner, and that refusal is neither a closed-account nor a loan error, so
		// it used to abort the entire batch: one unpostable schedule stopped every
		// other schedule from posting that day. HealTransferCategories clears the
		// cause on file open; this guards the window in which changing an account's
		// type creates a fresh one mid-session.
		if !result.Skipped && s.transferCategoryUnsupported(st) {
			result.Skipped = true
			result.SkipReason = "investment-to-investment transfer cannot carry a category"
		}

		// One transaction per candidate: every overdue occurrence's posted rows
		// AND the schedule's next_date advance commit together, so a failure
		// mid-candidate rolls back only this schedule — the other candidates'
		// already-committed posts are untouched. Everything reachable inside the
		// closure runs on the tx-bound copy b: b.txnSvc (its
		// CreateTransfer/CreateWithSplits join this tx), b.txnRepo,
		// b.buildMultiLineTransaction / b.finalizeLoanPayoff (loan balance reads
		// on the bound accountRepo see prior in-tx occurrences), and
		// b.estimateAmountForSchedule (routes through b.q()). The skip-with-reason
		// paths break the loop and return nil so the tx still commits the
		// completion mark; only hard errors roll back and abort the batch.
		// occurrences counts what this candidate actually posted. It is NOT
		// len(result.Transactions): an occurrence can post real rows and still
		// contribute nothing to that list. It is declared outside the closure
		// because the persist gate and the reporting gate below both read it.
		occurrences := 0

		postErr := s.runInTx(func(b *Service) error {
			// Post all overdue occurrences for this scheduled transaction
			for !result.Skipped && !st.IsCompleted() && b.isAutoPostDue(st, today) {
				// Price this occurrence before writing anything: a variable-amount
				// schedule with no usable estimate is a skip, not a failure, and a
				// skip must not roll the candidate back.
				amount, priced := b.autoPostAmount(st)
				if !priced {
					result.Skipped = true
					result.SkipReason = "variable amount with no estimate available"
					break
				}

				posted, err := b.postOccurrence(st, st.NextDate, amount)
				if err != nil {
					// A loan-recompute failure (paid off, negative-am, missing APR,
					// missing interest line) skips this schedule with a reason — it
					// must never abort the rest of the batch.
					if isLoanComputationError(err) {
						result.Skipped = true
						result.SkipReason = loanSkipReason(err)
						// A paid-off loan is terminal: mark it completed so it stops
						// surfacing as due.
						if errors.Is(err, ErrLoanPaidOff) {
							st.MarkCompleted()
						}
						break
					}
					return err
				}

				// Record the transfer_id so undo removes the PAIR. Undoing a
				// leg-at-a-time is refused by transaction.Service and would orphan
				// the counterpart regardless.
				if !posted.transferID.IsNil() {
					result.TransferIDs = append(result.TransferIDs, posted.transferID)
				}
				// posted.txn is nil only for an inv↔inv transfer occurrence, which
				// has no regular-ledger row. Its transfer_id is already recorded
				// above, so undo can still reach it; appending nil would make undo
				// panic on a nil dereference.
				if posted.txn != nil {
					result.Transactions = append(result.Transactions, posted.txn)
				}
				// Count OCCURRENCES, not posted rows. An investment↔investment
				// transfer occurrence writes both its legs to the investment ledger
				// and so contributes no regular row at all; keying the persist
				// below off result.Transactions meant its advance was never
				// written, and the transfer re-posted on every file open.
				occurrences++
				summary.PostedCount++

				// Advance the schedule. A false return means this occurrence was
				// the last one the schedule allows.
				advanced := st.AdvanceSchedule()
				// Payoff completion: a loan-shaped schedule whose balance reached
				// zero (e.g. a clamped final payment) is marked completed, which
				// also stops the loop.
				if err := b.finalizeLoanPayoff(st); err != nil {
					return fmt.Errorf("auto-post created transaction but failed to finalize loan payoff: %w", err)
				}
				// Stop on the schedule's own terminal signal as well as on the loop
				// condition above. The two are deliberately independent: the loop
				// condition asks IsCompleted, and if that ever disagrees with
				// AdvanceSchedule again this loop must still terminate rather than
				// spin inside an open transaction.
				if !advanced {
					break
				}
			}

			// Persist schedule mutations. A posted occurrence advanced the schedule;
			// a paid-off skip marked it completed. Other skips (closed account,
			// no-estimate, non-terminal loan errors) leave the schedule untouched
			// and due, so they are not persisted.
			if occurrences > 0 || (result.Skipped && st.IsCompleted()) {
				if err := b.repo.Update(st); err != nil {
					return fmt.Errorf("failed to update schedule after auto-post: %w", err)
				}
			}
			return nil
		})
		if postErr != nil {
			return nil, postErr
		}

		if result.Skipped {
			summary.SkippedCount++
		}

		// Report on the same occurrence count the persist used. Undo reaches a
		// candidate's posted rows only through summary.Results, and it already
		// looks at both Transactions and TransferIDs — so a candidate whose only
		// output was an investment↔investment pair must appear here, or its undo
		// entry silently has nothing to undo.
		if occurrences > 0 || result.Skipped {
			summary.Results = append(summary.Results, result)
		}
	}

	return summary, nil
}

// autoPostAmount decides what one AUTO-POSTED occurrence should pay, and reports
// whether it could be priced at all.
//
// It is the auto-post twin of manualAmount, and differs from it in exactly the
// two ways the two engines are allowed to differ. It estimates against the
// IN-MEMORY schedule — which earlier occurrences of this catch-up have already
// advanced — and through the bound receiver, so the estimate sees the rows those
// occurrences just posted; a manual post re-reads the stored row instead. And an
// unpriceable schedule reports false rather than raising AmountRequiredError,
// because a batch skips what it cannot post and keeps going.
//
// A nil amount means "use the template's own", which is the answer for every
// shape that is not a variable-amount single-line schedule.
func (b *Service) autoPostAmount(st *Transaction) (*types.Money, bool) {
	if st.IsTransfer() || len(st.Splits) > 0 || st.HasAmount() {
		return nil, true
	}
	estimated, err := b.estimateAmountForSchedule(st)
	if err != nil || estimated == nil {
		return nil, false
	}
	return estimated, true
}

// isAutoPostDue checks if a scheduled transaction should be auto-posted based on lead days.
func (s *Service) isAutoPostDue(st *Transaction, today types.Date) bool {
	effectiveDate := st.NextDate.AddDays(-st.PostLeadDays)
	return !effectiveDate.After(today)
}

// estimateAmountForSchedule estimates the amount for a variable-amount scheduled transaction.
// This avoids re-fetching from the DB since we already have the scheduled transaction in memory.
func (s *Service) estimateAmountForSchedule(st *Transaction) (*types.Money, error) {
	if st.HasAmount() {
		return &st.Amount.Money, nil
	}

	if !st.AmountEstimateCount.Valid || st.AmountEstimateCount.Int64 <= 0 {
		return nil, nil
	}

	if !st.HasPayee() {
		return nil, nil
	}

	count := int(st.AmountEstimateCount.Int64)
	transactions, err := s.getRecentTransactionsByPayee(st.AccountID, st.PayeeID.ID, count)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, nil
	}

	var total types.Money
	for _, txn := range transactions {
		total = total.Add(txn.Amount)
	}

	average := total.Div(int64(len(transactions)))
	return &average, nil
}
