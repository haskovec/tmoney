package integration

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// These tests cover the SECONDARY surfaces that act on transfer legs — the ones
// the phase-1..6 suites missed, because each package's tests exercise the new
// owner rather than the old call site that was left pointing at the refusing
// service.
//
// Every case here is a path a user can reach from the UI or the CLI.

// TestCutover_InvestmentRegisterDelete_CashTransfer covers deleting a cash
// transfer from the INVESTMENT register.
//
// Phase 5b made investment.Service.DeleteTransaction refuse a transfer_cash leg
// (correctly — it would orphan a counterpart in the other ledger). The
// investment register's delete key was never re-pointed at the transfer owner,
// so the refusal reached the user as an error.
func TestCutover_InvestmentRegisterDelete_CashTransfer(t *testing.T) {
	for _, sh := range []struct {
		name   string
		toType account.Type
	}{
		{"inv-to-bank", account.TypeChecking},
		{"inv-to-inv", account.TypeHSA},
	} {
		t.Run(sh.name, func(t *testing.T) {
			svc, _ := openTransferServices(t)
			brokerage := makeTransferAccount(t, svc, "Brokerage", account.TypeInvestment)
			other := makeTransferAccount(t, svc, "Other "+sh.name, sh.toType)

			res, err := svc.Transfer.Create(transfer.Spec{
				FromAccountID: brokerage.ID,
				ToAccountID:   other.ID,
				Date:          types.NewDate(2024, 6, 1),
				Amount:        types.MustNewMoney("300.00"),
			})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			// The investment-ledger leg is what the investment register has
			// selected when the user presses the delete key.
			invLeg := res.From
			if invLeg.Ledger != transfer.LedgerInvestment {
				t.Fatalf("expected the From leg on the investment ledger, got %v", invLeg.Ledger)
			}

			// Deleting the whole transfer by transfer_id must succeed and remove
			// BOTH legs, wherever they live.
			if _, err := svc.Transfer.Delete(res.TransferID); err != nil {
				t.Fatalf("Transfer.Delete from the investment side: %v", err)
			}
			if _, err := svc.Transfer.Get(res.TransferID); err == nil {
				t.Error("transfer still readable after Delete")
			}
			rows, err := svc.InvestmentRepo.ListByAccount(brokerage.ID, investment.TransactionFilter{})
			if err != nil {
				t.Fatalf("list investment rows: %v", err)
			}
			for _, r := range rows {
				if r.TransferID.Valid {
					t.Errorf("investment leg %s survived the delete", r.ID)
				}
			}
		})
	}
}

// TestCutover_InvestmentServiceStillRefusesCashTransferLeg pins the refusal
// itself, so the fix above cannot be "make DeleteTransaction permissive again".
// The refusal is correct; the CALLER had to change.
func TestCutover_InvestmentServiceStillRefusesCashTransferLeg(t *testing.T) {
	svc, _ := openTransferServices(t)
	brokerage := makeTransferAccount(t, svc, "Brokerage", account.TypeInvestment)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: brokerage.ID,
		ToAccountID:   checking.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("120.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = svc.Investment.DeleteTransaction(res.From.RowID)
	var target *investment.IsCashTransferLegError
	if !errors.As(err, &target) {
		t.Fatalf("DeleteTransaction(cash transfer leg) = %T (%v), want *investment.IsCashTransferLegError", err, err)
	}
}

// TestCutover_CLIVoid_TransferLeg covers `tmoney transaction void <transfer leg>`.
//
// Phase 5a made transaction.Service.VoidTransaction refuse a whole-transfer leg.
// The TUI was re-pointed at transfer.Void; the CLI's void command was not, so it
// surfaces the raw refusal.
func TestCutover_CLIVoid_TransferLeg(t *testing.T) {
	svc, _ := openTransferServices(t)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)
	savings := makeTransferAccount(t, svc, "Savings", account.TypeSavings)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   savings.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("80.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The old CLI path.
	var refused *transaction.IsTransferError
	if err := svc.Transaction.VoidTransaction(res.From.RowID); !errors.As(err, &refused) {
		t.Fatalf("precondition: VoidTransaction should refuse a transfer leg, got %T (%v)", err, err)
	}

	// The path the CLI must take instead: resolve the leg, void the transfer.
	resolved, err := svc.Transfer.Resolve(res.From.RowID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := svc.Transfer.Void(resolved.TransferID); err != nil {
		t.Fatalf("Transfer.Void: %v", err)
	}

	voided, err := svc.Transfer.Get(res.TransferID)
	if err != nil {
		t.Fatalf("Get after Void: %v", err)
	}
	for _, leg := range voided.Legs() {
		if leg.Status != transaction.StatusVoid {
			t.Errorf("leg %s status = %q, want void", leg.RowID, leg.Status)
		}
		if !leg.Amount.IsZero() {
			t.Errorf("leg %s amount = %s, want 0", leg.RowID, leg.Amount)
		}
	}
}

// TestCutover_CLIVoid_InvestmentInvolvingIsNamed pins that an inv-involving
// transfer gets the SPECIFIC error, not a generic one. The design's whole point
// for this case was replacing an unhelpful message with a named refusal.
func TestCutover_CLIVoid_InvestmentInvolvingIsNamed(t *testing.T) {
	svc, _ := openTransferServices(t)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)
	brokerage := makeTransferAccount(t, svc, "Brokerage", account.TypeInvestment)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: checking.ID,
		ToAccountID:   brokerage.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("60.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resolved, err := svc.Transfer.Resolve(res.From.RowID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	_, err = svc.Transfer.Void(resolved.TransferID)
	var target *transfer.VoidNotSupportedError
	if !errors.As(err, &target) {
		t.Fatalf("Void(inv-involving) = %T (%v), want *transfer.VoidNotSupportedError", err, err)
	}
}

// TestCutover_AutoPostUndo_TransferSchedule covers Ctrl+Z after a transfer
// schedule auto-posts.
//
// AutoPost creates the pair through the transfer owner, but AutoPostCommand.Undo
// deleted the posted rows one at a time through transaction.Service.Delete —
// which phase 5a made refuse a transfer leg.
func TestCutover_AutoPostUndo_TransferSchedule(t *testing.T) {
	svc, _ := openTransferServices(t)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)
	savings := makeTransferAccount(t, svc, "Savings", account.TypeSavings)

	st := scheduled.NewTransactionWithAmount(checking.ID, scheduled.FrequencyMonthly,
		types.Today(), types.MustNewMoney("-150.00"))
	st.SetTransfer(savings.ID)
	st.AutoPost = true
	st.NextDate = types.Today()
	if err := svc.Scheduled.Create(st); err != nil {
		t.Fatalf("Create schedule: %v", err)
	}

	summary, err := svc.Scheduled.AutoPost()
	if err != nil {
		t.Fatalf("AutoPost: %v", err)
	}
	if summary.PostedCount != 1 {
		t.Fatalf("PostedCount = %d, want 1", summary.PostedCount)
	}

	// Every recorded transaction must be usable for undo — no nil entries.
	for _, r := range summary.Results {
		for i, txn := range r.Transactions {
			if txn == nil {
				t.Fatalf("Results[..].Transactions[%d] is nil; undo cannot address it", i)
			}
		}
	}

	cmd := undo.NewAutoPostCommand(svc.Transaction, svc.Transfer, svc.Scheduled, summary)
	if err := cmd.Undo(); err != nil {
		t.Fatalf("AutoPostCommand.Undo after a transfer post: %v", err)
	}

	// Both legs are gone.
	for _, acctID := range []types.ID{checking.ID, savings.ID} {
		rows, err := svc.TransactionRepo.ListByAccount(acctID)
		if err != nil {
			t.Fatalf("list rows: %v", err)
		}
		for _, r := range rows {
			if r.TransferID.Valid {
				t.Errorf("transfer leg %s in account %s survived the undo", r.ID, acctID)
			}
		}
	}
}

// TestCutover_InvestmentRegisterClear_UsesTheOwner covers Space-to-clear on a
// transfer leg in the INVESTMENT register.
//
// The regular register was re-pointed at transfer.SetLegStatus in phase 3; the
// investment register still called investment.Service.SetClearedStatus. That did
// not hard-fail, which is why nothing caught it — but it is a second write path
// onto a transfer leg, it is not undoable, and it rewrites the whole row instead
// of the one status column.
func TestCutover_InvestmentRegisterClear_UsesTheOwner(t *testing.T) {
	svc, _ := openTransferServices(t)
	brokerage := makeTransferAccount(t, svc, "Brokerage", account.TypeInvestment)
	checking := makeTransferAccount(t, svc, "Checking", account.TypeChecking)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: brokerage.ID,
		ToAccountID:   checking.ID,
		Date:          types.NewDate(2024, 6, 1),
		Amount:        types.MustNewMoney("45.00"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Clearing the investment leg through the owner touches ONLY that leg, and is
	// undoable.
	mgr := undo.NewManager()
	cmd := undo.NewSetTransferLegStatusCommand(svc.Transfer, res.From.RowID, transaction.StatusCleared)
	if err := mgr.Execute(cmd); err != nil {
		t.Fatalf("SetTransferLegStatusCommand: %v", err)
	}

	got, err := svc.Transfer.Get(res.TransferID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, leg := range got.Legs() {
		want := transaction.StatusUncleared
		if leg.RowID == res.From.RowID {
			want = transaction.StatusCleared
		}
		if leg.Status != want {
			t.Errorf("leg %s status = %q, want %q", leg.RowID, leg.Status, want)
		}
	}

	// And it undoes.
	if _, err := mgr.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	reverted, err := svc.Transfer.Get(res.TransferID)
	if err != nil {
		t.Fatalf("Get after undo: %v", err)
	}
	for _, leg := range reverted.Legs() {
		if leg.Status != transaction.StatusUncleared {
			t.Errorf("leg %s status = %q after undo, want uncleared", leg.RowID, leg.Status)
		}
	}
}
