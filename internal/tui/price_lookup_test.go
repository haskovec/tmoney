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
	app.priceDialog = buildAddPriceDialog(sec)
	// A weekend date; the fake ignores it but the resolved quote date is 07-31.
	app.priceDialog.Fields()[0].Value = "2024-08-03"

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

	fields := app.priceDialog.Fields()
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
	app.priceDialog = buildAddPriceDialog(sec)
	app.priceDialog.Fields()[0].Value = "2024-07-31"

	_, cmd := app.startPriceLookup()
	msg, ok := cmd().(priceLookupResultMsg)
	if !ok {
		t.Fatalf("expected priceLookupResultMsg")
	}
	if msg.err == nil {
		t.Fatal("expected an error for an unknown ticker")
	}
	app.handlePriceLookupResult(msg)
	if app.priceDialog == nil {
		t.Error("dialog should remain open after a failed lookup")
	}
}
