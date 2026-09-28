package report

import (
	"errors"
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// ErrNoInvestmentValuer is the Err of an investment account's figure when the
// service was built without WithInvestmentValuer. Its register balance is not
// its value, so the figure is refused rather than guessed.
var ErrNoInvestmentValuer = errors.New("no investment valuer is wired to the report service")

// AccountFigure is what an account is worth, as the CLI shows it.
//
// A register account's Displayed and Cash are both its register balance (the
// account_balances view). An investment account's Displayed is its valuation's
// TotalValue and its Cash is CashBalance; its register balance is never used
// in their place. When the valuation fails, Err is set and the money fields
// are not valid.
type AccountFigure struct {
	AccountID types.ID
	Type      account.Type
	Currency  string
	Displayed types.Money
	Cash      types.Money
	// Estimated is true when a holding had no price and was valued at cost.
	Estimated bool
	Err       error
}

// AccountFigure returns one account's figure as of today. A valuation error
// is returned, not stored in the figure.
func (s *Service) AccountFigure(id types.ID) (*AccountFigure, error) {
	acct, err := s.accountRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	figs, err := s.AccountFigures([]*account.Account{acct})
	if err != nil {
		return nil, err
	}
	if figs[0].Err != nil {
		return nil, figs[0].Err
	}
	return &figs[0], nil
}

// AccountFigures returns a figure for each account, in the order given, as of
// today. A valuation error goes into that account's Err and the others are
// still valued; only a failure to read the register balances is returned.
func (s *Service) AccountFigures(accounts []*account.Account) ([]AccountFigure, error) {
	registerBalances, err := s.registerBalances()
	if err != nil {
		return nil, err
	}
	today := types.Today()
	figs := make([]AccountFigure, len(accounts))
	for i, acct := range accounts {
		fig := AccountFigure{AccountID: acct.ID, Type: acct.Type, Currency: acct.Currency}
		if acct.Type.IsInvestmentType() {
			s.valueInvestmentFigure(&fig, today)
		} else {
			bal := registerBalances[acct.ID]
			fig.Displayed, fig.Cash = bal, bal
		}
		figs[i] = fig
	}
	return figs, nil
}

// valueInvestmentFigure fills fig from the investment valuer.
func (s *Service) valueInvestmentFigure(fig *AccountFigure, asOf types.Date) {
	if s.investmentValue == nil {
		fig.Err = ErrNoInvestmentValuer
		return
	}
	val, err := s.investmentValue.GetAccountValuation(fig.AccountID, asOf)
	if err != nil {
		fig.Err = fmt.Errorf("valuation failed: %w", err)
		return
	}
	fig.Displayed = val.TotalValue
	fig.Cash = val.CashBalance
	fig.Estimated = val.HasMissingPrices
}

// registerBalances reads every account's register balance from the
// account_balances view, the one definition of that figure.
func (s *Service) registerBalances() (map[types.ID]types.Money, error) {
	rows, err := s.db.Conn().Query(`SELECT id, current_balance FROM account_balances`)
	if err != nil {
		return nil, fmt.Errorf("failed to read register balances: %w", err)
	}
	defer rows.Close()
	out := make(map[types.ID]types.Money)
	for rows.Next() {
		var id types.ID
		var bal types.Money
		if err := rows.Scan(&id, &bal); err != nil {
			return nil, fmt.Errorf("failed to scan register balance: %w", err)
		}
		out[id] = bal
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating register balances: %w", err)
	}
	return out, nil
}

// CurrencyTotal is the net worth of the accounts in one currency. Money in
// different currencies is never added: there is no exchange rate.
type CurrencyTotal struct {
	Currency    string
	Assets      types.Money
	Liabilities types.Money // signed: negative when owed
	NetWorth    types.Money
	// Available is false when an account in this currency could not be
	// valued; the three sums then leave that account out and must not be
	// shown as the total.
	Available bool
	// Estimated is true when any account in this currency is Estimated.
	Estimated bool
}

// TotalsByCurrency adds the figures up per currency, sorted by currency code.
func TotalsByCurrency(figs []AccountFigure) []CurrencyTotal {
	byCur := make(map[string]*CurrencyTotal)
	for _, fig := range figs {
		t, ok := byCur[fig.Currency]
		if !ok {
			t = &CurrencyTotal{
				Currency: fig.Currency, Available: true,
				Assets: types.ZeroMoney, Liabilities: types.ZeroMoney, NetWorth: types.ZeroMoney,
			}
			byCur[fig.Currency] = t
		}
		if fig.Err != nil {
			t.Available = false
			continue
		}
		t.Estimated = t.Estimated || fig.Estimated
		switch {
		case fig.Type.IsAssetType():
			t.Assets = t.Assets.Add(fig.Displayed)
		case fig.Type.IsLiabilityType():
			t.Liabilities = t.Liabilities.Add(fig.Displayed)
		}
	}
	out := make([]CurrencyTotal, 0, len(byCur))
	for _, t := range byCur {
		t.NetWorth = t.Assets.Add(t.Liabilities)
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}
