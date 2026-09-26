package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
)

// newReplaceEnv builds an App on a real database with a brokerage and a
// checking account, and puts row on the brokerage register as the row being
// edited.
func newReplaceEnv(t *testing.T, row func(svc *app.Services, brokerageID types.ID) *investment.Transaction) (*App, *app.Services, *account.Account, *account.Account, *investment.Transaction) {
	t.Helper()
	svc := app.NewServices(dbtest.New(t))
	mk := func(name string, at account.Type) *account.Account {
		a := account.NewAccount(name, at, "USD", types.MustNewMoney("1000.00"), types.NewDate(2019, time.January, 1))
		if err := svc.AccountRepo.Create(a); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return a
	}
	brokerage := mk("Northwind Brokerage", account.TypeInvestment)
	checking := mk("Checking", account.TypeChecking)
	r := row(svc, brokerage.ID)

	a := &App{
		currentView: ViewInvestmentRegister,
		keys:        defaultKeyMap(),
		menubar:     widget.NewMenuBar(),
		statusbar:   widget.NewStatusBar(),
		sidebar:     NewSidebar(),
		services:    *svc,
		undoManager: undo.NewManager(),
		investmentRegister: &investmentRegisterData{
			account:      brokerage,
			transactions: []*investment.Transaction{r},
		},
		investmentEditTxnID: r.ID,
	}
	return a, svc, brokerage, checking, r
}

func withdrawalRow(t *testing.T) func(*app.Services, types.ID) *investment.Transaction {
	return func(svc *app.Services, acctID types.ID) *investment.Transaction {
		wd, err := svc.Investment.Withdrawal(acctID, types.NewDate(2019, time.December, 19), types.MustNewMoney("400.00"), "to checking")
		if err != nil {
			t.Fatal(err)
		}
		return wd
	}
}

func TestEditWithdrawal_ToTransferCash_ReplacesRow(t *testing.T) {
	a, svc, brokerage, checking, wd := newReplaceEnv(t, withdrawalRow(t))

	dispatchType(t, a, investment.TransactionTypeTransferCash)

	d := a.transfer.dlg
	if d == nil || !d.IsVisible() {
		t.Fatal("the transfer dialog did not open")
	}
	if d.Title() != "Change to Transfer" {
		t.Errorf("title = %q, want %q", d.Title(), "Change to Transfer")
	}
	fields := d.Fields()
	if got := fields[0].SelectedOption(); got != brokerage.Name {
		t.Errorf("From = %q, want %q", got, brokerage.Name)
	}
	if got := fields[1].SelectedOption(); got != checking.Name {
		t.Errorf("To = %q, want %q", got, checking.Name)
	}
	if fields[2].Value != "400.00" {
		t.Errorf("Amount = %q, want 400.00", fields[2].Value)
	}
	if fields[3].Value != "12/19/2019" {
		t.Errorf("Date = %q, want 12/19/2019", fields[3].Value)
	}
	if fields[4].Value != "to checking" {
		t.Errorf("Memo = %q, want %q", fields[4].Value, "to checking")
	}

	runCmd(t, a, a.transfer.submit(a.transferDeps(), brokerage.ID), 1)

	if _, err := svc.InvestmentRepo.GetByID(wd.ID); err == nil {
		t.Error("the withdrawal row should be replaced")
	}
	rows, err := svc.InvestmentRepo.ListByAccount(brokerage.ID, investment.TransactionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Type != investment.TransactionTypeTransferCash || !rows[0].TransferID.Valid {
		t.Fatalf("brokerage rows = %+v, want one linked transfer_cash row", rows)
	}
	if !rows[0].TotalAmount.Equal(types.MustNewMoney("-400.00")) {
		t.Errorf("leg amount = %s, want -400.00", rows[0].TotalAmount)
	}
}

func TestEditDeposit_ToTransferCash_PutsAccountOnToSide(t *testing.T) {
	a, _, brokerage, checking, _ := newReplaceEnv(t, func(svc *app.Services, acctID types.ID) *investment.Transaction {
		dep, err := svc.Investment.Deposit(acctID, types.NewDate(2019, time.December, 19), types.MustNewMoney("250.00"), "")
		if err != nil {
			t.Fatal(err)
		}
		return dep
	})

	dispatchType(t, a, investment.TransactionTypeTransferCash)

	fields := a.transfer.dlg.Fields()
	if got := fields[0].SelectedOption(); got != checking.Name {
		t.Errorf("From = %q, want %q", got, checking.Name)
	}
	if got := fields[1].SelectedOption(); got != brokerage.Name {
		t.Errorf("To = %q, want %q", got, brokerage.Name)
	}
}

// Only a Deposit or a Withdrawal can become a transfer. Other rows get a
// notification, not a dialog that fails on save.
func TestEditFee_ToTransferCash_Refused(t *testing.T) {
	a, _, _, _, _ := newReplaceEnv(t, func(svc *app.Services, acctID types.ID) *investment.Transaction {
		fee, err := svc.Investment.Fee(acctID, types.NewDate(2019, time.December, 19), types.MustNewMoney("5.00"), "")
		if err != nil {
			t.Fatal(err)
		}
		return fee
	})

	dispatchType(t, a, investment.TransactionTypeTransferCash)

	if a.transfer.dlg != nil {
		t.Error("no transfer dialog should open for a fee")
	}
	if len(a.statusbar.Notifications()) == 0 {
		t.Error("expected a notification")
	}
}
