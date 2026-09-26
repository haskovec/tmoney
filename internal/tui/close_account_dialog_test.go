package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// Reopening an account from the menu while its investment register is on
// screen must unfreeze that register, so Enter edits the row again.
func TestReopenAccount_UnfreezesInvestmentRegister(t *testing.T) {
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40

	acct := account.NewAccount("Northwind Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	if err := a.services.Account.Close(acct.ID, types.Today()); err != nil {
		t.Fatal(err)
	}

	runCmd(t, a, a.loadSidebarData(), 1)
	if !a.sidebar.SetCursorToAccount(acct.ID) || !a.sidebar.Select() {
		t.Fatal("account not in the sidebar")
	}
	a.switchView(ViewInvestmentRegister)
	runCmd(t, a, a.loadInvestmentRegisterData(acct.ID), 1)
	if !a.investmentRegister.account.IsClosed() {
		t.Fatal("setup: register should show the account as closed")
	}

	_, cmd := a.handleMenuAction(widget.MenuActionReopenAccount, "")
	runCmd(t, a, cmd, 3)

	if a.investmentRegister.account.IsClosed() {
		t.Error("register still holds the account as closed after reopen")
	}
}
