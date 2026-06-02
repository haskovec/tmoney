package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

func TestInvestmentTypeSelector_NewIncludesSpinOff(t *testing.T) {
	app := &App{}
	app.openInvestmentTypeSelector(false)
	opts := app.investmentTypeSelector.Fields()[0].Options
	if len(opts) != len(investmentTransactionTypeOptions())+1 {
		t.Fatalf("New selector should add one option, got %d", len(opts))
	}
	if opts[len(opts)-1] != "Spin-Off…" {
		t.Errorf("last option = %q, want Spin-Off…", opts[len(opts)-1])
	}
}

func TestInvestmentTypeSelector_EditExcludesSpinOff(t *testing.T) {
	app := &App{}
	app.openInvestmentTypeSelector(true) // no register: selectedInvestmentTransaction is nil-safe
	opts := app.investmentTypeSelector.Fields()[0].Options
	if len(opts) != len(investmentTransactionTypeOptions()) {
		t.Errorf("Edit selector should not add options, got %d", len(opts))
	}
	for _, o := range opts {
		if o == "Spin-Off…" {
			t.Fatal("Edit selector must not include Spin-Off…")
		}
	}
}

func TestInvestmentTypeSelector_SpinOffDispatch(t *testing.T) {
	secID := types.NewID()
	txn := &investment.Transaction{
		BaseModel:  types.NewBaseModel(),
		Type:       investment.TransactionTypeBuy,
		SecurityID: types.NullableID{ID: secID, Valid: true},
	}
	app := &App{
		width:   80,
		height:  24,
		keys:    defaultKeyMap(),
		sidebar: NewSidebar(),
		investmentRegister: &investmentRegisterData{
			account:       &account.Account{BaseModel: types.NewBaseModel(), Name: "Brokerage", Type: account.TypeInvestment},
			transactions:  []*investment.Transaction{txn},
			securityNames: map[types.ID]string{secID: "GBTC"},
		},
	}
	app.buildInvestmentRegisterTable()

	app.openInvestmentTypeSelector(false)
	fields := app.investmentTypeSelector.Fields()
	fields[0].SelectedIndex = len(fields[0].Options) - 1 // the Spin-Off… entry

	// Enter advances focus to the primary button, then submits.
	var cmd tea.Cmd
	for i := 0; i < 5 && app.investmentTypeSelector != nil; i++ {
		_, cmd = app.handleInvestmentTypeSelectorKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	}

	if app.investmentTypeSelector != nil {
		t.Fatal("selector should be closed after submitting Spin-Off…")
	}
	if cmd == nil {
		t.Error("expected a command to load the spin-off dialog")
	}
	if app.spinOffDialogPreSelectedID == nil || *app.spinOffDialogPreSelectedID != secID {
		t.Errorf("spin-off parent should be pre-filled to the selected holding %v, got %v", secID, app.spinOffDialogPreSelectedID)
	}
}
