package scheduled

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// TestService_PostSingleLineTransfer_ToInvestmentAccount is the phase-4 exit
// criterion of specs/design-unified-transfer.md, and a real bug fix.
//
// Before: such a schedule could be CREATED (scheduled_repository only checks that
// the account exists) but could never POST. Posting went through
// transaction.Service.CreateTransfer, which rejected the investment leg with
// NotRegularAccountError — and in AutoPost that error is neither a
// ClosedAccountError nor a loan error, so it aborted the ENTIRE batch. One
// paycheck→401k schedule stopped every other schedule that day from posting.
//
// After: posting routes through the transfer owner, which writes the investment
// leg to investment_transactions as a transfer_cash row.
func TestService_PostSingleLineTransfer_ToInvestmentAccount(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	ira := env.account(t, "Rollover IRA", account.TypeInvestment)

	st := newTransferSchedule(checking.ID, ira.ID, "500.00")
	if err := env.svc.Create(st); err != nil {
		t.Fatalf("Create: %v", err)
	}
	originalNext := st.NextDate

	from, err := env.svc.Post(st.ID, nil)
	if err != nil {
		t.Fatalf("Post to an investment account: %v (this is the bug phase 4 fixes)", err)
	}

	// The regular-ledger leg is reported and is the outflow side.
	if from == nil {
		t.Fatal("Post returned no transaction; the bank leg should be reported")
	}
	if from.AccountID != checking.ID {
		t.Errorf("from.AccountID = %v, want Checking", from.AccountID)
	}
	if !from.Amount.Equal(types.MustNewMoney("-500.00")) {
		t.Errorf("from.Amount = %s, want -500.00", from.Amount)
	}
	if !from.TransferID.Valid {
		t.Fatal("bank leg is not transfer-linked")
	}

	// The investment leg exists, is typed transfer_cash, and is positive.
	invRows, err := investment.NewRepository(env.svc.db).ListByAccount(ira.ID, investment.TransactionFilter{})
	if err != nil {
		t.Fatalf("list investment rows: %v", err)
	}
	if len(invRows) != 1 {
		t.Fatalf("expected 1 investment row in the IRA, got %d", len(invRows))
	}
	inv := invRows[0]
	if inv.Type != investment.TransactionTypeTransferCash {
		t.Errorf("investment leg type = %q, want %q", inv.Type, investment.TransactionTypeTransferCash)
	}
	if !inv.Type.AffectsCash() {
		t.Error("investment leg does not affect cash; it would vanish from the IRA's cash balance")
	}
	if !inv.TotalAmount.Equal(types.MustNewMoney("500.00")) {
		t.Errorf("investment leg amount = %s, want 500.00", inv.TotalAmount)
	}
	if !inv.TransferID.Valid || inv.TransferID.ID != from.TransferID.ID {
		t.Errorf("investment leg transfer_id = %+v, want %s", inv.TransferID, from.TransferID.ID)
	}
	if !inv.TransferAccountID.Valid || inv.TransferAccountID.ID != checking.ID {
		t.Errorf("investment leg transfer_account_id = %+v, want Checking", inv.TransferAccountID)
	}

	// And the schedule advanced, in the same transaction as the two legs.
	reloaded, err := env.svc.GetByID(st.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if reloaded.NextDate == originalNext {
		t.Error("schedule did not advance after posting")
	}
}

// TestService_AutoPostTransfer_ToInvestmentAccountDoesNotAbortBatch pins the
// blast-radius half of the same bug: the investment-leg rejection used to abort
// AutoPost entirely, so one such schedule silently stopped every other due
// schedule from posting.
func TestService_AutoPostTransfer_ToInvestmentAccountDoesNotAbortBatch(t *testing.T) {
	env := newTransferTestEnv(t)
	checking := env.account(t, "Checking", account.TypeChecking)
	savings := env.account(t, "Savings", account.TypeSavings)
	ira := env.account(t, "Rollover IRA", account.TypeInvestment)

	// Due today: one transfer into the IRA, one ordinary bank↔bank transfer.
	toIRA := newTransferSchedule(checking.ID, ira.ID, "300.00")
	toIRA.NextDate = types.Today()
	toIRA.AutoPost = true
	if err := env.svc.Create(toIRA); err != nil {
		t.Fatalf("Create IRA schedule: %v", err)
	}

	toSavings := newTransferSchedule(checking.ID, savings.ID, "125.00")
	toSavings.NextDate = types.Today()
	toSavings.AutoPost = true
	if err := env.svc.Create(toSavings); err != nil {
		t.Fatalf("Create savings schedule: %v", err)
	}

	summary, err := env.svc.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost: %v (the IRA schedule used to abort the whole batch)", err)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("expected 2 auto-post results, got %d", len(summary.Results))
	}
	if summary.SkippedCount != 0 {
		t.Errorf("SkippedCount = %d, want 0", summary.SkippedCount)
	}
	if summary.PostedCount != 2 {
		t.Errorf("PostedCount = %d, want 2", summary.PostedCount)
	}

	// Both transfers landed: two bank legs in Checking, one investment leg.
	checkingRows, err := env.txnRepo.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list checking rows: %v", err)
	}
	linked := 0
	for _, r := range checkingRows {
		if r.TransferID.Valid {
			linked++
		}
	}
	if linked != 2 {
		t.Errorf("expected 2 transfer-linked bank legs in Checking, got %d", linked)
	}

	invRows, err := investment.NewRepository(env.svc.db).ListByAccount(ira.ID, investment.TransactionFilter{})
	if err != nil {
		t.Fatalf("list investment rows: %v", err)
	}
	if len(invRows) != 1 {
		t.Errorf("expected 1 investment row in the IRA, got %d", len(invRows))
	}
}
