package scheduled

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Writing the ledger rows for ONE occurrence of a schedule.
//
// Everything in this file is a PARTICIPANT in the sense of
// specs/design-service-decomposition.md section 2.3: it performs row writes on a
// transaction-bound receiver, opens no transaction of its own, persists nothing
// about the schedule, mutates nothing on the schedule, and classifies no skip.
// Its callers — the manual entry points in posting.go and the catch-up loop in
// auto_post.go — own the transaction, the advance, the persist, and the skip.
//
// That division is what lets one engine serve both. A participant cannot
// deadlock, because it never opens a transaction; and it can be called once or
// in a loop, because it does not persist the schedule.

// postedOccurrence is what writing one occurrence produced.
type postedOccurrence struct {
	// txn is the posted regular-ledger row. It is nil for an investment-to-
	// investment transfer, which has no regular-ledger leg at all — callers
	// must not append it to a result list unchecked.
	txn *transaction.Transaction
	// splits are the posted child rows. Nil unless the schedule is multi-line.
	splits []*transaction.Split
	// transferID addresses the posted PAIR, and is zero unless a transfer was
	// posted. Undo needs the pair rather than a leg: deleting a single leg is
	// refused by transaction.Service and would orphan its counterpart anyway.
	transferID types.ID
}

// postOccurrence writes the ledger rows for one occurrence of st, dated date.
//
// It must be called on a tx-bound receiver, so that every write inside joins the
// caller's transaction. Calling it on an unbound service is the mistake section
// 2.2 describes: the writes would land outside the caller's transaction.
//
// amount overrides the template's stored amount for this occurrence; nil means
// "use the template's own". Resolving a variable-amount schedule's estimate is
// deliberately the CALLER's job, because the two callers legitimately disagree
// about it — a manual post estimates from the stored row, while auto-post
// estimates against the in-memory schedule that earlier occurrences of the same
// catch-up have already advanced, and sees those occurrences' rows. Taking the
// amount as a parameter is what makes that disagreement stop existing here.
func (b *Service) postOccurrence(st *Transaction, date types.Date, amount *types.Money) (postedOccurrence, error) {
	// The three shapes are mutually exclusive: Transaction.Validate refuses a
	// transfer schedule that also carries split lines.
	switch {
	case st.IsTransfer():
		return b.postOccurrenceTransfer(st, date, amount)
	case len(st.Splits) > 0:
		return b.postOccurrenceMultiLine(st, date)
	default:
		return b.postOccurrenceSingle(st, date, amount)
	}
}

// postOccurrenceSingle writes a plain single-line transaction from the template.
func (b *Service) postOccurrenceSingle(st *Transaction, date types.Date, amount *types.Money) (postedOccurrence, error) {
	txnAmount := st.Amount.Money
	if amount != nil {
		txnAmount = *amount
	}

	txn := transaction.NewTransaction(st.AccountID, date, txnAmount)
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
		return postedOccurrence{}, fmt.Errorf("failed to create transaction: %w", err)
	}
	return postedOccurrence{txn: txn}, nil
}

// postOccurrenceTransfer writes a clean linked transfer pair from a single-line
// transfer template (account_id = From, transfer_account_id = To).
//
// The schedule stores the amount as the signed effect on the source account
// (negative) and CreateTransfer wants a positive magnitude, so the amount is
// taken as an absolute value. CreateTransfer stamps the schedule's memo and
// optional category onto whichever legs can hold them.
//
// Routing through the transfer owner is what makes a schedule whose target is an
// investment account postable at all. It could be created but never posted
// before: transaction.CreateTransfer rejected the investment leg with
// NotRegularAccountError, which auto-post could classify as neither a
// closed-account nor a loan error and so aborted the entire batch.
func (b *Service) postOccurrenceTransfer(st *Transaction, date types.Date, amount *types.Money) (postedOccurrence, error) {
	if b.transferPort == nil {
		return postedOccurrence{}, fmt.Errorf("posting a transfer schedule requires the transfer service; scheduled.SetTransferPort was never called")
	}

	magnitude := st.Amount.Money.Abs()
	if amount != nil {
		magnitude = amount.Abs()
	}
	memo := ""
	if st.Memo.Valid {
		memo = st.Memo.String
	}

	transferID, regularLegID, err := b.transferPort.CreateTransfer(
		b.q(), st.AccountID, st.TransferAccountID.ID, date, magnitude, memo, st.CategoryID,
	)
	if err != nil {
		return postedOccurrence{}, fmt.Errorf("failed to create scheduled transfer: %w", err)
	}

	out := postedOccurrence{transferID: transferID}
	// An investment-to-investment occurrence has no regular-ledger leg to report;
	// its transfer_id above is how undo reaches it.
	if !regularLegID.IsNil() && b.txnRepo != nil {
		posted, gerr := b.txnRepo.GetByID(regularLegID)
		if gerr != nil {
			return postedOccurrence{}, fmt.Errorf("failed to read the posted transfer leg: %w", gerr)
		}
		out.txn = posted
	}
	return out, nil
}

// postOccurrenceMultiLine writes a multi-line transaction by delegating to
// transaction.Service.CreateWithSplits, which mints fresh TransferIDs and creates
// paired counterparts for any transfer-line splits. The template's own children
// are left in place.
//
// A build failure is returned unwrapped so callers can classify it with
// errors.Is: a loan-shaped schedule recomputes its interest and principal from
// the loan's live balance here, and the typed errors that can produce are the
// difference between a skip and a hard failure.
func (b *Service) postOccurrenceMultiLine(st *Transaction, date types.Date) (postedOccurrence, error) {
	if b.txnSvc == nil {
		return postedOccurrence{}, fmt.Errorf("multi-line scheduled posting requires a transaction service; scheduled.NewService was called with txnSvc=nil")
	}

	built, err := b.buildMultiLineTransaction(st, date)
	if err != nil {
		return postedOccurrence{}, err
	}
	if err := b.txnSvc.CreateWithSplits(built.parent, built.splits); err != nil {
		return postedOccurrence{}, fmt.Errorf("failed to create multi-line transaction: %w", err)
	}
	return postedOccurrence{txn: built.parent, splits: built.splits}, nil
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
