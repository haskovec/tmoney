package report

import (
	"errors"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// figureValuer is an InvestmentValuer with a result or an error per account.
type figureValuer struct {
	results map[types.ID]ValuationResult
	errs    map[types.ID]error
}

func (v *figureValuer) GetAccountValuation(id types.ID, _ types.Date) (*ValuationResult, error) {
	if err := v.errs[id]; err != nil {
		return nil, err
	}
	r := v.results[id]
	return &r, nil
}

func newFigureEnv(t *testing.T, valuer InvestmentValuer) (*Service, *account.Repository) {
	t.Helper()
	database := createTestDB(t)
	repo := account.NewRepository(database)
	var opts []ServiceOption
	if valuer != nil {
		opts = append(opts, WithInvestmentValuer(valuer))
	}
	return NewService(repo, database, opts...), repo
}

func mkAccount(t *testing.T, repo *account.Repository, name string, typ account.Type, currency, opening string) *account.Account {
	t.Helper()
	acct := account.NewAccount(name, typ, currency, types.MustNewMoney(opening), types.Today())
	if err := repo.Create(acct); err != nil {
		t.Fatal(err)
	}
	return acct
}

func TestAccountFigures(t *testing.T) {
	valuer := &figureValuer{results: map[types.ID]ValuationResult{}, errs: map[types.ID]error{}}
	svc, repo := newFigureEnv(t, valuer)

	checking := mkAccount(t, repo, "Contoso Checking", account.TypeChecking, "USD", "250.00")
	// The opening balance is the brokerage's register balance, 100. Its value
	// is what the valuer says; the 100 must not stand in for it.
	brokerage := mkAccount(t, repo, "Northwind Brokerage", account.TypeInvestment, "USD", "100.00")
	broken := mkAccount(t, repo, "Fabrikam IRA", account.TypeInvestment, "USD", "0")
	valuer.results[brokerage.ID] = ValuationResult{
		TotalValue: types.MustNewMoney("1500.00"), CashBalance: types.MustNewMoney("300.00"), HasMissingPrices: true,
	}
	valuer.errs[broken.ID] = errors.New("price lookup failed")

	figs, err := svc.AccountFigures([]*account.Account{checking, brokerage, broken})
	if err != nil {
		t.Fatal(err)
	}

	if f := figs[0]; f.Err != nil || !f.Displayed.Equal(types.MustNewMoney("250.00")) || !f.Cash.Equal(f.Displayed) {
		t.Errorf("checking figure = %+v, want 250.00 displayed and cash", f)
	}
	if f := figs[1]; f.Err != nil || !f.Displayed.Equal(types.MustNewMoney("1500.00")) ||
		!f.Cash.Equal(types.MustNewMoney("300.00")) || !f.Estimated {
		t.Errorf("brokerage figure = %+v, want 1500.00 displayed, 300.00 cash, estimated", f)
	}
	if f := figs[2]; f.Err == nil {
		t.Errorf("broken figure = %+v, want an error", f)
	}
}

func TestAccountFigure_ReturnsValuationError(t *testing.T) {
	boom := errors.New("price lookup failed")
	valuer := &figureValuer{errs: map[types.ID]error{}}
	svc, repo := newFigureEnv(t, valuer)
	acct := mkAccount(t, repo, "Northwind Brokerage", account.TypeInvestment, "USD", "100.00")
	valuer.errs[acct.ID] = boom

	if _, err := svc.AccountFigure(acct.ID); !errors.Is(err, boom) {
		t.Fatalf("AccountFigure() error = %v, want %v", err, boom)
	}
}

func TestAccountFigures_NoValuerRefusesInvestment(t *testing.T) {
	svc, repo := newFigureEnv(t, nil)
	acct := mkAccount(t, repo, "Northwind Brokerage", account.TypeInvestment, "USD", "100.00")

	figs, err := svc.AccountFigures([]*account.Account{acct})
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(figs[0].Err, ErrNoInvestmentValuer) {
		t.Errorf("Err = %v, want ErrNoInvestmentValuer", figs[0].Err)
	}
}

func TestTotalsByCurrency(t *testing.T) {
	m := types.MustNewMoney
	figs := []AccountFigure{
		{Type: account.TypeChecking, Currency: "USD", Displayed: m("100")},
		{Type: account.TypeCreditCard, Currency: "USD", Displayed: m("-30")},
		{Type: account.TypeInvestment, Currency: "USD", Displayed: m("500"), Estimated: true},
		{Type: account.TypeSavings, Currency: "EUR", Displayed: m("40")},
	}

	got := TotalsByCurrency(figs)
	if len(got) != 2 || got[0].Currency != "EUR" || got[1].Currency != "USD" {
		t.Fatalf("totals = %+v, want EUR then USD", got)
	}
	eur, usd := got[0], got[1]
	if !eur.NetWorth.Equal(m("40")) || !eur.Available || eur.Estimated {
		t.Errorf("EUR = %+v, want 40, available, not estimated", eur)
	}
	if !usd.Assets.Equal(m("600")) || !usd.Liabilities.Equal(m("-30")) || !usd.NetWorth.Equal(m("570")) || !usd.Estimated {
		t.Errorf("USD = %+v, want assets 600, liabilities -30, net 570, estimated", usd)
	}

	t.Run("an error makes only its currency unavailable", func(t *testing.T) {
		got := TotalsByCurrency(append(figs, AccountFigure{Type: account.TypeInvestment, Currency: "USD", Err: errors.New("x")}))
		if got[1].Available {
			t.Error("USD total is available with a failed USD row")
		}
		if !got[0].Available {
			t.Error("EUR total lost its availability to a USD error")
		}
	})
}
