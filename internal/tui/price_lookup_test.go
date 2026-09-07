package tui

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

func TestPriceDialog_LookupFillsPriceAndResolvedDate(t *testing.T) {
	app, fp, secs := setupRefreshTUITest(t, "GBTC")
	fp.quotes["GBTC"] = &price.Quote{
		Date:     types.NewDate(2024, time.July, 31),
		Price:    types.MustNewMoney("52.07"),
		Currency: "USD",
	}
	sec := secs[0]
	app.priceView = &priceViewData{selectedSecurity: sec}
	app.price = &priceSurface{modalSurface: modalSurface{dlg: buildAddPriceDialog(sec)}}
	// A weekend date; the fake ignores it but the resolved quote date is 07-31.
	app.price.dlg.Fields()[0].Value = "2024-08-03"

	_, cmd := app.startPriceLookup()
	if cmd == nil {
		t.Fatal("startPriceLookup returned nil cmd")
	}
	msg, ok := cmd().(priceLookupResultMsg)
	if !ok {
		t.Fatalf("expected priceLookupResultMsg, got different msg")
	}
	if msg.err != nil {
		t.Fatalf("lookup error: %v", msg.err)
	}
	app.handlePriceLookupResult(msg)

	fields := app.price.dlg.Fields()
	if fields[1].Value != "52.07" {
		t.Errorf("Price field = %q, want 52.07", fields[1].Value)
	}
	if fields[0].Value != "2024-07-31" {
		t.Errorf("Date field = %q, want resolved 2024-07-31", fields[0].Value)
	}
}

func TestPriceDialog_LookupErrorKeepsDialogOpen(t *testing.T) {
	app, _, secs := setupRefreshTUITest(t, "GBTC")
	// No quote registered for GBTC → the fake returns an error.
	sec := secs[0]
	app.priceView = &priceViewData{selectedSecurity: sec}
	app.price = &priceSurface{modalSurface: modalSurface{dlg: buildAddPriceDialog(sec)}}
	app.price.dlg.Fields()[0].Value = "2024-07-31"

	_, cmd := app.startPriceLookup()
	msg, ok := cmd().(priceLookupResultMsg)
	if !ok {
		t.Fatalf("expected priceLookupResultMsg")
	}
	if msg.err == nil {
		t.Fatal("expected an error for an unknown ticker")
	}
	app.handlePriceLookupResult(msg)
	if app.price == nil {
		t.Error("dialog should remain open after a failed lookup")
	}
}

// TestPriceDialog_LookupPrefill_AnchorsPriceCursor guards the cursor after a
// quote lookup fills the Price field. The Date field beside it must stay at 0:
// it is a masked date that overwrites digits from the first position.
func TestPriceDialog_LookupPrefill_AnchorsPriceCursor(t *testing.T) {
	app, fp, secs := setupRefreshTUITest(t, "GBTC")
	fp.quotes["GBTC"] = &price.Quote{
		Date:     types.NewDate(2024, time.July, 31),
		Price:    types.MustNewMoney("52.07"),
		Currency: "USD",
	}
	sec := secs[0]
	app.priceView = &priceViewData{selectedSecurity: sec}
	app.price = &priceSurface{modalSurface: modalSurface{dlg: buildAddPriceDialog(sec)}}
	app.price.dlg.Fields()[0].Value = "2024-08-03"

	_, cmd := app.startPriceLookup()
	msg, ok := cmd().(priceLookupResultMsg)
	if !ok {
		t.Fatal("expected priceLookupResultMsg")
	}
	app.handlePriceLookupResult(msg)

	fields := app.price.dlg.Fields()
	assertPrefillEditable(t, fields[1], "price")
	if got := fields[0].CursorPos(); got != 0 {
		t.Errorf("date field CursorPos() = %d, want 0 (the mask overwrites from the first digit)", got)
	}
}
