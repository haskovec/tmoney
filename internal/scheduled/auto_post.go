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
// AutoPost differs from posting.go in three ways that are deliberate, not
// accidental: it estimates a variable amount against the in-memory schedule that
// prior loop iterations already advanced rather than re-reading the row; a skip is
// not an error, so the transaction still commits the completion mark; and it
// advances per occurrence but persists the schedule ONCE after the loop, and only
// conditionally. Any collapse of the duplication must preserve all three.

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
		postErr := s.runInTx(func(b *Service) error {
			// Post all overdue occurrences for this scheduled transaction
			for !result.Skipped && !st.IsCompleted() && b.isAutoPostDue(st, today) {
				var txn *transaction.Transaction
				if st.IsTransfer() {
					// Single-line transfer: post template values exactly as a clean
					// linked transfer pair.
					if b.transferPort == nil {
						return fmt.Errorf("transfer auto-post requires the transfer service; scheduled.SetTransferPort was never called")
					}
					magnitude := st.Amount.Money.Abs()
					// The transfer owner stamps the schedule's memo and optional
					// category onto whichever legs can hold them.
					memo := ""
					if st.Memo.Valid {
						memo = st.Memo.String
					}
					transferID, regularLegID, err := b.transferPort.CreateTransfer(
						b.q(), st.AccountID, st.TransferAccountID.ID, st.NextDate, magnitude, memo, st.CategoryID,
					)
					if err != nil {
						return fmt.Errorf("failed to create transfer auto-post transaction: %w", err)
					}
					// Record the transfer_id so undo removes the PAIR. Undoing a
					// leg-at-a-time is refused by transaction.Service and would
					// orphan the counterpart regardless.
					result.TransferIDs = append(result.TransferIDs, transferID)
					// An inv↔inv occurrence has no regular-ledger leg to report.
					if !regularLegID.IsNil() && b.txnRepo != nil {
						if posted, gerr := b.txnRepo.GetByID(regularLegID); gerr == nil {
							txn = posted
						}
					}
				} else if len(st.Splits) > 0 {
					// Multi-line schedule: delegate to the multi-line create path
					// so transfer-line counterparts are minted and persisted.
					if b.txnSvc == nil {
						return fmt.Errorf("multi-line auto-post requires a transaction service; scheduled.NewService was called with txnSvc=nil")
					}
					built, err := b.buildMultiLineTransaction(st, st.NextDate)
					if err != nil {
						// A loan-recompute failure (paid off, negative-am, missing
						// APR, missing interest line) skips this schedule with a
						// reason — it must never abort the rest of the batch.
						if isLoanComputationError(err) {
							result.Skipped = true
							result.SkipReason = loanSkipReason(err)
							// A paid-off loan is terminal: mark it completed so it
							// stops surfacing as due.
							if errors.Is(err, ErrLoanPaidOff) {
								st.MarkCompleted()
							}
							break
						}
						return fmt.Errorf("failed to build multi-line auto-post transaction: %w", err)
					}
					if err := b.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
						return fmt.Errorf("failed to create multi-line auto-post transaction: %w", err)
					}
					txn = built.parent
				} else {
					// Determine the amount to use
					var txnAmount types.Money
					if st.HasAmount() {
						txnAmount = st.Amount.Money
					} else {
						// Variable amount - try to estimate
						estimated, estErr := b.estimateAmountForSchedule(st)
						if estErr != nil || estimated == nil {
							result.Skipped = true
							result.SkipReason = "variable amount with no estimate available"
							break
						}
						txnAmount = *estimated
					}

					// Create the transaction with the scheduled date (not today)
					txn = transaction.NewTransaction(st.AccountID, st.NextDate, txnAmount)

					// Copy optional fields
					if st.HasPayee() {
						txn.SetPayee(st.PayeeID.ID)
					}
					if st.HasCategory() {
						txn.SetCategory(st.CategoryID.ID)
					}
					if st.Memo.Valid && st.Memo.String != "" {
						txn.SetMemo(st.Memo.String)
					}

					if err := b.txnRepo.Create(txn); err != nil {
						return fmt.Errorf("failed to create auto-post transaction: %w", err)
					}
				}

				// txn is nil only for an inv↔inv transfer occurrence, which has no
				// regular-ledger row. Its transfer_id is already recorded above, so
				// undo can still reach it; appending nil here would make undo panic.
				if txn != nil {
					result.Transactions = append(result.Transactions, txn)
				}
				summary.PostedCount++

				// Advance the schedule
				st.AdvanceSchedule()
				// Payoff completion: a loan-shaped schedule whose balance reached
				// zero (e.g. a clamped final payment) is marked completed, which
				// also stops the loop.
				if err := b.finalizeLoanPayoff(st); err != nil {
					return fmt.Errorf("auto-post created transaction but failed to finalize loan payoff: %w", err)
				}
			}

			// Persist schedule mutations. A posted occurrence advanced the schedule;
			// a paid-off skip marked it completed. Other skips (closed account,
			// no-estimate, non-terminal loan errors) leave the schedule untouched
			// and due, so they are not persisted.
			if len(result.Transactions) > 0 || (result.Skipped && st.IsCompleted()) {
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

		if len(result.Transactions) > 0 || result.Skipped {
			summary.Results = append(summary.Results, result)
		}
	}

	return summary, nil
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
