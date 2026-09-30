package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// The merger preview lists the accounts the merger will change. A lot-tracked
// account that sold out keeps a position row with shares, and the merger skips
// that account, so the preview must not list it.
func TestLoadMergerConfirmData_SkipsSoldOutLotAccount(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := NewApp(dbtest.New(t), nil)
	svc := a.services
	date := types.NewDate(2026, time.January, 5)

	source := security.NewSecurity("OLD", "Fabrikam Old", security.TypeStock)
	target := security.NewSecurity("NEW", "Fabrikam New", security.TypeStock)
	for _, sec := range []*security.Security{source, target} {
		if err := svc.Security.Create(sec); err != nil {
			t.Fatal(err)
		}
	}
	newAccount := func(name string, trackLots bool) *account.Account {
		t.Helper()
		acct := account.NewAccount(name, account.TypeInvestment, "USD", types.ZeroMoney, date)
		acct.TrackLots = trackLots
		if err := svc.Account.Create(acct); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Investment.Deposit(acct.ID, date, types.MustNewMoney("1000.00"), ""); err != nil {
			t.Fatal(err)
		}
		total := types.MustNewMoney("400.00")
		if _, err := svc.Investment.Buy(acct.ID, source.ID, date, types.MustNewQuantity("4"), &total, nil, types.ZeroMoney, ""); err != nil {
			t.Fatal(err)
		}
		return acct
	}
	holder := newAccount("Contoso Brokerage", false)
	soldOut := newAccount("Northwind Sold Out", true)
	lots, err := investment.NewLotRepository(a.db).ListByAccountAndSecurity(soldOut.ID, source.ID, false)
	if err != nil || len(lots) != 1 {
		t.Fatalf("lots = %v, %v; want one open lot", lots, err)
	}
	sellTotal := types.MustNewMoney("480.00")
	if _, err := svc.Investment.Sell(soldOut.ID, source.ID, date, types.MustNewQuantity("4"), &sellTotal, nil, types.ZeroMoney, "",
		[]investment.SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("4")}}); err != nil {
		t.Fatal(err)
	}

	a.mergerConfirm.params = &mergerConfirmParams{
		sourceSecurityID: source.ID,
		targetSecurityID: target.ID,
		mergerDate:       date,
	}
	msg, ok := a.loadMergerConfirmData()().(mergerConfirmDataMsg)
	if !ok {
		t.Fatal("loadMergerConfirmData did not return its data message")
	}
	if len(msg.data.accounts) != 1 || msg.data.accounts[0].accountID != holder.ID {
		t.Errorf("preview accounts = %+v, want only %s", msg.data.accounts, holder.Name)
	}
}
