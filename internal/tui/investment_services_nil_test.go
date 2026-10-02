package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/types"
)

// The investment service and its read model, the valuation service, are two
// fields on app.Services. Production always wires both, but each load must
// guard the service it calls, not its sibling: a load that checks one and
// calls the other panics when only the checked one is wired, and skips its
// work when only the called one is.

// newOneInvestmentAccountApp returns an App with every service wired and one
// open investment account. The account tracks lots, so the lot-detail load
// runs to the end.
func newOneInvestmentAccountApp(t *testing.T) (*App, *account.Account) {
	t.Helper()
	a := NewApp(dbtest.New(t), nil)
	a.width, a.height = 120, 40
	acct := account.NewAccount("Northwind Brokerage", account.TypeInvestment, "USD", types.ZeroMoney, types.MustParseDate("2020-01-01"))
	acct.TrackLots = true
	if err := a.services.Account.Create(acct); err != nil {
		t.Fatal(err)
	}
	return a, acct
}

// runLoad runs a load command and returns its message. A panic fails the test
// rather than the run.
func runLoad(t *testing.T, cmd tea.Cmd) (msg tea.Msg) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("the load panicked: %v", r)
		}
	}()
	return cmd()
}

func TestInvestmentLoads_WithoutTheValuationService(t *testing.T) {
	loads := map[string]func(a *App, id types.ID) tea.Cmd{
		"dashboard":           func(a *App, _ types.ID) tea.Cmd { return a.loadDashboardData() },
		"investment register": func(a *App, id types.ID) tea.Cmd { return a.loadInvestmentRegisterData(id) },
		"portfolio":           func(a *App, id types.ID) tea.Cmd { return a.portfolio.load(a.portfolioDeps(), id) },
		"portfolio lots": func(a *App, id types.ID) tea.Cmd {
			return a.portfolio.loadLotDetail(a.portfolioDeps(), id, types.NewID())
		},
	}
	for name, load := range loads {
		t.Run(name, func(t *testing.T) {
			a, acct := newOneInvestmentAccountApp(t)
			a.services.InvestmentValuation = nil
			runLoad(t, load(a, acct.ID))
		})
	}
}

func TestInvestmentLoads_WithoutTheInvestmentService(t *testing.T) {
	t.Run("dashboard", func(t *testing.T) {
		a, acct := newOneInvestmentAccountApp(t)
		a.services.Investment = nil
		msg, ok := runLoad(t, a.loadDashboardData()).(dashboardLoadedMsg)
		if !ok {
			t.Fatal("the load did not return dashboard data")
		}
		if msg.data.investmentHoldings[acct.ID] == nil {
			t.Error("the dashboard skipped the account's valuation, which needs only the valuation service")
		}
	})
	t.Run("investment register", func(t *testing.T) {
		a, acct := newOneInvestmentAccountApp(t)
		a.services.Investment = nil
		msg, ok := runLoad(t, a.loadInvestmentRegisterData(acct.ID)).(investmentRegisterLoadedMsg)
		if !ok {
			t.Fatal("the load did not return register data")
		}
		if msg.data.valuation == nil {
			t.Error("the register skipped the valuation, which needs only the valuation service")
		}
	})
	t.Run("portfolio", func(t *testing.T) {
		a, acct := newOneInvestmentAccountApp(t)
		a.services.Investment = nil
		msg, ok := runLoad(t, a.portfolio.load(a.portfolioDeps(), acct.ID)).(portfolioLoadedMsg)
		if !ok {
			t.Fatal("the load did not return portfolio data")
		}
		if msg.data.valuation == nil {
			t.Error("the portfolio skipped the valuation, which needs only the valuation service")
		}
	})
	t.Run("portfolio lots", func(t *testing.T) {
		a, acct := newOneInvestmentAccountApp(t)
		a.services.Investment = nil
		if _, ok := runLoad(t, a.portfolio.loadLotDetail(a.portfolioDeps(), acct.ID, types.NewID())).(portfolioLotDetailMsg); !ok {
			t.Error("the lot detail load failed, but it needs only the valuation service")
		}
	})
}
