package tui

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// These tests pin the Transactions menu items that act on the open register.
// Edit Transaction, Delete Transaction and Search… did nothing anywhere: they
// had no case in handleMenuAction.

func menuPick(a *App, action widget.MenuAction) {
	a.handleMenuAction(action, "")
}

func lastNotice(a *App) string {
	n := a.statusbar.Notifications()
	if len(n) == 0 {
		return ""
	}
	return n[len(n)-1].Text
}

func TestTransactionsMenu_RegisterEditDeleteSearch(t *testing.T) {
	a, r := newRegisterSearchApp(t)
	// The rent row is a transfer, so its delete prompt differs from row 0's.
	r.rent.TransferID = types.NullableID{ID: types.NewID(), Valid: true}
	r.rent.TransferAccountID = types.NullableID{ID: types.NewID(), Valid: true}
	a.buildRegisterTable()
	a.sidebar.SetFocused(true)
	a.register.table.SetFocused(false)

	menuPick(a, widget.MenuActionSearch)
	if !a.register.searching {
		t.Fatal("Search… did not start the register search")
	}
	if a.sidebar.IsFocused() {
		t.Error("Search… left focus on the sidebar; the keys after it must act on the found rows")
	}
	regType(a, "rent")
	regKey(a, regEnter)
	if a.register.selectedTransaction() != r.rent {
		t.Fatal("the search did not find the rent row")
	}

	menuPick(a, widget.MenuActionDeleteTransaction)
	if !a.confirm.IsVisible() || a.confirm.dlg.Title() != "Delete Transaction" {
		t.Fatal("Delete Transaction did not ask to delete the selected row")
	}
	if !strings.Contains(a.confirm.dlg.Message(), "transfer") {
		t.Errorf("Delete Transaction asked about another row: %q", a.confirm.dlg.Message())
	}
	a.confirm = confirmSurface{}

	if _, cmd := a.handleMenuAction(widget.MenuActionEditTransaction, ""); cmd == nil {
		t.Error("Edit Transaction returned no command; it must open the edit flow")
	}
}

func TestTransactionsMenu_RegisterClosedAccount(t *testing.T) {
	a, _ := newRegisterSearchApp(t)
	a.register.data.account.Active = false

	menuPick(a, widget.MenuActionDeleteTransaction)
	if a.confirm.IsVisible() {
		t.Error("Delete Transaction offered to delete a row of a closed account")
	}
	if got := lastNotice(a); got != closedAccountChangesNotice {
		t.Errorf("notice = %q, want %q", got, closedAccountChangesNotice)
	}
	if _, cmd := a.handleMenuAction(widget.MenuActionEditTransaction, ""); cmd != nil {
		t.Error("Edit Transaction opened a row of a closed account")
	}
}

func TestTransactionsMenu_InvestmentRegister(t *testing.T) {
	a, _ := newFilterTestApp(t, 0)

	menuPick(a, widget.MenuActionSearch)
	if !a.investmentRegister.filterSearching {
		t.Error("Search… did not start the security filter")
	}
	a.investmentRegister.resetFilter()
	a.investmentRegister.buildTable(a.styles)

	menuPick(a, widget.MenuActionDeleteTransaction)
	if !a.confirm.IsVisible() || a.confirm.dlg.Title() != "Delete Transaction" {
		t.Error("Delete Transaction did not ask to delete the selected row")
	}
	a.confirm = confirmSurface{}

	menuPick(a, widget.MenuActionEditTransaction)
	if a.investmentTypeSelector == nil || !a.investmentTypeSelector.IsVisible() || a.investmentEditTxnID.IsNil() {
		t.Error("Edit Transaction did not open the edit selector for the selected row")
	}
	a.investmentTypeSelector = nil

	menuPick(a, widget.MenuActionNewTransaction)
	if a.investmentTypeSelector == nil || !a.investmentTypeSelector.IsVisible() || !a.investmentEditTxnID.IsNil() {
		t.Error("New Transaction did not open the new-transaction selector")
	}
	a.investmentTypeSelector = nil

	menuPick(a, widget.MenuActionNewTransfer)
	if got := lastNotice(a); got == "" {
		t.Error("New Transfer did nothing and said nothing in an investment register")
	}
}

// Outside a register each item says where it works instead of doing nothing.
func TestTransactionsMenu_OutsideARegister(t *testing.T) {
	for _, action := range []widget.MenuAction{
		widget.MenuActionNewTransaction, widget.MenuActionNewTransfer,
		widget.MenuActionEditTransaction, widget.MenuActionDeleteTransaction,
		widget.MenuActionSearch,
	} {
		a := newModalTestApp()
		if _, cmd := a.handleMenuAction(action, ""); cmd != nil {
			t.Errorf("menu action %v returned a command on the dashboard", action)
		}
		if got := lastNotice(a); got != needRegisterNotice {
			t.Errorf("menu action %v: notice = %q, want %q", action, got, needRegisterNotice)
		}
	}
}
