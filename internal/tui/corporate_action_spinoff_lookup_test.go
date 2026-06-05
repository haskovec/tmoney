package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

func TestSpinOffDialog_PriceLookupFillsChildPrice(t *testing.T) {
	database := dbtest.New(t)

	secRepo := security.NewRepository(database)
	priceSvc := price.NewService(price.NewRepository(database), secRepo, database)
	securitySvc := security.NewService(secRepo, database)

	fp := &fakeRefreshProvider{quotes: map[string]*price.Quote{}, errors: map[string]error{}}
	priceSvc.ProviderRegistry().Register(fp)

	child := security.NewSecurity("BTC", "Grayscale Bitcoin Mini Trust", security.TypeETF)
	if err := secRepo.Create(child); err != nil {
		t.Fatalf("create security: %v", err)
	}
	fp.quotes["BTC"] = &price.Quote{
		Date:     types.NewDate(2024, time.July, 31),
		Price:    types.MustNewMoney("5.84"),
		Currency: "USD",
	}

	ids := []types.ID{child.ID}
	app := &App{
		statusbar:                widget.NewStatusBar(),
		priceSvc:                 priceSvc,
		securitySvc:              securitySvc,
		spinOffDialog:            buildSpinOffDialog([]string{"BTC - Grayscale Bitcoin Mini Trust"}, ids, nil),
		spinOffDialogSecurityIDs: ids,
	}
	fields := app.spinOffDialog.Fields()
	fields[1].SelectedIndex = 0 // spin-off security = BTC
	fields[2].Value = "07/31/2024"

	_, cmd := app.startSpinOffPriceLookup()
	if cmd == nil {
		t.Fatal("expected a lookup command")
	}
	msg, ok := cmd().(spinOffPriceLookupMsg)
	if !ok {
		t.Fatalf("expected spinOffPriceLookupMsg")
	}
	if msg.err != nil {
		t.Fatalf("lookup error: %v", msg.err)
	}
	app.handleSpinOffPriceLookupResult(msg)

	if got := app.spinOffDialog.Fields()[5].Value; got != "5.84" {
		t.Errorf("Spin-Off Price field = %q, want 5.84", got)
	}
}
