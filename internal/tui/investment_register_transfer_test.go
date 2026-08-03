package tui

import (
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// These tests cover the investment register's delete and clear keys acting on a
// cash-transfer leg.
//
// Both were left pointing at investment.Service after phase 5b made
// DeleteTransaction refuse a transfer_cash leg. Nothing caught it because no test
// drove those keys against a transfer — the register tests are unit-level on
// formatting, and the transfer suite exercises the owner rather than the caller.

// TestIsCashTransferLeg pins the routing decision both keys turn on.
func TestIsCashTransferLeg(t *testing.T) {
	acct := types.NewID()
	other := types.NewID()
	date := types.NewDate(2024, time.June, 1)
	link := types.NullableID{ID: types.NewID(), Valid: true}

	t.Run("nil", func(t *testing.T) {
		if isCashTransferLeg(nil) {
			t.Error("nil should not be a cash-transfer leg")
		}
	})

	t.Run("linked transfer_cash is a leg", func(t *testing.T) {
		txn := investment.NewTransaction(acct, date, investment.TransactionTypeTransferCash, types.MustNewMoney("-100.00"))
		txn.SetTransfer(link.ID, other)
		if !isCashTransferLeg(txn) {
			t.Error("a linked transfer_cash row is a cash-transfer leg")
		}
	})

	// Share transfers are owned by internal/investment: both legs are investment
	// rows and their delete cascade reverses lot/position effects, which the
	// transfer owner does not do. Routing them away would lose that.
	t.Run("transfer_shares is NOT a leg", func(t *testing.T) {
		txn := investment.NewTransaction(acct, date, investment.TransactionTypeTransferShares, types.MustNewMoney("-100.00"))
		txn.SetTransfer(link.ID, other)
		if isCashTransferLeg(txn) {
			t.Error("a share transfer must stay with investment.Service")
		}
	})

	// An unlinked transfer_cash row is the counterpart of a split transfer LINE,
	// owned by the split lifecycle rather than by the transfer service.
	t.Run("unlinked transfer_cash is NOT a leg", func(t *testing.T) {
		txn := investment.NewTransaction(acct, date, investment.TransactionTypeTransferCash, types.MustNewMoney("-100.00"))
		if isCashTransferLeg(txn) {
			t.Error("an unlinked row cannot be a transfer leg")
		}
	})

	t.Run("every other type is NOT a leg", func(t *testing.T) {
		for _, ty := range investment.AllTransactionTypes() {
			if ty == investment.TransactionTypeTransferCash {
				continue
			}
			txn := investment.NewTransaction(acct, date, ty, types.MustNewMoney("-100.00"))
			txn.SetTransfer(link.ID, other)
			if isCashTransferLeg(txn) {
				t.Errorf("type %q must not route to the transfer owner", ty)
			}
		}
	})
}

// invRegTransferEnv is a real App with a real database and a loaded investment
// register holding one cash transfer.
type invRegTransferEnv struct {
	app        *App
	svc        *app.Services
	brokerage  *account.Account
	other      *account.Account
	transferID types.ID
	invLegID   types.ID
	regLegID   types.ID
}

func newInvRegTransferEnv(t *testing.T, otherType account.Type) *invRegTransferEnv {
	t.Helper()
	database := dbtest.New(t)
	svc := app.NewServices(database)

	mk := func(name string, at account.Type) *account.Account {
		a := account.NewAccount(name, at, "USD", types.MustNewMoney("1000.00"), types.NewDate(2020, time.January, 1))
		if err := svc.AccountRepo.Create(a); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return a
	}
	brokerage := mk("Brokerage", account.TypeInvestment)
	other := mk("Other", otherType)

	res, err := svc.Transfer.Create(transfer.Spec{
		FromAccountID: brokerage.ID,
		ToAccountID:   other.ID,
		Date:          types.NewDate(2024, time.June, 1),
		Amount:        types.MustNewMoney("250.00"),
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	invLeg, err := svc.InvestmentRepo.GetByID(res.From.RowID)
	if err != nil {
		t.Fatalf("load investment leg: %v", err)
	}

	a := &App{
		currentView:   ViewInvestmentRegister,
		keys:          defaultKeyMap(),
		menubar:       widget.NewMenuBar(),
		statusbar:     widget.NewStatusBar(),
		sidebar:       NewSidebar(),
		investmentSvc: svc.Investment,
		transferSvc:   svc.Transfer,
		undoManager:   undo.NewManager(),
		investmentRegister: &investmentRegisterData{
			account:      brokerage,
			transactions: []*investment.Transaction{invLeg},
		},
	}
	a.buildInvestmentRegisterTable()
	// A fresh sidebar starts focused, and handleInvestmentRegisterKeys delegates
	// every key to it while it is. Move focus to the register table, which is the
	// state a user is in when they press "d" on a row.
	a.sidebar.SetFocused(false)
	if a.investmentTable != nil {
		a.investmentTable.SetFocused(true)
	}

	return &invRegTransferEnv{
		app: a, svc: svc, brokerage: brokerage, other: other,
		transferID: res.TransferID, invLegID: res.From.RowID, regLegID: res.To.RowID,
	}
}

// TestInvestmentRegister_DeleteKey_CashTransfer drives the real delete key.
func TestInvestmentRegister_DeleteKey_CashTransfer(t *testing.T) {
	for name, otherType := range map[string]account.Type{
		"inv-to-bank": account.TypeChecking,
		"inv-to-inv":  account.TypeHSA,
	} {
		t.Run(name, func(t *testing.T) {
			env := newInvRegTransferEnv(t, otherType)

			// Press the delete key ("d" — see app_keymap.go); it opens a
			// confirmation.
			model, _ := env.app.handleInvestmentRegisterKeys(tea.KeyPressMsg{Code: 'd', Text: "d"})
			a := model.(*App)
			if a.confirmAction == nil {
				t.Fatal("delete key did not open a confirmation dialog")
			}

			// Confirm.
			msg := a.confirmAction()
			if em, ok := msg.(errMsg); ok {
				t.Fatalf("confirming the delete failed: %v", em.err)
			}

			// Both legs are gone, wherever they lived.
			if _, err := env.svc.Transfer.Get(env.transferID); err == nil {
				t.Error("transfer still readable after delete")
			}
			if _, err := env.svc.InvestmentRepo.GetByID(env.invLegID); err == nil {
				t.Error("investment leg survived the delete")
			}

			// And it is undoable, which it never was through DeleteTransaction.
			if !a.undoManager.CanUndo() {
				t.Fatal("deleting a transfer from the investment register should be undoable")
			}
			if _, err := a.undoManager.Undo(); err != nil {
				t.Fatalf("undo: %v", err)
			}
			if _, err := env.svc.Transfer.Get(env.transferID); err != nil {
				t.Errorf("transfer should be back after undo: %v", err)
			}
		})
	}
}

// TestInvestmentRegister_ClearKey_CashTransfer drives the real clear toggle and
// asserts it touches ONE leg and is undoable.
func TestInvestmentRegister_ClearKey_CashTransfer(t *testing.T) {
	env := newInvRegTransferEnv(t, account.TypeChecking)

	_, cmd := env.app.toggleInvestmentTransactionStatus()
	if cmd == nil {
		t.Fatal("clear toggle produced no command")
	}
	if msg := cmd(); msg != nil {
		if em, ok := msg.(errMsg); ok {
			t.Fatalf("clear toggle failed: %v", em.err)
		}
	}

	got, err := env.svc.Transfer.Get(env.transferID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, leg := range got.Legs() {
		want := transaction.StatusUncleared
		if leg.RowID == env.invLegID {
			want = transaction.StatusCleared
		}
		if leg.Status != want {
			t.Errorf("leg %s status = %q, want %q (only the selected leg moves)", leg.RowID, leg.Status, want)
		}
	}

	if !env.app.undoManager.CanUndo() {
		t.Fatal("clearing a transfer leg should be undoable")
	}
	if _, err := env.app.undoManager.Undo(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	reverted, err := env.svc.Transfer.Get(env.transferID)
	if err != nil {
		t.Fatalf("Get after undo: %v", err)
	}
	for _, leg := range reverted.Legs() {
		if leg.Status != transaction.StatusUncleared {
			t.Errorf("leg %s = %q after undo, want uncleared", leg.RowID, leg.Status)
		}
	}
}
