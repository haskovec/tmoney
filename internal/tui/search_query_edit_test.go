package tui

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/sidebar"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// queryInputs are typed into each search, then one Backspace must remove one
// character as the user sees it. A byte cut left invalid UTF-8 that matched
// nothing; a rune cut left half a character, such as a lone skin tone.
var queryInputs = []struct{ name, text, afterBackspace string }{
	{"precomposed", "Café", "Caf"},
	{"combining", "Café", "Caf"},
	{"cjk", "東京", "東"},
	{"skin tone", "👍🏽", ""},
	{"zwj family", "👨‍👩‍👧", ""},
	{"flag", "🇺🇸", ""},
	{"ascii", "Acme", "Acm"},
}

// keysFor returns the key presses that type text. A terminal sends a
// multi-rune character as one key, so "graphemes" mode does that; "runes"
// mode sends one key per rune, as when the characters arrive one by one.
func keysFor(text, mode string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	if mode == "runes" {
		for _, r := range text {
			keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		return keys
	}
	for len(text) > 0 {
		g, _ := ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
		code := []rune(g)[0]
		if utf8.RuneCountInString(g) > 1 {
			code = tea.KeyExtended
		}
		keys = append(keys, tea.KeyPressMsg{Code: code, Text: g})
		text = text[len(g):]
	}
	return keys
}

// querySite is one search under test.
type querySite struct {
	name   string
	newApp func(t *testing.T) *App
	query  func(a *App) string
}

// querySecurities returns one security per input, so each query matches.
func querySecurities() []*security.Security {
	var out []*security.Security
	for i, in := range queryInputs {
		out = append(out, security.NewSecurity(fmt.Sprintf("ZZ%d", i), in.text, security.TypeStock))
	}
	return out
}

func querySites() []querySite {
	return []querySite{
		{
			name: "prices",
			newApp: func(t *testing.T) *App {
				a := NewApp(dbtest.New(t), nil)
				a.width, a.height = 120, 40
				a.switchView(ViewPrices)
				runCmd(t, a, a.prices.load(a.priceDeps()), 1)
				a.prices.data.securities = querySecurities()
				return a
			},
			query: func(a *App) string { return a.prices.data.searchQuery },
		},
		{
			name: "securities",
			newApp: func(t *testing.T) *App {
				a := NewApp(dbtest.New(t), nil)
				a.width, a.height = 120, 40
				a.switchView(ViewSecurities)
				runCmd(t, a, a.loadSecurityViewData(), 1)
				a.securities.data.securities = querySecurities()
				a.buildSecurityTable()
				return a
			},
			query: func(a *App) string { return a.securities.data.searchQuery },
		},
		{
			name: "corporate actions",
			newApp: func(t *testing.T) *App {
				a := NewApp(dbtest.New(t), nil)
				a.width, a.height = 120, 40
				a.switchView(ViewCorporateActions)
				runCmd(t, a, a.corporateActions.load(a.corporateActionDeps()), 1)
				secMap := map[types.ID]*security.Security{}
				var actions []*investment.CorporateAction
				params, _ := investment.SplitParams{Numerator: 2, Denominator: 1}.ToJSON()
				for _, in := range queryInputs {
					id := types.NewID()
					secMap[id] = &security.Security{Ticker: in.text, Name: in.text}
					actions = append(actions, investment.NewCorporateAction(investment.ActionTypeSplit, id, types.NewDate(2024, time.June, 1), params))
				}
				a.corporateActions.data.actions = actions
				a.corporateActions.data.secMap = secMap
				a.corporateActions.buildTable()
				return a
			},
			query: func(a *App) string { return a.corporateActions.filter },
		},
		{
			name: "investment register",
			newApp: func(t *testing.T) *App {
				acct := types.NewID()
				names := map[types.ID]string{}
				full := map[types.ID]string{}
				var txns []*investment.Transaction
				for i, in := range queryInputs {
					id := types.NewID()
					names[id] = fmt.Sprintf("ZZ%d", i)
					full[id] = in.text
					txns = append(txns, investment.NewTransactionWithSecurity(acct, types.NewDate(2024, time.March, 15),
						investment.TransactionTypeBuy, types.MustNewMoney("-10.00"), id, types.MustNewQuantity("1")))
				}
				styles := widget.NewStyles()
				styles.Resize(120, 40)
				sb := sidebar.New()
				sb.SetFocused(false)
				a := &App{
					currentView: ViewInvestmentRegister,
					width:       120,
					height:      40,
					ready:       true,
					keys:        defaultKeyMap(),
					styles:      styles,
					sidebar:     sb,
					menubar:     widget.NewMenuBar(),
					statusbar:   widget.NewStatusBar(),
					investmentRegister: investmentRegisterViewState{data: &investmentRegisterData{
						account:           &account.Account{BaseModel: types.BaseModel{ID: acct}, Name: "Example 401k", Type: account.TypeInvestment, Active: true},
						transactions:      txns,
						securityNames:     names,
						securityFullNames: full,
					}},
				}
				a.investmentRegister.buildTable(a.styles)
				return a
			},
			query: func(a *App) string { return a.investmentRegister.filterQuery },
		},
	}
}

func TestSearchQuery_BackspaceRemovesOneCharacter(t *testing.T) {
	for _, site := range querySites() {
		for _, mode := range []string{"graphemes", "runes"} {
			for _, in := range queryInputs {
				t.Run(fmt.Sprintf("%s/%s/%s", site.name, mode, in.name), func(t *testing.T) {
					a := site.newApp(t)
					press(a, tea.KeyPressMsg{Code: '/', Text: "/"})
					press(a, keysFor(in.text, mode)...)
					if got := site.query(a); got != in.text {
						t.Fatalf("query after typing = %+q, want %+q", got, in.text)
					}
					press(a, tea.KeyPressMsg{Code: tea.KeyBackspace})
					got := site.query(a)
					if !utf8.ValidString(got) || got != in.afterBackspace {
						t.Errorf("query after Backspace = %+q, want %+q", got, in.afterBackspace)
					}
				})
			}
		}
	}
}
