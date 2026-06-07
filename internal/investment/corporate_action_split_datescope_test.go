package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/types"
)

// A stock split must only adjust shares held as of the split date. Shares
// bought AFTER the split were recorded at post-split quantities and must not be
// re-split. These tests pin that date-scoping (the reported VTI case: a 2008
// split applied to an account that only ever held post-2008 shares).

func TestCorporateActionService_Split_SkipsLotsPurchasedAfterSplitDate(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Wealthfront IRA")
	sec := createSec(t, env.secRepo, "VTI")
	buyDate := types.NewDate(2019, time.March, 1)   // bought post-split
	splitDate := types.NewDate(2008, time.June, 18) // VTI's real 2008 2:1 split

	if _, err := env.invSvc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1500.00")
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, buyDate, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	lots, err := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	if len(lots) != 1 {
		t.Fatalf("expected 1 lot, got %d", len(lots))
	}
	if !lots[0].Shares.Equal(types.MustNewQuantity("10")) {
		t.Errorf("lot shares = %s, want 10 (split predates the purchase; lot must be untouched)", lots[0].Shares.String())
	}
	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("10")) {
		t.Errorf("position shares = %s, want 10", pos.Shares.String())
	}
}

func TestCorporateActionService_Split_OnlyAdjustsLotsOnOrBeforeSplitDate(t *testing.T) {
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	preDate := types.NewDate(2020, time.January, 10)  // before split
	splitDate := types.NewDate(2020, time.August, 31) // AAPL 4:1 split
	postDate := types.NewDate(2021, time.March, 1)    // after split

	if _, err := env.invSvc.Deposit(acct.ID, preDate, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	pre := types.MustNewMoney("1000.00") // 10 sh @ $100
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, preDate, types.MustNewQuantity("10"), &pre, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() pre error = %v", err)
	}
	post := types.MustNewMoney("625.00") // 5 sh @ $125 (already split-adjusted)
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, postDate, types.MustNewQuantity("5"), &post, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() post error = %v", err)
	}

	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 4, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	preLot, postLot := lotsByDate(t, env, acct.ID, sec.ID, preDate, postDate)
	if !preLot.Shares.Equal(types.MustNewQuantity("40")) {
		t.Errorf("pre-split lot shares = %s, want 40 (4:1 applied)", preLot.Shares.String())
	}
	if !preLot.CostPerShare.Equal(types.MustNewMoney("25")) {
		t.Errorf("pre-split lot cost/share = %s, want 25", preLot.CostPerShare.String())
	}
	if !postLot.Shares.Equal(types.MustNewQuantity("5")) {
		t.Errorf("post-split lot shares = %s, want 5 (must be untouched)", postLot.Shares.String())
	}
	if !postLot.CostPerShare.Equal(types.MustNewMoney("125")) {
		t.Errorf("post-split lot cost/share = %s, want 125 (untouched)", postLot.CostPerShare.String())
	}
	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("45")) {
		t.Errorf("position shares = %s, want 45 (40 + 5)", pos.Shares.String())
	}
}

func TestCorporateActionService_Split_DateScoped_ReverseRoundTrips(t *testing.T) {
	// Reversal is date-scoped symmetrically: it un-adjusts the same lots the
	// split adjusted. (The downstream-transaction guard blocks reversing a split
	// that has later activity, so this exercises the pre-split-only path.)
	env := createCATestEnv(t)
	acct := createLotTrackingAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	preDate := types.NewDate(2020, time.January, 10)
	splitDate := types.NewDate(2020, time.August, 31)

	if _, err := env.invSvc.Deposit(acct.ID, preDate, types.MustNewMoney("100000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	pre := types.MustNewMoney("1000.00") // 10 sh @ $100
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, preDate, types.MustNewQuantity("10"), &pre, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	ca, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 4, Denominator: 1})
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	// Sanity: split applied (10 → 40 @ $25).
	lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if len(lots) != 1 || !lots[0].Shares.Equal(types.MustNewQuantity("40")) {
		t.Fatalf("post-split lot = %v, want 40 shares", lots)
	}

	if err := env.caSvc.DeleteAction(ca.ID); err != nil {
		t.Fatalf("DeleteAction() (reverse) error = %v", err)
	}

	lots, _ = env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
	if len(lots) != 1 {
		t.Fatalf("expected 1 lot after reverse, got %d", len(lots))
	}
	if !lots[0].Shares.Equal(types.MustNewQuantity("10")) {
		t.Errorf("lot shares after reverse = %s, want 10", lots[0].Shares.String())
	}
	if !lots[0].CostPerShare.Equal(types.MustNewMoney("100")) {
		t.Errorf("lot cost/share after reverse = %s, want 100", lots[0].CostPerShare.String())
	}
	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("10")) {
		t.Errorf("position shares after reverse = %s, want 10", pos.Shares.String())
	}
}

func TestCorporateActionService_Split_NonLot_SkipsSharesAcquiredAfterSplit(t *testing.T) {
	env := createCATestEnv(t)
	acct := createInvAccount(t, env.accountRepo, "401k") // non-lot
	sec := createSec(t, env.secRepo, "VTI")
	splitDate := types.NewDate(2008, time.June, 18)
	buyDate := types.NewDate(2019, time.March, 1) // entirely post-split

	if _, err := env.invSvc.Deposit(acct.ID, buyDate, types.MustNewMoney("10000.00"), ""); err != nil {
		t.Fatalf("Deposit() error = %v", err)
	}
	total := types.MustNewMoney("1500.00")
	if _, err := env.invSvc.Buy(acct.ID, sec.ID, buyDate, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy() error = %v", err)
	}

	if _, err := env.caSvc.Split(sec.ID, splitDate, SplitParams{Numerator: 2, Denominator: 1}); err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
	if err != nil {
		t.Fatalf("GetByAccountAndSecurity() error = %v", err)
	}
	if !pos.Shares.Equal(types.MustNewQuantity("10")) {
		t.Errorf("non-lot position shares = %s, want 10 (held nothing at the 2008 split)", pos.Shares.String())
	}
}

// lotsByDate returns the open lots matching the two given purchase dates.
func lotsByDate(t *testing.T, env *testCAServiceEnv, accountID, securityID types.ID, dateA, dateB types.Date) (a, b *Lot) {
	t.Helper()
	lots, err := env.lotRepo.ListByAccountAndSecurity(accountID, securityID, false)
	if err != nil {
		t.Fatalf("ListByAccountAndSecurity() error = %v", err)
	}
	for _, l := range lots {
		switch {
		case l.PurchaseDate.Time().Equal(dateA.Time()):
			a = l
		case l.PurchaseDate.Time().Equal(dateB.Time()):
			b = l
		}
	}
	if a == nil || b == nil {
		t.Fatalf("expected lots on %s and %s, got %d lots", dateA.String(), dateB.String(), len(lots))
	}
	return a, b
}
