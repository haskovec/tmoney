package report

import (
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// Holdings returns the holdings report as of today: the open positions of
// the active investment accounts, added up per security, plus one Cash row
// per currency for their uninvested cash.
//
// The accounts are the ones NetWorthReport values today, so for a currency
// whose accounts were all valued, the section's Value is that currency's
// investment value in net worth. An account the valuer cannot value goes into
// Failed and makes its section not Available; the others are still added.
func (s *Service) Holdings() (*Holdings, error) {
	today := types.Today()
	accounts, err := s.accountRepo.List(true)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	// Read the labels before any valuation: the valuer runs its own queries,
	// and holding the pooled connection while it waits for another
	// deadlocked a one-connection pool (see netWorthAsOf).
	labels, err := s.securityLabels()
	if err != nil {
		return nil, err
	}

	rpt := &Holdings{AsOfDate: today}
	sections := make(map[string]*holdingsBuilder)
	for _, acct := range accounts {
		if !acct.Type.IsInvestmentType() || acct.OpeningDate.After(today) {
			continue
		}
		b, ok := sections[acct.Currency]
		if !ok {
			b = newHoldingsBuilder(acct.Currency)
			sections[acct.Currency] = b
		}
		val, err := s.valueAccount(acct.ID, today)
		if err != nil {
			rpt.Failed = append(rpt.Failed, AccountFailure{
				AccountID: acct.ID, Name: acct.Name, Currency: acct.Currency, Err: err,
			})
			b.available = false
			continue
		}
		b.addCash(acct, val.CashBalance)
		for _, h := range val.Holdings {
			b.addHolding(acct, h, labels[h.SecurityID])
		}
	}

	for _, b := range sections {
		rpt.Sections = append(rpt.Sections, b.build())
	}
	sort.Slice(rpt.Sections, func(i, j int) bool { return rpt.Sections[i].Currency < rpt.Sections[j].Currency })
	return rpt, nil
}

// securityLabel is what a holdings row shows for its security.
type securityLabel struct {
	label string // the ticker, or the name when there is no ticker
	name  string
}

// securityLabels reads the label of every security, hidden ones included: a
// hidden security that is still held is still a holding.
func (s *Service) securityLabels() (map[types.ID]securityLabel, error) {
	rows, err := s.db.Conn().Query(`SELECT id, ticker, name FROM securities`)
	if err != nil {
		return nil, fmt.Errorf("failed to read securities: %w", err)
	}
	defer rows.Close()
	out := make(map[types.ID]securityLabel)
	for rows.Next() {
		var id types.ID
		var ticker, name string
		if err := rows.Scan(&id, &ticker, &name); err != nil {
			return nil, fmt.Errorf("failed to scan security: %w", err)
		}
		label := ticker
		if label == "" {
			label = name
		}
		out[id] = securityLabel{label: label, name: name}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating securities: %w", err)
	}
	return out, nil
}

// holdingsBuilder adds up one currency's holdings.
type holdingsBuilder struct {
	currency  string
	available bool
	rows      map[types.ID]*HoldingRow
	cash      HoldingRow
}

func newHoldingsBuilder(currency string) *holdingsBuilder {
	return &holdingsBuilder{
		currency:  currency,
		available: true,
		rows:      make(map[types.ID]*HoldingRow),
		cash: HoldingRow{
			SecurityID: types.NilID, Cash: true, Label: "Cash", Name: "Uninvested cash",
			Shares: types.ZeroQuantity, Price: types.ZeroMoney, Value: types.ZeroMoney,
			CostBasis: types.ZeroMoney, Gain: types.ZeroMoney,
		},
	}
}

// addCash adds an account's cash to the Cash row. An account with no cash
// gets no line in the row's split.
func (b *holdingsBuilder) addCash(acct *account.Account, cash types.Money) {
	if cash.IsZero() {
		return
	}
	b.cash.Value = b.cash.Value.Add(cash)
	b.cash.CostBasis = b.cash.CostBasis.Add(cash)
	b.cash.Accounts = append(b.cash.Accounts, HoldingAccount{
		AccountID: acct.ID, Name: acct.Name, Shares: types.ZeroQuantity, Value: cash,
	})
}

// addHolding adds one account's position to its security's row.
func (b *holdingsBuilder) addHolding(acct *account.Account, h HoldingFigure, sec securityLabel) {
	row, ok := b.rows[h.SecurityID]
	if !ok {
		label := sec.label
		if label == "" {
			label = h.SecurityID.String()
		}
		row = &HoldingRow{
			SecurityID: h.SecurityID, Label: label, Name: sec.name,
			Shares: types.ZeroQuantity, Price: types.ZeroMoney, Value: types.ZeroMoney,
			CostBasis: types.ZeroMoney, Gain: types.ZeroMoney,
		}
		b.rows[h.SecurityID] = row
	}
	row.Shares = row.Shares.Add(h.Shares)
	row.Value = row.Value.Add(h.MarketValue)
	row.CostBasis = row.CostBasis.Add(h.CostBasis)
	if h.HasPricing {
		row.Price = h.Price
	} else {
		row.Estimated = true
	}
	row.Accounts = append(row.Accounts, HoldingAccount{
		AccountID: acct.ID, Name: acct.Name, Shares: h.Shares, Value: h.MarketValue,
	})
}

// build returns the section: gains, totals and percentages, with the rows
// and each row's accounts sorted largest first.
func (b *holdingsBuilder) build() HoldingsSection {
	sec := HoldingsSection{
		Currency:  b.currency,
		Available: b.available,
		Value:     types.ZeroMoney,
		CostBasis: types.ZeroMoney,
		Gain:      types.ZeroMoney,
	}
	for _, row := range b.rows {
		if row.Estimated {
			row.Price = types.ZeroMoney
		} else {
			row.Gain = row.Value.Sub(row.CostBasis)
		}
		sec.Rows = append(sec.Rows, *row)
	}
	if !b.cash.Value.IsZero() {
		sec.Rows = append(sec.Rows, b.cash)
	}

	for _, row := range sec.Rows {
		sec.Value = sec.Value.Add(row.Value)
		sec.CostBasis = sec.CostBasis.Add(row.CostBasis)
		sec.Gain = sec.Gain.Add(row.Gain)
		sec.Estimated = sec.Estimated || row.Estimated
	}
	for i := range sec.Rows {
		row := &sec.Rows[i]
		row.Percent = percentOf(row.Value, sec.Value)
		for j := range row.Accounts {
			row.Accounts[j].Percent = percentOf(row.Accounts[j].Value, row.Value)
		}
		sort.SliceStable(row.Accounts, func(x, y int) bool {
			a, c := row.Accounts[x], row.Accounts[y]
			if cmp := a.Value.Cmp(c.Value); cmp != 0 {
				return cmp > 0
			}
			return a.Name < c.Name
		})
	}
	sort.Slice(sec.Rows, func(i, j int) bool {
		a, c := sec.Rows[i], sec.Rows[j]
		if cmp := a.Value.Cmp(c.Value); cmp != 0 {
			return cmp > 0
		}
		if a.Label != c.Label {
			return a.Label < c.Label
		}
		return a.SecurityID.String() < c.SecurityID.String()
	})
	return sec
}

// percentOf is part's share of whole, in percent; 0 when whole is not
// positive.
func percentOf(part, whole types.Money) float64 {
	if !whole.IsPositive() {
		return 0
	}
	return part.Float64() / whole.Float64() * 100
}
