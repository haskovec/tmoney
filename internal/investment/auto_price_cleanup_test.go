package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// Auto-price cleanup on edit/delete
//
// A buy/sell/reinvest/fee-liquidation auto-creates a security_prices row
// (source=transaction) at its date. Editing the transaction's date or deleting
// it used to leave that row behind — the orphan that stretched the VTI chart
// across ~2000 years after a buy's year was fixed from 0018 to 2018. These
// tests pin the reconciliation: orphans are removed, but prices shared by a
// surviving same-day transaction are kept (and re-pointed to a survivor), and
// manual/import/api prices are never touched.
//
// The original "0018" typo can no longer be entered at all — Account opening-
// date validation (account.Account.ValidateTransactionDate) now rejects a buy
// dated before the account opened. This test therefore exercises the same
// price-moves-with-the-date reconciliation across a legitimate date edit.
// =============================================================================

func TestAutoPriceCleanup_EditBuyDate_MovesPrice(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	origDate := types.NewDate(2018, time.May, 3)
	editedDate := types.NewDate(2019, time.June, 15)
	shares := types.MustNewQuantity("1")
	buyPrice := types.MustNewMoney("135.05")

	buy, err := env.svc.Buy(acct.ID, sec.ID, origDate, shares, nil, &buyPrice, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.priceRepo.GetBySecurityAndDate(sec.ID, origDate); err != nil {
		t.Fatalf("expected auto-price at original date after Buy: %v", err)
	}

	if _, err := env.svc.UpdateBuy(buy.ID, acct.ID, sec.ID, editedDate, shares, nil, &buyPrice, types.ZeroMoney, ""); err != nil {
		t.Fatalf("UpdateBuy() error = %v", err)
	}

	if p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, origDate); err == nil {
		t.Errorf("expected original-date auto-price removed, still present: %s", p.Price.String())
	}
	got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, editedDate)
	if err != nil {
		t.Fatalf("expected auto-price at corrected date: %v", err)
	}
	if !got.Price.Equal(buyPrice) {
		t.Errorf("corrected-date price = %s, want %s", got.Price.String(), buyPrice.String())
	}
}

func TestAutoPriceCleanup_SharedDateAcrossAccounts_PriceSurvives(t *testing.T) {
	env := createFullTestService(t)
	acctA := createLotTrackingAccount(t, env.accountRepo, "A")
	acctB := createLotTrackingAccount(t, env.accountRepo, "B")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2018, time.May, 3)
	otherDate := types.NewDate(2018, time.May, 10)
	shares := types.MustNewQuantity("1")
	pA := types.MustNewMoney("135.05")
	pB := types.MustNewMoney("135.20")

	// A is entered first and seeds the (security, date) price at 135.05.
	buyA, err := env.svc.Buy(acctA.ID, sec.ID, date, shares, nil, &pA, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy(A) error = %v", err)
	}
	// B buys the same security on the same day; autoCreatePrice no-ops (price exists).
	if _, err := env.svc.Buy(acctB.ID, sec.ID, date, shares, nil, &pB, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(B) error = %v", err)
	}
	seed, _ := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if !seed.Price.Equal(pA) {
		t.Fatalf("seed price = %s, want %s (A first)", seed.Price.String(), pA.String())
	}

	// Move A's buy off the shared date.
	if _, err := env.svc.UpdateBuy(buyA.ID, acctA.ID, sec.ID, otherDate, shares, nil, &pA, types.ZeroMoney, ""); err != nil {
		t.Fatalf("UpdateBuy() error = %v", err)
	}

	// The shared-date price must survive (B still holds it) and re-point to B's price.
	got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if err != nil {
		t.Fatalf("expected shared-date price to survive: %v", err)
	}
	if !got.Price.Equal(pB) {
		t.Errorf("shared-date price = %s, want %s (re-pointed to surviving B)", got.Price.String(), pB.String())
	}
}

func TestAutoPriceCleanup_TwoSameDayLots_DifferentPrices(t *testing.T) {
	// The scenario the user asked about: one account buys the same security in
	// two lots on the same day at slightly different prices, then edits one lot
	// off the date. The day's price row must remain (the other lot needs it) and
	// re-point to the surviving lot's price.
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2018, time.May, 3)
	otherDate := types.NewDate(2018, time.May, 4)
	shares := types.MustNewQuantity("1")
	p1 := types.MustNewMoney("135.05")
	p2 := types.MustNewMoney("135.20")

	buy1, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &p1, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() 1 error = %v", err)
	}
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &p2, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() 2 error = %v", err)
	}
	lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if len(lots) != 2 {
		t.Fatalf("expected 2 lots, got %d", len(lots))
	}

	// Edit lot 1's date.
	if _, err := env.svc.UpdateBuy(buy1.ID, acct.ID, sec.ID, otherDate, shares, nil, &p1, types.ZeroMoney, ""); err != nil {
		t.Fatalf("UpdateBuy() error = %v", err)
	}

	// Original date keeps a price, re-pointed to surviving lot 2.
	got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if err != nil {
		t.Fatalf("price at original date should survive: %v", err)
	}
	if !got.Price.Equal(p2) {
		t.Errorf("original-date price = %s, want %s (surviving lot)", got.Price.String(), p2.String())
	}
	// Moved lot created a price at its new date.
	moved, err := env.priceRepo.GetBySecurityAndDate(sec.ID, otherDate)
	if err != nil {
		t.Fatalf("price at moved date expected: %v", err)
	}
	if !moved.Price.Equal(p1) {
		t.Errorf("moved-date price = %s, want %s", moved.Price.String(), p1.String())
	}
}

func TestAutoPriceCleanup_ManualPriceUntouched(t *testing.T) {
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2018, time.May, 3)
	otherDate := types.NewDate(2018, time.May, 10)
	shares := types.MustNewQuantity("1")
	buyPrice := types.MustNewMoney("135.05")
	manualPrice := types.MustNewMoney("130.00")

	// A manually entered price on the date — autoCreatePrice will not overwrite it.
	if err := env.priceRepo.Create(price.NewPrice(sec.ID, date, manualPrice, price.SourceManual)); err != nil {
		t.Fatalf("seed manual price: %v", err)
	}
	buy, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &buyPrice, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	// Move the buy off the date — the manual price must be left completely alone.
	if _, err := env.svc.UpdateBuy(buy.ID, acct.ID, sec.ID, otherDate, shares, nil, &buyPrice, types.ZeroMoney, ""); err != nil {
		t.Fatalf("UpdateBuy() error = %v", err)
	}

	got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if err != nil {
		t.Fatalf("manual price should survive: %v", err)
	}
	if got.Source != price.SourceManual {
		t.Errorf("source = %s, want manual", got.Source)
	}
	if !got.Price.Equal(manualPrice) {
		t.Errorf("manual price changed to %s, want %s", got.Price.String(), manualPrice.String())
	}
}

func TestAutoPriceCleanup_SameDatePriceEdit_RefreshesDailyPrice(t *testing.T) {
	// Editing only the price (same date) should refresh the day's auto-price to
	// the corrected value rather than leaving the original seed price stale.
	env := createFullTestService(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "VTI")
	date := types.NewDate(2018, time.May, 3)
	shares := types.MustNewQuantity("1")
	p1 := types.MustNewMoney("135.05")
	p2 := types.MustNewMoney("136.00")

	buy, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &p1, types.ZeroMoney, "")
	if err != nil {
		t.Fatalf("Buy() error = %v", err)
	}
	if _, err := env.svc.UpdateBuy(buy.ID, acct.ID, sec.ID, date, shares, nil, &p2, types.ZeroMoney, ""); err != nil {
		t.Fatalf("UpdateBuy() error = %v", err)
	}
	got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
	if err != nil {
		t.Fatalf("price expected at date: %v", err)
	}
	if !got.Price.Equal(p2) {
		t.Errorf("daily price = %s, want %s after same-date price edit", got.Price.String(), p2.String())
	}
}

func TestAutoPriceCleanup_DeleteBuy(t *testing.T) {
	t.Run("deleting the sole buy removes its auto-price", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		date := types.NewDate(2018, time.May, 3)
		buyPrice := types.MustNewMoney("135.05")

		buy, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("1"), nil, &buyPrice, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() error = %v", err)
		}
		if err := env.svc.DeleteTransaction(buy.ID); err != nil {
			t.Fatalf("DeleteTransaction() error = %v", err)
		}
		if p, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date); err == nil {
			t.Errorf("expected auto-price removed after delete, still present: %s", p.Price.String())
		}
	})

	t.Run("deleting one of two same-day buys keeps the price, re-pointed to survivor", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "VTI")
		date := types.NewDate(2018, time.May, 3)
		shares := types.MustNewQuantity("1")
		p1 := types.MustNewMoney("135.05")
		p2 := types.MustNewMoney("135.20")

		buy1, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &p1, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy() 1 error = %v", err)
		}
		if _, err := env.svc.Buy(acct.ID, sec.ID, date, shares, nil, &p2, types.ZeroMoney, ""); err != nil {
			t.Fatalf("Buy() 2 error = %v", err)
		}

		if err := env.svc.DeleteTransaction(buy1.ID); err != nil {
			t.Fatalf("DeleteTransaction() error = %v", err)
		}

		got, err := env.priceRepo.GetBySecurityAndDate(sec.ID, date)
		if err != nil {
			t.Fatalf("price should survive (buy2 remains): %v", err)
		}
		if !got.Price.Equal(p2) {
			t.Errorf("price = %s, want %s (re-pointed to surviving buy)", got.Price.String(), p2.String())
		}
	})
}
