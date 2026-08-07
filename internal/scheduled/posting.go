package scheduled

import (
	"errors"
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Turning a due schedule into real ledger rows.
//
// A schedule posts in one of three shapes — a single-line transaction, a split
// (multi-line) transaction, or a transfer — and each entry point advances the
// schedule and persists it once the rows are written.
//
// NOTE: AutoPost in auto_post.go does NOT call these helpers. It re-implements all
// three shapes inline against its own loop. Collapsing the two engines into one is
// the next phase of specs/design-service-decomposition.md (section 5); the reason
// naive delegation is wrong is written up in section 5.2 and is semantic, not a
// transaction-nesting problem.

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
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, nil, &CompletedError{ID: id.String()}
	}

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, nil, err
	}

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		return s.postMultiLine(st, st.NextDate)
	}

	txn, err := s.postSingleLine(st, st.NextDate, amount)
	return txn, nil, err
}

// PostWithDate creates a transaction from a scheduled transaction with a specific date.
// Useful when posting a due transaction on a date other than the scheduled next_date.
func (s *Service) PostWithDate(id types.ID, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check if schedule is already completed
	if st.IsCompleted() {
		return nil, &CompletedError{ID: id.String()}
	}

	// A schedule may not post into a closed account (manual post refuses with a
	// clear error and the schedule stays due).
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return nil, err
	}

	if len(st.Splits) > 0 {
		if amount != nil {
			return nil, fmt.Errorf("amount override is not supported on multi-line scheduled transactions; use the preview dialog to edit per-instance amounts")
		}
		txn, _, err := s.postMultiLine(st, date)
		return txn, err
	}

	return s.postSingleLine(st, date, amount)
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

// postSingleLine creates a single-line transaction from a legacy scheduled
// template and advances the schedule. The returned transaction is the
// newly-created real transaction.
func (s *Service) postSingleLine(st *Transaction, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	// Single-line transfer schedules create a clean linked transfer pair.
	if st.IsTransfer() {
		return s.postSingleLineTransfer(st, date, amount)
	}

	// Determine the amount to use
	var txnAmount types.Money
	switch {
	case amount != nil:
		txnAmount = *amount
	case st.HasAmount():
		txnAmount = st.Amount.Money
	default:
		// Variable amount with no amount provided - try to estimate
		estimated, err := s.EstimateAmount(st.ID)
		if err != nil {
			return nil, &AmountRequiredError{ID: st.ID.String()}
		}
		if estimated == nil {
			return nil, &AmountRequiredError{ID: st.ID.String()}
		}
		txnAmount = *estimated
	}

	// Create the transaction with the specified date
	txn := transaction.NewTransaction(st.AccountID, date, txnAmount)

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

	// Create the transaction and advance the schedule in one transaction: the
	// posted row and the next_date advance commit together, so a failure to
	// advance rolls back the post too (no double-post window).
	if err := s.runInTx(func(b *Service) error {
		if err := b.txnRepo.Create(txn); err != nil {
			return fmt.Errorf("failed to create transaction: %w", err)
		}
		// Advance the schedule. If AdvanceSchedule returns false the series is
		// complete; either way we persist the updated state below.
		st.AdvanceSchedule()
		return b.repo.Update(st)
	}); err != nil {
		return nil, err
	}

	return txn, nil
}

// postSingleLineTransfer creates a clean linked transfer pair from a
// single-line transfer schedule (account_id = From, transfer_account_id = To)
// and advances the schedule. The schedule stores the amount as the signed
// effect on the source account (negative); CreateTransfer wants a positive
// magnitude, so the amount is taken as an absolute value. An optional override
// (the post-time preview's edited amount) replaces the stored estimate for
// this one occurrence without changing the template.
func (s *Service) postSingleLineTransfer(st *Transaction, date types.Date, amount *types.Money) (*transaction.Transaction, error) {
	if s.transferPort == nil {
		return nil, fmt.Errorf("posting a transfer schedule requires the transfer service; scheduled.SetTransferPort was never called")
	}
	if s.txnRepo == nil {
		return nil, fmt.Errorf("posting a transfer schedule requires a transaction repository; scheduled.NewService was called with txnRepo=nil")
	}

	var magnitude types.Money
	switch {
	case amount != nil:
		magnitude = amount.Abs()
	case st.HasAmount():
		magnitude = st.Amount.Money.Abs()
	default:
		return nil, &AmountRequiredError{ID: st.ID.String()}
	}

	// CreateTransfer stamps the schedule's memo and optional category onto both
	// legs directly (the returned in-memory pair already reflects them). A
	// post-time preview edit to the category is applied afterward by the
	// command layer via UpdateTransfer, so the template's category is the
	// default for this occurrence.
	memo := ""
	if st.Memo.Valid {
		memo = st.Memo.String
	}

	// Create the transfer pair and advance the schedule in one transaction: the
	// posted legs and the next_date advance commit together (the port receives
	// this tx and binds to it), closing the double-post window.
	//
	// Routing through the transfer owner is what makes a schedule whose target is
	// an investment account POSTABLE. It could be created but never posted
	// before: transaction.CreateTransfer rejected the investment leg with
	// NotRegularAccountError, which in AutoPost is neither a closed-account nor a
	// loan error and therefore aborted the entire batch.
	var regularLegID types.ID
	if err := s.runInTx(func(b *Service) error {
		_, legID, err := b.transferPort.CreateTransfer(
			b.q(), st.AccountID, st.TransferAccountID.ID, date, magnitude, memo, st.CategoryID,
		)
		if err != nil {
			return fmt.Errorf("failed to create scheduled transfer: %w", err)
		}
		regularLegID = legID
		st.AdvanceSchedule()
		return b.repo.Update(st)
	}); err != nil {
		return nil, err
	}

	// The posted row this returns is the regular-ledger leg. An inv↔inv transfer
	// has none, so it reports no transaction — callers tolerate nil (PostResult
	// carries the schedule outcome regardless).
	if regularLegID.IsNil() {
		return nil, nil
	}
	return s.txnRepo.GetByID(regularLegID)
}

// postMultiLine creates a multi-line real transaction from a multi-line
// scheduled template by delegating to transaction.Service.CreateWithSplits
// (which mints fresh TransferIDs and creates paired counterparts for any
// transfer-line splits). The template's children are left in place and the
// schedule advances by one cadence.
func (s *Service) postMultiLine(st *Transaction, date types.Date) (*transaction.Transaction, []*transaction.Split, error) {
	if s.txnSvc == nil {
		return nil, nil, fmt.Errorf("multi-line scheduled posting requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}

	built, err := s.buildMultiLineTransaction(st, date)
	if err != nil {
		// A loan already paid off at post time is a terminal state: refuse the
		// post and mark the schedule completed on the spot, so an ad-hoc payoff
		// transfer cannot strand a never-postable due schedule.
		if errors.Is(err, ErrLoanPaidOff) {
			st.MarkCompleted()
			_ = s.repo.Update(st)
		}
		return nil, nil, err
	}

	// Create the transaction, run payoff completion, and advance the schedule in
	// one transaction: the posted rows and the next_date advance commit together
	// (b.txnSvc is bound, so CreateWithSplits joins this tx). finalizeLoanPayoff
	// reads the loan balance through the bound accountRepo so it sees the
	// just-posted principal counterpart before deciding whether the loan is done.
	if err := s.runInTx(func(b *Service) error {
		if err := b.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
			return fmt.Errorf("failed to create multi-line transaction: %w", err)
		}
		st.AdvanceSchedule()
		if err := b.finalizeLoanPayoff(st); err != nil {
			return fmt.Errorf("failed to finalize loan payoff: %w", err)
		}
		return b.repo.Update(st)
	}); err != nil {
		return nil, nil, err
	}
	return built.parent, built.splits, nil
}

// builtMultiLineTransaction holds the parent transaction and child splits
// assembled from a multi-line template, ready for CreateWithSplits.
type builtMultiLineTransaction struct {
	parent *transaction.Transaction
	splits []*transaction.Split
}

// buildMultiLineTransaction translates a multi-line scheduled template into a
// transaction + splits payload suitable for transaction.Service.CreateWithSplits.
// The parent transaction inherits the template's account, payee, and memo
// (but never a scalar category — multi-line parents must have no category).
// Transfer-line template entries become transfer-line splits with no
// TransferID; the transaction service mints one per call when persisting.
func (s *Service) buildMultiLineTransaction(st *Transaction, date types.Date) (*builtMultiLineTransaction, error) {
	// Loan-shaped schedules recompute their interest/principal split from the
	// loan's live balance at posting time; the stored template is never trusted.
	if s.isLoanShaped(st) {
		return s.buildLoanTransaction(st, date)
	}

	parent := transaction.NewTransaction(st.AccountID, date, st.Amount.Money)
	if st.HasPayee() {
		parent.SetPayee(st.PayeeID.ID)
	}
	if st.Memo.Valid && st.Memo.String != "" {
		parent.SetMemo(st.Memo.String)
	}

	splits := make([]*transaction.Split, 0, len(st.Splits))
	for _, tmpl := range st.Splits {
		var ts *transaction.Split
		if tmpl.TransferAccountID.Valid {
			// Transfer-line: the service mints TransferID. Carry the template's
			// optional category onto the posted split; counterpart mirroring
			// (createTransferLineCounterpart) copies it to the bank-side paired
			// row when the target is a regular account.
			ts = &transaction.Split{
				BaseModel:         types.NewBaseModel(),
				TransactionID:     parent.ID,
				Amount:            tmpl.Amount,
				TransferAccountID: types.NullableID{ID: tmpl.TransferAccountID.ID, Valid: true},
			}
			if tmpl.CategoryID.Valid {
				ts.CategoryID = tmpl.CategoryID.ID
			}
		} else {
			ts = transaction.NewSplit(parent.ID, tmpl.CategoryID.ID, tmpl.Amount)
		}
		if tmpl.Memo.Valid && tmpl.Memo.String != "" {
			ts.SetMemo(tmpl.Memo.String)
		}
		splits = append(splits, ts)
	}

	return &builtMultiLineTransaction{parent: parent, splits: splits}, nil
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
