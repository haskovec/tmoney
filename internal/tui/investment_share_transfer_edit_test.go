package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// shareTransferRegister puts one leg of a share transfer on the investment
// register of acct, cursor on it and the table focused, as a user is when they
// press Enter to edit the row.
func shareTransferRegister(t *testing.T, a *App, acct *account.Account, leg *investment.Transaction) {
	t.Helper()
	a.currentView = ViewInvestmentRegister
	a.investmentRegister.data = &investmentRegisterData{
		account:      acct,
		transactions: []*investment.Transaction{leg},
	}
	a.buildInvestmentRegisterTable()
	a.sidebar.SetFocused(false)
	a.investmentRegister.table.SetFocused(true)
}

// The edit dialog sends from the register's account, so Enter on the
// receiving leg must not open it: the save would reverse the transfer.
func TestInvestmentRegister_Enter_ShareTransferLegs(t *testing.T) {
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40
	date := types.NewDate(2024, time.March, 1)

	src := account.NewAccount("Northwind Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, date)
	dst := account.NewAccount("Contoso IRA", account.TypeInvestment, "USD", types.ZeroMoney, date)
	for _, acct := range []*account.Account{src, dst} {
		if err := a.services.Account.Create(acct); err != nil {
			t.Fatal(err)
		}
	}
	sec := security.NewSecurity("FABR", "Fabrikam Inc.", security.TypeStock)
	if err := a.services.Security.Create(sec); err != nil {
		t.Fatal(err)
	}
	price := types.MustNewMoney("50.00")
	if _, err := a.services.Investment.Buy(src.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &price, types.ZeroMoney, ""); err != nil {
		t.Fatal(err)
	}
	res, err := a.services.Investment.TransferShares(src.ID, dst.ID, sec.ID, date, types.MustNewQuantity("5"), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("destination leg is refused and names the source", func(t *testing.T) {
		shareTransferRegister(t, a, dst, res.DestinationTransaction)
		a.handleInvestmentRegisterKeys(tea.KeyPressMsg{Code: tea.KeyEnter})

		if a.investmentTypeSelector != nil {
			t.Error("the edit selector opened for the receiving leg")
		}
		notes := a.statusbar.Notifications()
		if len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Text, "Edit this share transfer from Northwind Brokerage") {
			t.Errorf("notifications = %v, want one that names the source account", notes)
		}
	})

	t.Run("source leg opens the editor", func(t *testing.T) {
		shareTransferRegister(t, a, src, res.SourceTransaction)
		a.handleInvestmentRegisterKeys(tea.KeyPressMsg{Code: tea.KeyEnter})

		if a.investmentTypeSelector == nil {
			t.Error("the edit selector did not open for the sending leg")
		}
	})
}
